package recording_test

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/reserve"
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
	out := make([]byte, 0, 2*188)
	cc := byte(0)
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
		packet[0], packet[1], packet[2], packet[3] = 0x47, 0x40, 0x12, 0x10|cc
		packet[4] = 0 // pointer_field
		copy(packet[5:], section)
		cc = (cc + 1) & 0x0F
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
