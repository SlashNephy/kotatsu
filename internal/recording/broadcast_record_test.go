package recording_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/recording"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
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
	f.clock.Advance(programStart().Add(30*time.Minute + reserve.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
	require.NoError(t, wait(t, done))

	var notBroadcast bool
	var aired *int64
	require.NoError(t, f.pool.QueryRow(t.Context(), "SELECT not_broadcast, aired_event_id FROM recordings WHERE reservation_id = $1", reservationID).
		Scan(&notBroadcast, &aired))
	assert.True(t, notBroadcast)
	assert.Equal(t, new(int64(99)), aired)
	assert.Len(t, f.files(t, reservationID), 2, "the recorded TS is kept")
	// 完了の通知で、放送されなかったことを知らせる。
	var flagged bool
	var airedName *string
	require.NoError(t, f.pool.QueryRow(t.Context(),
		"SELECT coalesce((payload->>'notBroadcast')::boolean, false), payload->>'airedName' FROM notification_events WHERE kind = 'recording_completed'").
		Scan(&flagged, &airedName))
	assert.True(t, flagged)
	assert.Equal(t, new("臨時ニュース"), airedName)
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
	f.clock.Advance(programStart().Add(30*time.Minute + reserve.DefaultMarginSeconds*time.Second).Sub(f.clock.Now()))
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

// steppedStream は、chunks を 1 つずつ、next で送るたびに書くストリーム。録画の途中で EIT[p/f] を変えるときに使う。
type steppedStream struct {
	chunks  chan []byte
	written chan struct{}
}

func newSteppedStream() *steppedStream {
	return &steppedStream{chunks: make(chan []byte), written: make(chan struct{})}
}

func (s *steppedStream) OpenServiceStream(ctx context.Context, _ int64, _ *mirakurun.StreamOptions) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = writer.CloseWithError(ctx.Err())

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

var _ recording.StreamOpener = new(steppedStream)

// TestRecorderStopsWhenLeftAirWhileEndIsUndecided は、EPG で終了時刻が未定のままでも、EIT[p/f] の放送中が次の番組に
// 切り替わったら、延長せずに後ろのマージンを録って止めることを確かめる。2026-10-16 の緊急地震速報で、放送局が放送中の番組の
// 長さを未定に、次の番組を開始も長さも未定の臨時の番組 (eventId 65522) にした後、予定どおりに次の番組へ切り替えた事例を再現する。
func TestRecorderStopsWhenLeftAirWhileEndIsUndecided(t *testing.T) {
	t.Parallel()

	const emergencyEventID = 65522
	f := newFixture(t)
	reservationID := f.reserve(t, 0)
	stream := newSteppedStream()
	recorder, _ := f.newRecorder(t, stream, nil)
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
	f.clock.Advance(reserve.DefaultMarginSeconds * time.Second)
	require.NoError(t, wait(t, done))
	got, _ := f.recording(t, reservationID)
	assert.Equal(t, string(recording.StateCompleted), got.state)
	_, ended := broadcast()
	require.NotNil(t, ended)
	assert.True(t, ended.Equal(programEnd), "the broadcast ended at %v", ended)
}
