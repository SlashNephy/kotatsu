package recording_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/usecase/recording"
)

// errStopForTest は、テストで録画を止めるときの理由。
var errStopForTest = errors.New("stopped by the test")

// crcMPEG は、MPEG-2 の PSI の CRC32 を求める。
func crcMPEG(b []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, v := range b {
		crc ^= uint32(v) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04C11DB7
			} else {
				crc <<= 1
			}
		}
	}

	return crc
}

// eitPFPackets は、サービス sid の EIT[p/f] (自 TS) の放送中 present と次 following の番組のセクションを、TS のパケットにして返す。
func eitPFPackets(sid, present, following uint16) []byte {
	cc := byte(0)

	return eitPFPacketsFrom(&cc, sid, present, following)
}

// eitPFPacketsFrom は、eitPFPackets を、continuity_counter を cc から続けて返す。同じストリームで EIT[p/f] を送り直すときに使う。
func eitPFPacketsFrom(cc *byte, sid, present, following uint16) []byte {
	out := make([]byte, 0, 2*188)
	for sectionNumber, eventID := range []uint16{present, following} {
		// transport_stream_id、original_network_id、segment_last_section_number、last_table_id、イベント (eventId、
		// start_time、duration、記述子の長さ 0)。
		body := []byte{0x7F, 0xE0, 0x7F, 0xE0, 0x01, 0x4E}
		body = binary.BigEndian.AppendUint16(body, eventID)
		body = append(body, 0xE8, 0x1F, 0x23, 0x00, 0x00, 0x00, 0x30, 0x00, 0x80, 0x00)
		// section_length は、ヘッダーの残り (5 バイト)、本体 (18 バイト)、CRC (4 バイト)。
		const sectionLength = 5 + 18 + 4
		section := []byte{0x4E, 0xF0, sectionLength}
		section = binary.BigEndian.AppendUint16(section, sid)
		section = append(section, 0xC1, byte(sectionNumber), 1)
		section = append(section, body...)
		section = binary.BigEndian.AppendUint32(section, crcMPEG(section))
		packet := make([]byte, 188)
		for i := range packet {
			packet[i] = 0xFF
		}
		packet[0], packet[1], packet[2], packet[3] = 0x47, 0x40, 0x12, 0x10|*cc
		packet[4] = 0 // pointer_field
		copy(packet[5:], section)
		*cc = (*cc + 1) & 0x0F
		out = append(out, packet...)
	}

	return out
}

// TestRecorderMarksNotBroadcast は、予定の時間に、予約した番組 (eventId 6) が EIT[p/f] の放送中にも次の番組にも出てこなければ、
// 予定の終わりで止め、録った TS は残して、放送されなかったことと、その時間に放送されていた番組を記録することを確かめる (#389)。
func TestRecorderMarksNotBroadcast(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	// 予約した番組と時間の重なる、実際に放送された番組 (eventId 99) を番組表に入れる。
	_, err := f.pool.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name,
		description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		VALUES ($1, 99, 0, $2, $3, false, true, '臨時ニュース', '', '臨時ニュース', '', '{}', '[]', '[]', $2, $2)`,
		grService(0), programStart(), programStart().Add(30*time.Minute))
	require.NoError(t, err)
	stream := newDrainedStream(eitPFPackets(0x0400, 99, 100))
	recorder, _ := f.newRecorder(t, stream, nil)
	_, done := f.session(t, recorder, reservationID)

	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	stream.wait(t)
	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))

	var notBroadcast bool
	var aired *int64
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT not_broadcast, aired_event_id FROM recordings WHERE reservation_id = $1", reservationID).
		Scan(&notBroadcast, &aired))
	assert.True(t, notBroadcast)
	assert.Equal(t, new(int64(99)), aired)
	assert.Len(t, f.files(t, reservationID), 1, "the recorded TS is kept")
	// 完了の通知で、放送されなかったことを知らせる。
	var flagged bool
	var airedName *string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT coalesce((payload->>'notBroadcast')::boolean, false), payload->>'airedName' FROM notification_events WHERE kind = 'recording_completed'").
		Scan(&flagged, &airedName))
	assert.True(t, flagged)
	assert.Equal(t, new("臨時ニュース"), airedName)
}

// TestRecorderMarksNotBroadcastWhenFollowingUntilLimit は、予約した番組 (eventId 6) が EIT[p/f] の次の番組のまま
// 放送中にならなければ、延長の上限まで待って止め、放送されなかったことを記録することを確かめる (#640)。
// 野球中継の延長で大幅に繰り下がり、そのまま放送されなかった場合に当たる。
func TestRecorderMarksNotBroadcastWhenFollowingUntilLimit(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	stream := newDrainedStream(eitPFPackets(0x0400, 99, 6))
	recorder, _ := f.newRecorder(t, stream, nil)
	_, done := f.session(t, recorder, reservationID)

	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	stream.wait(t)

	// 予定の終わりを過ぎても、次の番組にいるので待つ (繰り下げ)。
	plannedEnd := programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second)
	f.clock.Advance(plannedEnd.Add(time.Hour).Sub(f.clock.Now()))
	select {
	case err := <-done:
		require.FailNow(t, "the recording stopped while the program was following", "%v", err)
	case <-time.After(200 * time.Millisecond):
	}

	// 上限まで次の番組のままなら、上限で止め、放送されなかったと記録する。
	f.clock.Advance(plannedEnd.Add(recording.MaxExtension).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))
	var notBroadcast bool
	var aired *int64
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT not_broadcast, aired_event_id FROM recordings WHERE reservation_id = $1", reservationID).
		Scan(&notBroadcast, &aired))
	assert.True(t, notBroadcast)
	assert.Equal(t, new(int64(99)), aired)
	assert.Len(t, f.files(t, reservationID), 1, "the recorded TS is kept")
}

// TestRecorderExtendsWhileOnAir は、予約した番組が EIT[p/f] の放送中になったら、その時刻を記録し、
// 予定の終わりを過ぎても放送中のあいだは録り続けることを確かめる (#389)。
func TestRecorderExtendsWhileOnAir(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	stream := newDrainedStream(eitPFPackets(0x0400, 6, 7))
	recorder, _ := f.newRecorder(t, stream, nil)
	stop, done := f.session(t, recorder, reservationID)

	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	stream.wait(t)
	require.Eventually(t, func() bool {
		var started *time.Time
		err := f.pool.QueryRow(t.Context(), "SELECT broadcast_started_at FROM recordings WHERE reservation_id = $1", reservationID).Scan(&started)

		return err == nil && started != nil
	}, waitTimeout, 10*time.Millisecond)

	// 予定の終わりを過ぎても、放送中なので止めない。
	f.clock.Advance(programStart().Add(30*time.Minute + domain.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	select {
	case err := <-done:
		require.FailNow(t, "the recording stopped while the program was on air", "%v", err)
	case <-time.After(200 * time.Millisecond):
	}
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, "recording", got.state)
	stop(errStopForTest)
	<-done
}

// errStreamOffline は、steppedStream がつながらない間に、開こうとしたときのエラー。
var errStreamOffline = errors.New("the stream is offline")

// steppedStream は、chunks を 1 つずつ、next で送るたびに書くストリーム。録画の途中で EIT[p/f] を変えるときに使う。
// cut でストリームを切り、online で開けるようにするまで、開き直せなくする。
type steppedStream struct {
	chunks  chan []byte
	written chan struct{}
	cuts    chan struct{}
	offline atomic.Bool
}

func newSteppedStream() *steppedStream {
	return &steppedStream{chunks: make(chan []byte), written: make(chan struct{}), cuts: make(chan struct{}), offline: atomic.Bool{}}
}

func (s *steppedStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	if s.offline.Load() {
		return nil, errStreamOffline
	}
	reader, writer := io.Pipe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = writer.CloseWithError(ctx.Err())

				return
			case <-s.cuts:
				_ = writer.Close()

				return
			case chunk := <-s.chunks:
				// io.Pipe の Write は、読む側がすべて読むまで戻らない。
				if _, err := writer.Write(chunk); err != nil {
					return
				}
				s.written <- struct{}{}
			}
		}
	}()

	return reader, nil
}

// next は、chunk を書き、録画が読み終えるまで待つ。
func (s *steppedStream) next(t *testing.T, chunk []byte) {
	t.Helper()

	select {
	case s.chunks <- chunk:
	case <-time.After(waitTimeout):
		require.FailNow(t, "the recorder did not open the stream")
	}
	select {
	case <-s.written:
	case <-time.After(waitTimeout):
		require.FailNow(t, "the recorder did not read the stream")
	}
}

// cut は、今のストリームを切り、online を呼ぶまで開き直せなくする。
func (s *steppedStream) cut(t *testing.T) {
	t.Helper()

	s.offline.Store(true)
	select {
	case s.cuts <- struct{}{}:
	case <-time.After(waitTimeout):
		require.FailNow(t, "the recorder did not open the stream")
	}
}

// online は、ストリームを開き直せるようにする。
func (s *steppedStream) online() {
	s.offline.Store(false)
}

var _ recording.StreamOpener = new(steppedStream)

// nullPacket は、中身のない TS のパケット (PID 0x1FFF) を返す。
func nullPacket() []byte {
	packet := make([]byte, 188)
	for i := range packet {
		packet[i] = 0xFF
	}
	packet[0], packet[1], packet[2], packet[3] = 0x47, 0x1F, 0xFF, 0x10

	return packet
}

// TestRecorderFollowsBroadcastAcrossReconnect は、ストリームが切れている間に放送中の番組が変わっても、つなぎ直した後の
// EIT[p/f] で正しく判断することを確かめる (#638)。つなぎ直すと解析器は作り直すが、放送の状態は引き継ぐ。
func TestRecorderFollowsBroadcastAcrossReconnect(t *testing.T) {
	t.Parallel()

	programEnd := programStart().Add(30 * time.Minute)
	tests := []struct {
		name string
		// afterReconnect は、つなぎ直した後に流す EIT[p/f] の放送中と次の番組。nil なら、EIT[p/f] を流さない。
		afterReconnect *[2]uint16
		wantEnded      bool
	}{
		{name: "切断中に予約した番組が終わり、つなぎ直した後に次の番組が放送中なら、後ろのマージンを録って止める", afterReconnect: &[2]uint16{7, 8}, wantEnded: true},
		{name: "切断中に延長が始まり、つなぎ直した後も予約した番組が放送中なら、録り続ける", afterReconnect: &[2]uint16{6, 7}, wantEnded: false},
		{name: "つなぎ直した直後で、EIT[p/f] をまだ読んでいなければ、終わったとはしない", afterReconnect: nil, wantEnded: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			reservationID := f.reserve(t, 0)
			stream := newSteppedStream()
			recorder, _ := f.newRecorder(t, stream, nil)
			stop, done := f.session(t, recorder, reservationID)
			broadcast := func() (*time.Time, *time.Time) {
				var started, ended *time.Time
				err := f.pool.QueryRow(t.Context(), "SELECT broadcast_started_at, broadcast_ended_at FROM recordings WHERE reservation_id = $1",
					reservationID).Scan(&started, &ended)
				if err != nil {
					return nil, nil
				}

				return started, ended
			}

			cc := byte(0)
			f.clock.Advance(plannedStart().Sub(f.clock.Now()))
			stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 6, 7))
			require.Eventually(t, func() bool {
				started, _ := broadcast()

				return started != nil
			}, waitTimeout, 10*time.Millisecond)

			// 予定の終わりの前に切れ、終わりを過ぎるまでつながらない。最後に読んだ EIT[p/f] では予約した番組が放送中なので、止めずに延ばす。
			stream.cut(t)
			require.Eventually(t, func() bool {
				for _, file := range f.files(t, reservationID) {
					if !file.closed {
						return false
					}
				}

				return true
			}, waitTimeout, 10*time.Millisecond)
			f.clock.Advance(programEnd.Add(3 * time.Minute).Sub(f.clock.Now()))
			select {
			case err := <-done:
				require.FailNow(t, "the recording stopped while the stream was cut", "%v", err)
			case <-time.After(200 * time.Millisecond):
			}

			stream.online()
			reconnectedAt := f.clock.Now()
			if tt.afterReconnect != nil {
				stream.next(t, eitPFPacketsFrom(&cc, 0x0400, tt.afterReconnect[0], tt.afterReconnect[1]))
			} else {
				stream.next(t, nullPacket())
			}
			require.Equal(t, 2, f.segments(t, reservationID))

			if tt.wantEnded {
				require.Eventually(t, func() bool {
					_, ended := broadcast()

					return ended != nil
				}, waitTimeout, 10*time.Millisecond)
				_, ended := broadcast()
				assert.True(t, ended.Equal(reconnectedAt), "the broadcast ended at %v", ended)
				f.clock.Advance(domain.DefaultMarginSeconds * time.Second)
				require.NoError(t, wait(t, done))
				got, _ := f.recording(t, reservationID)
				assert.Equal(t, string(recording.StatePartial), got.state, "the cut is recorded as a gap")

				return
			}

			select {
			case err := <-done:
				require.FailNow(t, "the recording stopped after reconnecting", "%v", err)
			case <-time.After(200 * time.Millisecond):
			}
			_, ended := broadcast()
			assert.Nil(t, ended)
			got, _ := f.recording(t, reservationID)
			assert.Equal(t, string(recording.StateRecording), got.state)
			stop(errStopForTest)
			<-done
		})
	}
}

// TestRecorderStopsWhenLeftAirWhileEndIsUndecided は、EPG で終了時刻が未定のままでも、EIT[p/f] の放送中が次の番組に
// 切り替わったら、延長せずに後ろのマージンを録って止めることを確かめる。2026-10-16 の緊急地震速報で、放送局が放送中の番組の
// 長さを未定に、次の番組を開始も長さも未定の臨時の番組 (eventId 65522) にした後、予定どおりに次の番組へ切り替えた事例を再現する。
// 延長しないのに延長のログを出さないことも確かめる (#643)。
func TestRecorderStopsWhenLeftAirWhileEndIsUndecided(t *testing.T) {
	t.Parallel()

	const emergencyEventID = 65522
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	stream := newSteppedStream()
	logs := new(messageRecorder)
	recorder, _ := f.newRecorderWithLogger(t, stream, nil, nil, slog.New(logs))
	_, done := f.session(t, recorder, reservationID)
	broadcast := func() (*time.Time, *time.Time) {
		var started, ended *time.Time
		err := f.pool.QueryRow(t.Context(), "SELECT broadcast_started_at, broadcast_ended_at FROM recordings WHERE reservation_id = $1",
			reservationID).Scan(&started, &ended)
		if err != nil {
			return nil, nil
		}

		return started, ended
	}

	cc := byte(0)
	f.clock.Advance(plannedStart().Sub(f.clock.Now()))
	stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 6, 7))
	require.Eventually(t, func() bool {
		started, _ := broadcast()

		return started != nil
	}, waitTimeout, 10*time.Millisecond)

	// 番組の途中で、終了時刻が未定になった。EPG にも、EIT[p/f] の次の番組にも表れる。
	f.clock.Advance(programStart().Add(10 * time.Minute).Sub(f.clock.Now()))
	_, err := f.pool.Exec(t.Context(), "UPDATE programs SET end_undecided = true WHERE service_id = $1 AND event_id = 6", grService(0))
	require.NoError(t, err)
	stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 6, emergencyEventID))

	// 予定どおりの時刻に、放送中が次の番組に切り替わった。EPG は、終了時刻が未定のまま。
	programEnd := programStart().Add(30 * time.Minute)
	f.clock.Advance(programEnd.Sub(f.clock.Now()))
	stream.next(t, eitPFPacketsFrom(&cc, 0x0400, 7, emergencyEventID))
	require.Eventually(t, func() bool {
		_, ended := broadcast()

		return ended != nil
	}, waitTimeout, 10*time.Millisecond)

	// EPG では未定でも、5 分の延長はせず、切り替わってから後ろのマージンで止める。
	f.clock.Advance(domain.DefaultMarginSeconds * time.Second)
	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompleted), got.state)
	_, ended := broadcast()
	require.NotNil(t, ended)
	assert.True(t, ended.Equal(programEnd), "the broadcast ended at %v", ended)
	assert.Contains(t, logs.list(), "the program left the air; recording the end margin")
	assert.NotContains(t, logs.list(), "the program end is undecided; extending the recording")
}
