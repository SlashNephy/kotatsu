package edcb_test

import (
	"bytes"
	"context"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
)

// nhkSougou は、テストの ChSet5 にある NHK 総合 (東京) のサービス ID。
const nhkSougou = 3273601024

// feedServer は、NetworkTV を開き、TS を feed から少しずつ中継する偽の EpgTimerSrv と、TunerServer を返す。
// 中継は、notReady 回だけ失敗してから成功する。feed はテストの終わりに閉じる。
func feedServer(t *testing.T, notReady, bufferSize int) (*fakeServer, *edcb.TunerServer, *fakeIDs, chan<- []byte) {
	t.Helper()

	feed := make(chan []byte)
	t.Cleanup(sync.OnceFunc(func() { close(feed) }))
	s := streamServer(t, nil, notReady)
	s.replies[301] = reply{status: 1, feed: feed}
	ids := &fakeIDs{mu: sync.Mutex{}, used: false, acquired: []edcb.ServiceKey{}, released: []int32{}}
	server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{
		BonDrivers: nil, NetworkTVIDs: ids, PreemptionCheckInterval: time.Hour, SubscriberBufferSize: bufferSize,
	})

	return s, server, ids, feed
}

// countCommand は、偽の EpgTimerSrv が受け取った command の数を返す。
func countCommand(s *fakeServer, command uint32) int {
	count := 0
	for _, c := range commands(s.received()) {
		if c == command {
			count++
		}
	}

	return count
}

// readPackets は、stream から TS のパケットを n 個読む。
func readPackets(t *testing.T, stream io.Reader, n int) []byte {
	t.Helper()

	got := make([]byte, n*188)
	_, err := io.ReadFull(stream, got)
	require.NoError(t, err)

	return got
}

func TestTunerServerSharesStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		subscribers int
		// channel なら、2 つ目からの受け手は、同じ TS のチャンネルのストリームとして開く。
		channel bool
	}{
		{name: "one subscriber", subscribers: 1, channel: false},
		{name: "three subscribers", subscribers: 3, channel: false},
		// チャンネルのストリームは、その TS の最初のサービスと同じ NetworkTV を分け合う。
		{name: "service and channel", subscribers: 2, channel: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, server, ids, feed := feedServer(t, 0, 0)
			streams := make([]io.ReadCloser, 0, tt.subscribers)
			for i := range tt.subscribers {
				var stream io.ReadCloser
				var err error
				if tt.channel && i > 0 {
					stream, err = server.OpenChannelStream(t.Context(), "GR", "32736", nil)
				} else {
					stream, err = server.OpenServiceStream(t.Context(), nhkSougou, nil)
				}
				require.NoError(t, err)
				streams = append(streams, stream)
			}

			// 1 本の NetworkTV の TS を、受け手の全員が受け取る。
			packets := append(patPacket(32736, 1024), dataPacket()...)
			feed <- packets
			for _, stream := range streams {
				assert.Equal(t, packets, readPackets(t, stream, 2))
			}
			assert.Equal(t, 1, countCommand(s, 1073))
			acquired, _ := ids.state()
			assert.Len(t, acquired, 1)

			// 最後の受け手が去るまでは、NetworkTV を閉じない。
			for _, stream := range streams[:len(streams)-1] {
				require.NoError(t, stream.Close())
			}
			assert.Zero(t, countCommand(s, 1074))
			_, released := ids.state()
			assert.Empty(t, released)
			// 残った受け手は、受け取り続ける。
			last := streams[len(streams)-1]
			feed <- dataPacket()
			assert.Equal(t, dataPacket(), readPackets(t, last, 1))

			require.NoError(t, last.Close())
			assert.Equal(t, 1, countCommand(s, 1074))
			_, released = ids.state()
			assert.Equal(t, []int32{1000}, released)
		})
	}
}

func TestTunerServerSharedStreamLateSubscriber(t *testing.T) {
	t.Parallel()

	// 途中から加わった受け手は、パケットの区切りから受け取る。中継の TS は、パケットの途中で切れて届くことがある。
	_, server, _, feed := feedServer(t, 0, 0)
	first, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	pat, data := patPacket(32736, 1024), dataPacket()
	feed <- append(slices.Clone(pat), data[:100]...)
	assert.Equal(t, pat, readPackets(t, first, 1))

	late, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = late.Close() })
	feed <- append(slices.Clone(data[100:]), data...)
	assert.Equal(t, append(slices.Clone(data), data...), readPackets(t, first, 2))
	assert.Equal(t, append(slices.Clone(data), data...), readPackets(t, late, 2))
}

func TestTunerServerSharedStreamCutsSlowSubscriber(t *testing.T) {
	t.Parallel()

	// 受け手ごとに 2 パケットまで溜められる。読まない受け手だけを切り、ほかの受け手は待たせない。
	s, server, ids, feed := feedServer(t, 0, 2*188)
	fast, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = fast.Close() })
	slow, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = slow.Close() })

	feed <- patPacket(32736, 1024)
	assert.Equal(t, patPacket(32736, 1024), readPackets(t, fast, 1))
	for range 3 {
		feed <- dataPacket()
		assert.Equal(t, dataPacket(), readPackets(t, fast, 1))
	}

	_, err = io.ReadAll(slow)
	require.ErrorIs(t, err, edcb.ErrSubscriberTooSlow)
	// 切られた受け手がいても、NetworkTV は開いたままで、ほかの受け手は受け取り続ける。
	feed <- dataPacket()
	assert.Equal(t, dataPacket(), readPackets(t, fast, 1))
	assert.Zero(t, countCommand(s, 1074))
	_, released := ids.state()
	assert.Empty(t, released)
}

func TestTunerServerSharedStreamPreemptedCutsAll(t *testing.T) {
	t.Parallel()

	// EDCB の予約にチューナーを取られて局が変わったら、すべての受け手に、自分の局の分を返してから同じエラーで知らせ、
	// NetworkTV を 1 度だけ閉じる。
	s, server, ids, feed := feedServer(t, 0, 0)
	streams := make([]io.ReadCloser, 0, 2)
	for range 2 {
		stream, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = stream.Close() })
		streams = append(streams, stream)
	}
	own := append(patPacket(32736, 1024), dataPacket()...)
	feed <- append(slices.Clone(own), append(patPacket(0x4010, 101), dataPacket()...)...)
	for _, stream := range streams {
		got, err := io.ReadAll(stream)
		require.ErrorIs(t, err, tunerserver.ErrTunerPreempted)
		assert.Equal(t, own, got)
	}
	assert.Equal(t, 1, countCommand(s, 1074))
	_, released := ids.state()
	assert.Equal(t, []int32{1000}, released)
}

func TestTunerServerSharedStreamReopens(t *testing.T) {
	t.Parallel()

	// 最後の受け手が去って閉じた後に来た受け手には、NetworkTV を開き直す。
	s, server, ids, _ := feedServer(t, 0, 0)
	for range 2 {
		stream, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
		require.NoError(t, err)
		require.NoError(t, stream.Close())
	}
	assert.Equal(t, 2, countCommand(s, 1073))
	assert.Equal(t, 2, countCommand(s, 1074))
	_, released := ids.state()
	assert.Equal(t, []int32{1000, 1000}, released)
}

func TestTunerServerSharedStreamWaiterLeaves(t *testing.T) {
	t.Parallel()

	// 開いている途中に、最初の受け手が待ちきれずに去っても、ほかの受け手の開きは止めない。
	// 中継は 3 回失敗してから受け取れるようになる (0.3 秒ほど)。
	s, server, ids, feed := feedServer(t, 3, 0)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	opened := make(chan io.ReadCloser, 1)
	go func() {
		stream, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
		assert.NoError(t, err)
		opened <- stream
	}()
	_, err := server.OpenServiceStream(ctx, nhkSougou, nil)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	stream := <-opened
	require.NotNil(t, stream)
	feed <- patPacket(32736, 1024)
	assert.True(t, bytes.Equal(patPacket(32736, 1024), readPackets(t, stream, 1)))
	assert.Equal(t, 1, countCommand(s, 1073))
	require.NoError(t, stream.Close())
	_, released := ids.state()
	assert.Equal(t, []int32{1000}, released)
}

func TestTunerServerSharedStreamClosesWhenContextEnds(t *testing.T) {
	t.Parallel()

	// ほかのストリームと同じく、開いたときの ctx が終わったら閉じる。最後の受け手なら、NetworkTV も閉じる。
	s, server, ids, feed := feedServer(t, 0, 0)
	ctx, cancel := context.WithCancel(t.Context())
	stream, err := server.OpenServiceStream(ctx, nhkSougou, nil)
	require.NoError(t, err)
	feed <- patPacket(32736, 1024)
	assert.Equal(t, patPacket(32736, 1024), readPackets(t, stream, 1))

	cancel()
	_, err = stream.Read(make([]byte, 188))
	require.ErrorIs(t, err, context.Canceled)
	require.Eventually(t, func() bool {
		_, released := ids.state()

		return slices.Equal(released, []int32{1000})
	}, testWait, testTick)
	assert.Equal(t, 1, countCommand(s, 1074))
}

// slowIDs は、ID を消すのを、release が閉じられるまで待たせる。
type slowIDs struct {
	*fakeIDs

	release chan struct{}
}

func (s *slowIDs) Forget(ctx context.Context, id int32) error {
	<-s.release

	return s.fakeIDs.Forget(ctx, id)
}

func TestTunerServerSharedStreamWaitsForClose(t *testing.T) {
	t.Parallel()

	// 閉じている途中に来た受け手は、NetworkTV を閉じて ID を返し終えるのを待ってから開き直す。
	feed := make(chan []byte)
	t.Cleanup(sync.OnceFunc(func() { close(feed) }))
	s := streamServer(t, nil, 0)
	s.replies[301] = reply{status: 1, feed: feed}
	ids := &slowIDs{fakeIDs: &fakeIDs{mu: sync.Mutex{}, acquired: []edcb.ServiceKey{}, released: []int32{}}, release: make(chan struct{})}
	server := edcb.NewTunerServer(s.client(t), &edcb.TunerServerOptions{NetworkTVIDs: ids, PreemptionCheckInterval: time.Hour})
	first, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
	require.NoError(t, err)
	closed := make(chan error, 1)
	go func() { closed <- first.Close() }()
	require.Eventually(t, func() bool { return countCommand(s, 1074) == 1 }, testWait, testTick)

	reopened := make(chan error, 1)
	go func() {
		stream, err := server.OpenServiceStream(t.Context(), nhkSougou, nil)
		if err == nil {
			t.Cleanup(func() { _ = stream.Close() })
		}
		reopened <- err
	}()
	// ID を返し終えるまでは、開き直さない。fakeIDs は ID を 1 つしか配らないので、待たずに開くと、ID が足りずに失敗する。
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, countCommand(s, 1073))
	close(ids.release)
	require.NoError(t, <-closed)
	require.NoError(t, <-reopened)
	assert.Equal(t, 2, countCommand(s, 1073))
}
