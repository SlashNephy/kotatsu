package edcb_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// fakeIDs は、NetworkTV の ID を 1 つだけ配る。
type fakeIDs struct {
	mu       sync.Mutex
	used     bool
	acquired []edcb.ServiceKey
	released []int32
}

func (f *fakeIDs) Acquire(_ context.Context, service edcb.ServiceKey) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.used {
		return 0, edcb.ErrNoNetworkTVID
	}
	f.used = true
	f.acquired = append(f.acquired, service)

	return 1000, nil
}

func (f *fakeIDs) Release(_ context.Context, id int32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.used = false
	f.released = append(f.released, id)

	return nil
}

func (f *fakeIDs) state() ([]edcb.ServiceKey, []int32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]edcb.ServiceKey{}, f.acquired...), append([]int32{}, f.released...)
}

// streamServer は、ChSet5 と、NetworkTV を開いてプロセス 27 を返し、TS を中継する偽の EpgTimerSrv を返す。
// 中継は、notReady 回だけ失敗してから成功する。
func streamServer(t *testing.T, ts []byte, notReady int) *fakeServer {
	t.Helper()

	s := newFakeServer(t, map[uint32]reply{
		1060: {status: 1, payload: fixture(t, "file_copy_chset5.bin"), stream: nil, hold: false, drop: false},
		1073: {status: 1, payload: payload{}.u32(27), stream: nil, hold: false, drop: false},
		1074: {status: 1, payload: nil, stream: nil, hold: false, drop: false},
		301:  {status: 1, payload: nil, stream: ts, hold: false, drop: false},
	})
	for range notReady {
		s.queued[301] = append(s.queued[301], reply{status: 0, payload: nil, stream: nil, hold: false, drop: false})
	}

	return s
}

func TestTunerServerOpenServiceStream(t *testing.T) {
	t.Parallel()

	ts := make([]byte, 2*188)
	ts[0], ts[188] = 0x47, 0x47
	tests := []struct {
		name     string
		notReady int
		open     func(*edcb.TunerServer) (io.ReadCloser, error)
	}{
		{name: "service", notReady: 0, open: func(s *edcb.TunerServer) (io.ReadCloser, error) {
			return s.OpenServiceStream(context.Background(), 3273601024, nil)
		}},
		// 開いた直後は FIFO ができていないので、中継をやり直す。
		{name: "relay not ready yet", notReady: 2, open: func(s *edcb.TunerServer) (io.ReadCloser, error) {
			return s.OpenServiceStream(context.Background(), 3273601024, nil)
		}},
		// チャンネルのストリームは、その TS の最初のサービスを開く。
		{name: "channel", notReady: 0, open: func(s *edcb.TunerServer) (io.ReadCloser, error) {
			return s.OpenChannelStream(context.Background(), "GR", "32736", nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := streamServer(t, ts, tt.notReady)
			ids := &fakeIDs{mu: sync.Mutex{}, used: false, acquired: []edcb.ServiceKey{}, released: []int32{}}
			stream, err := tt.open(edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{BonDrivers: nil, NetworkTVIDs: ids, PreemptionCheckInterval: 0}))
			require.NoError(t, err)
			got, err := io.ReadAll(stream)
			require.NoError(t, err)
			assert.Equal(t, ts, got)
			acquired, released := ids.state()
			// テストの ChSet5 にある NHK 総合 (東京) を開く。
			assert.Equal(t, []edcb.ServiceKey{{ONID: 32736, TSID: 32736, SID: 1024}}, acquired)
			assert.Empty(t, released)

			// 閉じたら、NetworkTV を閉じ、ID を返す。2 度閉じても、1 度しか閉じない。
			require.NoError(t, stream.Close())
			require.NoError(t, stream.Close())
			_, released = ids.state()
			assert.Equal(t, []int32{1000}, released)
			want := []uint32{1060, 1073}
			for range tt.notReady + 1 {
				want = append(want, 301)
			}
			assert.Equal(t, append(want, 1074), commands(s.received()))
		})
	}
}

func TestTunerServerOpenServiceStreamFails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		serviceID int64
		// busy なら、ID をほかの人が使っている。
		busy        bool
		openStatus  uint32
		want        error
		wantRelease []int32
	}{
		{name: "unknown service", serviceID: 3273601999, busy: false, openStatus: 1, want: tunerserver.ErrNotFound, wantRelease: []int32{}},
		{name: "no free ID", serviceID: 3273601024, busy: true, openStatus: 1, want: tunerserver.ErrTunerUnavailable, wantRelease: []int32{}},
		// EDCB がチューナーを開けなければ、ID を返す。
		{name: "no tuner", serviceID: 3273601024, busy: false, openStatus: 0, want: tunerserver.ErrTunerUnavailable, wantRelease: []int32{1000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := streamServer(t, nil, 0)
			s.queued[1073] = []reply{{status: tt.openStatus, payload: payload{}.u32(27), stream: nil, hold: false, drop: false}}
			ids := &fakeIDs{mu: sync.Mutex{}, used: tt.busy, acquired: []edcb.ServiceKey{}, released: []int32{}}
			server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{BonDrivers: nil, NetworkTVIDs: ids, PreemptionCheckInterval: 0})
			_, err := server.OpenServiceStream(t.Context(), tt.serviceID, nil)
			require.ErrorIs(t, err, tt.want)
			_, released := ids.state()
			assert.Equal(t, tt.wantRelease, released)
		})
	}
}

func TestTunerServerOpenChannelStreamNotFound(t *testing.T) {
	t.Parallel()

	// チャンネルの設定にない TS は、NetworkTV の ID を取らずに、ないものとして返す。
	s := streamServer(t, nil, 0)
	ids := &fakeIDs{mu: sync.Mutex{}, used: false, acquired: []edcb.ServiceKey{}, released: []int32{}}
	server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{BonDrivers: nil, NetworkTVIDs: ids, PreemptionCheckInterval: 0})
	for _, tt := range []struct{ channelType, channel string }{{"GR", "1"}, {"BS", "32736"}} {
		_, err := server.OpenChannelStream(t.Context(), tt.channelType, tt.channel, nil)
		require.ErrorIs(t, err, tunerserver.ErrNotFound)
	}
	acquired, _ := ids.state()
	assert.Empty(t, acquired)
}

// crcMPEG は、CRC-32/MPEG-2 を求める。
func crcMPEG(b []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, c := range b {
		crc ^= uint32(c) << 24
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

// patPacket は、transport_stream_id が tsid で、NIT とサービス sid を持つ PAT のパケットを作る。
func patPacket(tsid, sid uint16) []byte {
	section := binary.BigEndian.AppendUint16([]byte{0x00, 0xB0, 0x11}, tsid)
	section = append(section, 0xC1, 0x00, 0x00, 0x00, 0x00, 0xE0, 0x10)
	section = binary.BigEndian.AppendUint16(section, sid)
	section = append(section, 0xE1, 0x00)
	section = binary.BigEndian.AppendUint32(section, crcMPEG(section))
	p := bytes.Repeat([]byte{0xFF}, 188)
	copy(p, []byte{0x47, 0x40, 0x00, 0x10, 0x00})
	copy(p[5:], section)

	return p
}

// dataPacket は、PID 0x100 のパケットを作る。
func dataPacket() []byte {
	p := bytes.Repeat([]byte{0xAA}, 188)
	copy(p, []byte{0x47, 0x01, 0x00, 0x10})

	return p
}

func TestTunerServerStreamPreemptedByPAT(t *testing.T) {
	t.Parallel()

	// EDCB の予約にチューナーを取られると、接続を保ったまま別の局 (TSID 0x4010) の TS が届く。
	own := append(patPacket(32736, 1024), dataPacket()...)
	other := append(patPacket(0x4010, 101), dataPacket()...)
	s := streamServer(t, append(append([]byte{}, own...), other...), 0)
	ids := &fakeIDs{mu: sync.Mutex{}, used: false, acquired: []edcb.ServiceKey{}, released: []int32{}}
	server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{BonDrivers: nil, NetworkTVIDs: ids, PreemptionCheckInterval: time.Hour})
	stream, err := server.OpenServiceStream(t.Context(), 3273601024, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })

	got, err := io.ReadAll(stream)
	require.ErrorIs(t, err, tunerserver.ErrTunerPreempted)
	// 別の局の TS は返さない。
	assert.NotContains(t, string(got), string(patPacket(0x4010, 101)))
	assert.True(t, bytes.HasPrefix(got, patPacket(32736, 1024)))
}

func TestTunerServerStreamPreemptedByCheck(t *testing.T) {
	t.Parallel()

	// NetworkTV がまだ開いているかを問い合わせて、閉じていたら、中継の接続を切って知らせる。
	s := streamServer(t, nil, 0)
	s.replies[301] = reply{status: 1, payload: nil, stream: patPacket(32736, 1024), hold: true, drop: false}
	s.queued[1073] = []reply{
		{status: 1, payload: payload{}.u32(27), stream: nil, hold: false, drop: false},
		{status: 1, payload: payload{}.u32(27), stream: nil, hold: false, drop: false},
		{status: 0, payload: nil, stream: nil, hold: false, drop: false},
	}
	ids := &fakeIDs{mu: sync.Mutex{}, used: false, acquired: []edcb.ServiceKey{}, released: []int32{}}
	server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{BonDrivers: nil, NetworkTVIDs: ids, PreemptionCheckInterval: 10 * time.Millisecond})
	stream, err := server.OpenServiceStream(t.Context(), 3273601024, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Close() })

	_, err = io.ReadAll(stream)
	require.ErrorIs(t, err, tunerserver.ErrTunerPreempted)
}
