package recording_test

import (
	"encoding/binary"
	"log/slog"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/japanese"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/episode"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// 同じ回かを確かめ直す (#1500) テストの番組。番組表 (数日前の EIT[schedule]) の番組名と概要には話数がなく、
// 放送の直前の EIT[p/f] の概要にだけ話数と副題が載る局を真似る。
const (
	recheckName        = "こたつ探偵"
	recheckSynopsis    = "こたつから一歩も出ない探偵が、近所の謎を解きます。"
	recheckDescription = "第６話「消えたみかん」　" + recheckSynopsis
	// recheckOther は、別の回 (第５話) の概要。
	recheckOther = "第５話「迷子のこたつ」　" + recheckSynopsis
)

// aribText は、文字列を ARIB STD-B24 の 8 単位符号 (初めの状態のまま、漢字系の集合を GL で使う) にする。JIS X 0208 の全角の文字だけを書ける。
func aribText(t *testing.T, s string) []byte {
	t.Helper()

	encoded, err := japanese.EUCJP.NewEncoder().Bytes([]byte(s))
	require.NoError(t, err)
	out := make([]byte, 0, len(encoded))
	for _, b := range encoded {
		require.GreaterOrEqual(t, b, byte(0x80), "only full-width characters can be written: %q", s)
		out = append(out, b&0x7F)
	}

	return out
}

// lengthByte は、記述子の長さの 1 バイトを返す。
func lengthByte(n int) byte {
	if n < 0 || n > 0xFF {
		panic("the field is too long")
	}

	return byte(n)
}

// length12 は、12 ビットの長さ (section_length や、記述子の並びの長さ) を返す。
func length12(n int) uint16 {
	if n < 0 || n > 0x0FFF {
		panic("the descriptors are too long")
	}

	return uint16(n)
}

// pfEvent は、EIT[p/f] のセクションに載せる番組。name と description が空なら、短形式イベント記述子を載せない。
type pfEvent struct {
	eventID     uint16
	name        string
	description string
}

// pfPackets は、サービス sid の EIT[p/f] (自 TS) の放送中 present と次 following の番組のセクションを、TS のパケットにして返す。
// 長いセクションは、複数のパケットに分ける。
func pfPackets(t *testing.T, cc *byte, sid uint16, present, following pfEvent) []byte {
	t.Helper()

	out := []byte{}
	for sectionNumber, event := range []pfEvent{present, following} {
		descriptors := []byte{}
		if event.name != "" || event.description != "" {
			name, text := aribText(t, event.name), aribText(t, event.description)
			body := append([]byte("jpn"), lengthByte(len(name)))
			body = append(body, name...)
			body = append(body, lengthByte(len(text)))
			body = append(body, text...)
			descriptors = append(append(descriptors, 0x4D, lengthByte(len(body))), body...)
		}
		// transport_stream_id、original_network_id、segment_last_section_number、last_table_id、イベント。
		body := []byte{0x7F, 0xE0, 0x7F, 0xE0, 0x01, 0x4E}
		body = binary.BigEndian.AppendUint16(body, event.eventID)
		body = append(body, 0xE8, 0x1F, 0x23, 0x00, 0x00, 0x00, 0x30, 0x00)
		body = binary.BigEndian.AppendUint16(body, 0x8000|length12(len(descriptors)))
		body = append(body, descriptors...)
		section := []byte{0x4E, 0xF0, 0}
		section = binary.BigEndian.AppendUint16(section, sid)
		section = append(section, 0xC1, byte(sectionNumber), 1)
		section = append(section, body...)
		// section_length は、section_length の後ろから CRC までの長さ。
		binary.BigEndian.PutUint16(section[1:3], 0xF000|length12(len(section)-3+4))
		section = binary.BigEndian.AppendUint32(section, crcMPEG(section))
		data := append([]byte{0}, section...)
		for first := true; len(data) > 0; first = false {
			packet := make([]byte, 188)
			for i := range packet {
				packet[i] = 0xFF
			}
			packet[0], packet[1], packet[2], packet[3] = 0x47, 0x00, 0x12, 0x10|*cc
			if first {
				packet[1] |= 0x40
			}
			*cc = (*cc + 1) & 0x0F
			n := copy(packet[4:], data)
			data = data[n:]
			out = append(out, packet...)
		}
	}

	return out
}

// recheckPF は、予約した番組 (eventId 6) を次の番組に置き、その概要にだけ話数を載せた EIT[p/f] を返す。
func recheckPF(t *testing.T, sid uint16) []byte {
	t.Helper()

	return loopablePF(t, sid, pfEvent{eventID: 5, name: "", description: ""}, pfEvent{eventID: 6, name: recheckName, description: recheckDescription})
}

// loopablePF は、EIT[p/f] を、繰り返し流しても continuity_counter が続くよう、16 回並べて返す。
func loopablePF(t *testing.T, sid uint16, present, following pfEvent) []byte {
	t.Helper()

	cc := byte(0)
	chunks := make([][]byte, 0, 16)
	for range 16 {
		chunks = append(chunks, pfPackets(t, &cc, sid, present, following))
	}

	return slices.Concat(chunks...)
}

// reserveBundled は、地上波の service 番目のサービスの 13:30 の番組 (eventId 6) を、ルール予約にする。bundles の要素ごとに、
// 予約に一致したルールを作り、要素が true なら、同じ回を束ねるルール (#1498) にする。番組表と予約の番組名と概要は、話数のないものにする。
func (f *fixture) reserveBundled(t *testing.T, service int, bundles ...bool) uuid.UUID {
	t.Helper()

	reservationID := f.reserve(t, service)
	_, err := f.pool.Exec(t.Context(), "UPDATE programs SET name = $1, description = $2 WHERE service_id = $3 AND event_id = 6",
		recheckName, recheckSynopsis, grService(service))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE reservations SET kind = 'rule', name = $1, description = $2 WHERE id = $3",
		recheckName, recheckSynopsis, reservationID)
	require.NoError(t, err)
	for _, bundle := range bundles {
		ruleID := id.New()
		_, err = f.pool.Exec(t.Context(), `INSERT INTO rules (id, name, memo, enabled, conditions, settings, avoid_duplicates, follow_event_relay,
			bundle_same_episodes, created_at, updated_at)
			VALUES ($1, 'こたつ探偵', '', true, '{"version":1,"conditions":{}}', '{}', false, true, $2, $3, $3)`, ruleID, bundle, testNow())
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), "INSERT INTO reservation_rules (reservation_id, rule_id) VALUES ($1, $2)", reservationID, ruleID)
		require.NoError(t, err)
	}

	return reservationID
}

// recordHistory は、地上波の service 番目のサービスで、1 日前に録った回を、録画履歴に入れる。
func (f *fixture) recordHistory(t *testing.T, service int, description string) {
	t.Helper()

	start := programStart().Add(-24 * time.Hour)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO recording_history (id, service_id, title_key, name, description, start_at, end_at, created_at)
		VALUES ($1, $2, $3, $3, $4, $5, $6, $6)`, id.New(), grService(service), recheckName, description, start, start.Add(30*time.Minute))
	require.NoError(t, err)
}

// disabledState は、予約が有効かどうかと、Kotatsu が無効にした理由と、先に録った局。
type disabledState struct {
	enabled bool
	reason  *string
	service *int64
}

func (f *fixture) disabledState(t *testing.T, reservationID uuid.UUID) disabledState {
	t.Helper()

	var s disabledState
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT enabled, disabled_reason, disabled_service_id FROM reservations WHERE id = $1", reservationID).
		Scan(&s.enabled, &s.reason, &s.service))

	return s
}

// recorded は、予約の録画の記録があるかどうかを返す関数を返す。
func (f *fixture) recorded(t *testing.T, reservationID uuid.UUID) func() bool {
	t.Helper()

	return func() bool {
		_, found := f.recording(t, reservationID)

		return found
	}
}

// startedEvents は、録画の開始の通知の数を返す。
func (f *fixture) startedEvents(t *testing.T) int {
	t.Helper()

	n := 0
	for _, e := range f.events(t) {
		if e.Kind == string(domain.NotificationKindRecordingStarted) {
			n++
		}
	}

	return n
}

// newFakeTunerStream は、どのサービスのストリームでも data を繰り返し流す、偽のチューナーサーバーを返す。
func newFakeTunerStream(t *testing.T, data []byte) recording.StreamOpener {
	t.Helper()

	// null パケットは continuity_counter を数えないので、繰り返してもドロップにならない。
	padded := slices.Concat(data, slices.Repeat(nullPacket(), 64))
	server := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), padded)))
	t.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(t, err)

	return client
}

// TestRecorderRechecksEpisodeFromPresentFollowing は、同じ回を束ねるルールの予約の録画で、録り始めに EIT[p/f] の概要にだけ載る話数を読み、
// 先に録った同じ回があれば録画をやめ、無ければ録り続けることを確かめる (#1500)。偽のチューナーサーバーで、EIT[p/f] を流す。
//   - やめた録画は、記録もファイルも消し、予約を理由と先に録った局を付けて無効にする。録画の開始は通知しない。
//   - 録り続ける録画は、番組表の番組と録画の概要を EIT[p/f] のもので書き直し、待つ上限が来てから、録画の開始を通知する。
//   - 同じ回を束ねないルールの予約は、確かめずに録り、すぐに録画の開始を通知する。
//
// 偽のチューナーサーバーは同じ TS を繰り返すので、予約した番組は次の番組のまま放送中にならない。録り続けるのを確かめたら、録画を止める。
func TestRecorderRechecksEpisodeFromPresentFollowing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// history は、別の局で 1 日前に録った回の概要。空なら録画履歴に入れない。
		history string
		// bundles は、予約に一致したルールが、同じ回を束ねるかどうか。
		bundles []bool
		// wantStopped は、録画をやめるかどうか。
		wantStopped bool
	}{
		{name: "先に録った同じ回があれば、やめる", history: recheckDescription, bundles: []bool{true}, wantStopped: true},
		{name: "先に録った回が別の回なら、録る", history: recheckOther, bundles: []bool{true}, wantStopped: false},
		{name: "先に録った回が無ければ、録る", history: "", bundles: []bool{true}, wantStopped: false},
		{name: "同じ回を束ねないルールの予約は、確かめずに録る", history: recheckDescription, bundles: []bool{false}, wantStopped: false},
		{
			name: "束ねない有効なルールも一致すれば、そのルールが受け持つので、確かめずに録る", history: recheckDescription,
			bundles: []bool{true, false}, wantStopped: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserveBundled(t, 0, tt.bundles...)
			if tt.history != "" {
				f.recordHistory(t, 1, tt.history)
			}
			recorder, dir := f.newRecorder(t, newFakeTunerStream(t, recheckPF(t, 0x0400)), nil)
			cancel, done := f.session(t, recorder, reservationID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))

			if tt.wantStopped {
				require.NoError(t, wait(t, done))
				_, found := f.recording(t, reservationID)
				assert.False(t, found, "the recording is deleted")
				assert.Empty(t, segmentFiles(t, dir), "the segments are removed")
				assert.Equal(t, disabledState{enabled: false, reason: new("recorded_episode"), service: new(grService(1))}, f.disabledState(t, reservationID))
				assert.Zero(t, f.startedEvents(t), "the start of the abandoned recording is not notified")

				return
			}
			require.Eventually(t, func() bool { return len(f.files(t, reservationID)) == 1 }, waitTimeout, 10*time.Millisecond)
			if !slices.Contains(tt.bundles, false) {
				// EIT[p/f] で書き直すまで待つ。書き直しても、待つ上限が来るまでは、録画の開始を通知しない。
				require.Eventually(t, func() bool {
					var description string
					require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT description FROM recordings WHERE reservation_id = $1", reservationID).Scan(&description))

					return description == recheckDescription
				}, waitTimeout, 10*time.Millisecond)
				var programDescription string
				require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT description FROM programs WHERE service_id = $1 AND event_id = 6", grService(0)).
					Scan(&programDescription))
				assert.Equal(t, recheckDescription, programDescription)
				assert.Zero(t, f.startedEvents(t))
				f.clock.Advance(recording.EpisodeRecheckTimeout)
			}
			require.Eventually(t, func() bool { return f.startedEvents(t) == 1 }, waitTimeout, 10*time.Millisecond)
			cancel(errStopForTest)
			require.ErrorIs(t, wait(t, done), errStopForTest)
			got, found := f.recording(t, reservationID)
			require.True(t, found)
			assert.Equal(t, string(domain.RecordingStateRecording), got.state)
			assert.Len(t, f.files(t, reservationID), 1)
			assert.True(t, f.disabledState(t, reservationID).enabled)
			assert.Equal(t, 1, f.startedEvents(t))
		})
	}
}

// TestRecorderKeepsRecordingWithoutPresentFollowing は、予約した番組が EIT[p/f] に出ないまま、待つ上限が過ぎたら、
// 番組表の情報のまま録り続け、録画の開始を通知することを確かめる (#1500)。待っている間に書いた分も消さない。
func TestRecorderKeepsRecordingWithoutPresentFollowing(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserveBundled(t, 0, true)
	f.recordHistory(t, 1, recheckDescription)
	// EIT[p/f] には、予約した番組 (eventId 6) が出ない。
	pf := loopablePF(t, 0x0400, pfEvent{eventID: 4, name: "", description: ""}, pfEvent{eventID: 5, name: recheckName, description: recheckDescription})
	recorder, _ := f.newRecorder(t, newFakeTunerStream(t, pf), nil)
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.Eventually(t, func() bool { return len(f.files(t, reservationID)) == 1 }, waitTimeout, 10*time.Millisecond)
	assert.Zero(t, f.startedEvents(t))

	// 待つ上限の手前では、まだ通知しない。
	f.clock.Advance(recording.EpisodeRecheckTimeout - time.Second)
	time.Sleep(100 * time.Millisecond)
	assert.Zero(t, f.startedEvents(t))
	f.clock.Advance(time.Second)
	require.Eventually(t, func() bool { return f.startedEvents(t) == 1 }, waitTimeout, 10*time.Millisecond)

	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))
	got, found := f.recording(t, reservationID)
	require.True(t, found)
	assert.Equal(t, string(domain.RecordingStateCompleted), got.state)
	assert.True(t, f.disabledState(t, reservationID).enabled)
	var description string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT description FROM recordings WHERE reservation_id = $1", reservationID).Scan(&description))
	assert.Equal(t, recheckSynopsis, description, "the EPG information is kept")
}

// TestRecorderStopsOnlyLaterOfSimultaneousEpisodes は、同時刻に始まる 2 局の同じ回の録画が、両方は止まらず、
// 後に録り始めた方だけが止まることを確かめる (#1500)。EIT[p/f] が届く順は、どちらが先でもよい。
// 後の録画に先に届いたときは、先の録画がまだ書き直していないので、待つ上限まで確かめ直して止まる。
func TestRecorderStopsOnlyLaterOfSimultaneousEpisodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// laterFirst は、後に録り始めた録画に、EIT[p/f] を先に届けるかどうか。
		laterFirst bool
	}{
		{name: "先の録画に先に届く", laterFirst: false},
		{name: "後の録画に先に届く", laterFirst: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			earlierID := f.reserveBundled(t, 0, true)
			laterID := f.reserveBundled(t, 1, true)
			earlierStream, laterStream := newSteppedStream(), newSteppedStream()
			earlierLogs := &messageRecorder{mu: sync.Mutex{}, messages: []string{}}
			laterLogs := &messageRecorder{mu: sync.Mutex{}, messages: []string{}}
			earlierRecorder, _ := f.newRecorderWithLogger(t, earlierStream, nil, nil, slog.New(earlierLogs))
			laterRecorder, _ := f.newRecorderWithLogger(t, laterStream, nil, nil, slog.New(laterLogs))
			stopEarlier, earlierDone := f.session(t, earlierRecorder, earlierID)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			// 先の録画の記録ができてから、後の録画を始める。
			require.Eventually(t, f.recorded(t, earlierID), waitTimeout, 10*time.Millisecond)
			_, laterDone := f.session(t, laterRecorder, laterID)
			require.Eventually(t, f.recorded(t, laterID), waitTimeout, 10*time.Millisecond)

			rechecked := func(logs *messageRecorder) func() bool {
				return func() bool { return slices.Contains(logs.list(), "rechecked the episode; it has not been recorded") }
			}
			if tt.laterFirst {
				laterStream.next(t, recheckPF(t, 0x0408))
				// 先の録画はまだ書き直していないので、同じ回とは分からない。
				require.Eventually(t, rechecked(laterLogs), waitTimeout, 10*time.Millisecond)
				earlierStream.next(t, recheckPF(t, 0x0400))
				require.Eventually(t, rechecked(earlierLogs), waitTimeout, 10*time.Millisecond)
				_, found := f.recording(t, laterID)
				require.True(t, found, "the later recording waits until the timeout")
				f.clock.Advance(recording.EpisodeRecheckTimeout)
			} else {
				earlierStream.next(t, recheckPF(t, 0x0400))
				require.Eventually(t, rechecked(earlierLogs), waitTimeout, 10*time.Millisecond)
				laterStream.next(t, recheckPF(t, 0x0408))
			}
			require.NoError(t, wait(t, laterDone))
			if !tt.laterFirst {
				f.clock.Advance(recording.EpisodeRecheckTimeout)
			}
			require.Eventually(t, func() bool { return f.startedEvents(t) == 1 }, waitTimeout, 10*time.Millisecond)
			_, found := f.recording(t, laterID)
			assert.False(t, found, "the later recording stops")
			assert.Equal(t, disabledState{enabled: false, reason: new("recorded_episode"), service: new(grService(0))}, f.disabledState(t, laterID))

			// 先の録画は録り続ける。確かめたら、止める。
			assert.Equal(t, 1, f.startedEvents(t))
			stopEarlier(errStopForTest)
			require.ErrorIs(t, wait(t, earlierDone), errStopForTest)
			got, found := f.recording(t, earlierID)
			require.True(t, found, "the earlier recording keeps recording")
			assert.Equal(t, string(domain.RecordingStateRecording), got.state)
			assert.True(t, f.disabledState(t, earlierID).enabled)
		})
	}
}

// TestStoppedReservationIsNotRecordedAgain は、同じ回と分かって録画をやめた予約を、スケジューラがもう一度録り始めないことと、
// 人が有効に戻せば、理由を消し、もう確かめ直さずに録ることを確かめる (#1500)。
func TestStoppedReservationIsNotRecordedAgain(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserveBundled(t, 0, true)
	f.recordHistory(t, 1, recheckDescription)
	recorder, _ := f.newRecorder(t, newFakeTunerStream(t, recheckPF(t, 0x0400)), nil)
	// 割り振りを書いて、録画を始める予定の時刻まで進める。
	f.clock.Advance(plannedStart().Add(-time.Minute).Sub(f.clock.Now()))
	f.schedule(t)
	require.Contains(t, f.assigned(t), reservationID)
	// mikan と同じく、割り振りを取ってから録る。
	q := sqlc.New(f.pool)
	holder := id.New()
	acquired, err := q.AcquireRecordingAssignment(t.Context(), sqlc.AcquireRecordingAssignmentParams{
		Holder: &holder, ExpiresAt: new(f.clock.Now().Add(time.Hour)), Now: f.clock.Now(), Active: []uuid.UUID{},
	})
	require.NoError(t, err)
	require.Equal(t, reservationID, acquired.ReservationID)
	assignment := &recording.Assignment{
		ID: acquired.ID, ReservationID: reservationID, Priority: recording.Priority, Generation: acquired.Generation, HandedOver: false,
		PreviousLeaseExpiresAt: nil,
	}
	_, done := f.sessionWith(t, recorder, assignment)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))
	require.False(t, f.disabledState(t, reservationID).enabled)
	// 録画の行を消したので、Worker と同じく、録画を始めなかった割り振りとして消せる (#523)。
	deleted, err := q.DeleteUnrecordedRecordingAssignment(t.Context(), sqlc.DeleteUnrecordedRecordingAssignmentParams{
		ID: acquired.ID, Generation: acquired.Generation, Holder: &holder,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	// 割り当てとスケジューラは、無効にした予約を録り始めない。
	_, err = f.assigner.RunOnce(t.Context())
	require.NoError(t, err)
	f.schedule(t)
	assert.NotContains(t, f.assigned(t), reservationID)

	// 人が有効に戻すと、理由を消し、また割り振る。録り始めに確かめ直さないので、録画の開始をすぐに通知する。
	_, err = f.reservations.Update(t.Context(), f.pool, audit.System("test"), reservationID, &reserve.Update{Enabled: new(true)})
	require.NoError(t, err)
	assert.Equal(t, disabledState{enabled: true, reason: nil, service: nil}, f.disabledState(t, reservationID))
	_, err = f.assigner.RunOnce(t.Context())
	require.NoError(t, err)
	f.schedule(t)
	assert.Contains(t, f.assigned(t), reservationID)
	stop, done := f.session(t, recorder, reservationID)
	require.Eventually(t, func() bool { return f.startedEvents(t) == 1 }, waitTimeout, 10*time.Millisecond)
	require.Eventually(t, func() bool { return len(f.files(t, reservationID)) == 1 }, waitTimeout, 10*time.Millisecond)
	stop(errStopForTest)
	require.ErrorIs(t, wait(t, done), errStopForTest)
	got, found := f.recording(t, reservationID)
	require.True(t, found)
	assert.Equal(t, string(domain.RecordingStateRecording), got.state)
}

// TestRecorderRecordsHistoryFromRewrittenName は、EIT[p/f] で番組名を書き直した録画を録り終えたら、録画履歴の番組名のキーと、
// 前の回を録らない (#1499) ときに引くシリーズ名を、書き直した後の番組名から作ることを確かめる (#1500)。
// 数日前の番組表では、番組名が枠の名前だけの局を真似る。
func TestRecorderRecordsHistoryFromRewrittenName(t *testing.T) {
	t.Parallel()

	const placeholder = "アニメ枠"
	f := newFixture(t)
	reservationID := f.reserveBundled(t, 0, true)
	_, err := f.pool.Exec(t.Context(), "UPDATE programs SET name = $1 WHERE service_id = $2 AND event_id = 6", placeholder, grService(0))
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), "UPDATE reservations SET name = $1 WHERE id = $2", placeholder, reservationID)
	require.NoError(t, err)
	stream := newSteppedStream()
	logs := &messageRecorder{mu: sync.Mutex{}, messages: []string{}}
	recorder, _ := f.newRecorderWithLogger(t, stream, nil, nil, slog.New(logs))
	_, done := f.session(t, recorder, reservationID)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))

	// 予約した番組が放送中になり、EIT[p/f] に作品名が載る。待つ上限まで進めて、確かめ終える。
	cc := byte(0)
	stream.next(t, pfPackets(t, &cc, 0x0400, pfEvent{eventID: 6, name: recheckName, description: recheckDescription},
		pfEvent{eventID: 7, name: "", description: ""}))
	require.Eventually(t, func() bool { return f.recordingName(t, reservationID) == recheckName }, waitTimeout, 10*time.Millisecond)
	f.clock.Advance(recording.EpisodeRecheckTimeout)
	require.Eventually(t, func() bool { return f.startedEvents(t) == 1 }, waitTimeout, 10*time.Millisecond)
	// 次の番組に切り替わったら、予定の終わりで止める。
	stream.next(t, pfPackets(t, &cc, 0x0400, pfEvent{eventID: 7, name: "", description: ""}, pfEvent{eventID: 8, name: "", description: ""}))
	require.Eventually(t, func() bool { return slices.Contains(logs.list(), "the program left the air; recording the end margin") },
		waitTimeout, 10*time.Millisecond)
	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))

	var name, titleKey string
	var series *string
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT name, title_key, series FROM recording_history").Scan(&name, &titleKey, &series))
	assert.Equal(t, recheckName, name)
	assert.Equal(t, rule.TitleKey(recheckName), titleKey)
	require.NotNil(t, series)
	assert.Equal(t, episode.SeriesOf(recheckName), *series)
	assert.NotEqual(t, episode.SeriesOf(placeholder), *series)
}

// recordingName は、予約の録画の番組名を返す。録画がなければ空にする。
func (f *fixture) recordingName(t *testing.T, reservationID uuid.UUID) string {
	t.Helper()

	var name string
	if err := f.pool.QueryRow(t.Context(), "SELECT name FROM recordings WHERE reservation_id = $1", reservationID).Scan(&name); err != nil {
		return ""
	}

	return name
}

// castHeading は、番組の詳細の見出し。
const castHeading = "出演者"

// TestChangedExtended は、EIT[p/f] の番組の詳細で録画の詳細を書き直すのが、正規化して違うときだけであることを確かめる (#1646)。
// 書き方の違い (全角と半角、波ダッシュ) だけでは書き直さず、EIT[p/f] に詳細がなければ前の値を残す。
func TestChangedExtended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		items   []*ts.ExtendedItem
		current string
		want    string
	}{
		{name: "no extended in EIT", items: nil, current: `{"出演者":"こたつ"}`, want: ""},
		{name: "same text", items: []*ts.ExtendedItem{{Heading: castHeading, Text: "こたつ"}}, current: `{"出演者":"こたつ"}`, want: ""},
		{name: "only width differs", items: []*ts.ExtendedItem{{Heading: castHeading, Text: "ＡＢＣ"}}, current: `{"出演者":"ABC"}`, want: ""},
		{name: "only wave dash differs", items: []*ts.ExtendedItem{{Heading: "内容", Text: "前編〜後編"}}, current: `{"内容":"前編～後編"}`, want: ""},
		{
			name:    "changed",
			items:   []*ts.ExtendedItem{{Heading: castHeading, Text: "こたつ"}, {Heading: castHeading, Text: "ほか"}, {Heading: "内容", Text: "第1話"}},
			current: `{}`, want: `{"出演者":"こたつほか","内容":"第1話"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := recording.ChangedExtended(tt.items, []byte(tt.current))
			require.NoError(t, err)
			if tt.want == "" {
				assert.Nil(t, got)

				return
			}
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}
