package edcb_test

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// notification は、GET_STATUS_NOTIFY2 の応答 (版と NOTIFY_SRV_INFO) を組む。
func notification(id, count uint32) reply {
	body := payload{}.u32(id)
	for _, v := range []uint16{2026, 11, 5, 13, 21, 0, 0, 0} {
		body = body.u16(v)
	}
	body = body.u32(0).u32(0).u32(count).str("").str("").str("")

	return reply{status: 1, payload: payload{}.u16(5).append(sized(body)), stream: nil, hold: false, drop: false}
}

func TestTunerServerEvents(t *testing.T) {
	t.Parallel()

	// 最後の通知の巡回カウンタ (5) より後の通知を待つ。予約の変化 (2) は読み飛ばし、EPG の読み込み (1) をイベントにする。
	s := newFakeServer(t, map[uint32]reply{2200: {status: 0, payload: nil, stream: nil, hold: true, drop: false}})
	s.queued[2200] = []reply{notification(edcb.NotifyUpdateSrvStatus, 5), notification(edcb.NotifyUpdateReserveInfo, 6), notification(edcb.NotifyUpdateEPGData, 7)}
	stream, err := edcb.NewTunerServer(s.client(t), nil).OpenEvents(t.Context())
	require.NoError(t, err)

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, &tunerserver.Event{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0}, event)
	// 4 回目の問い合わせ (巡回カウンタ 7 より後) は保留される。Close で切る。
	require.Eventually(t, func() bool { return len(s.received()) == 4 }, testWait, testTick)
	require.NoError(t, stream.Close())

	received := s.received()
	counts := make([]uint32, 0, len(received))
	for _, r := range received {
		counts = append(counts, binary.LittleEndian.Uint32(r.body[2:]))
	}
	assert.Equal(t, []uint32{0, 5, 6, 7}, counts)
}

func TestTunerServerEventsFails(t *testing.T) {
	t.Parallel()

	// 待っている間に EpgTimerSrv が失敗を返したら、Next はそのエラーを返す。続けて呼んでも同じエラーを返す。
	s := newFakeServer(t, map[uint32]reply{2200: {status: 0, payload: nil, stream: nil, hold: false, drop: false}})
	s.queued[2200] = []reply{notification(edcb.NotifyUpdateSrvStatus, 5)}
	stream, err := edcb.NewTunerServer(s.client(t), nil).OpenEvents(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })

	_, err = stream.Next()
	require.ErrorIs(t, err, edcb.ErrFailed)
	_, err = stream.Next()
	require.ErrorIs(t, err, edcb.ErrFailed)
}

func TestTunerServerEventsWhenBusy(t *testing.T) {
	t.Parallel()

	s := newFakeServer(t, map[uint32]reply{2200: {status: 208, payload: nil, stream: nil, hold: false, drop: false}})
	_, err := edcb.NewTunerServer(s.client(t), nil).OpenEvents(t.Context())
	require.ErrorIs(t, err, edcb.ErrBusy)
}

func TestTunerServerEventsAfterResponseTimeout(t *testing.T) {
	t.Parallel()

	// EpgTimerSrv は、保留した問い合わせを、応答の上限の時間が過ぎると、応答せずに閉じる。
	// ストリームは終わらせず、同じ巡回カウンタで待ち直し、その後の通知を返す。
	s := newFakeServer(t, map[uint32]reply{2200: {status: 0, payload: nil, stream: nil, hold: true, drop: false}})
	s.queued[2200] = []reply{
		notification(edcb.NotifyUpdateSrvStatus, 5),
		{status: 0, payload: nil, stream: nil, hold: false, drop: true},
		{status: 0, payload: nil, stream: nil, hold: false, drop: true},
		notification(edcb.NotifyUpdateEPGData, 6),
	}
	stream, err := edcb.NewTunerServer(s.client(t), nil).OpenEvents(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })

	event, err := stream.Next()
	require.NoError(t, err)
	assert.Equal(t, &tunerserver.Event{Type: tunerserver.EventTypeServiceChanged, ServiceID: 0}, event)
	received := s.received()
	counts := make([]uint32, 0, len(received))
	for _, r := range received[:4] {
		counts = append(counts, binary.LittleEndian.Uint32(r.body[2:]))
	}
	assert.Equal(t, []uint32{0, 5, 5, 5}, counts)
}
