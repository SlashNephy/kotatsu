package stream

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

func TestLiveRates(t *testing.T) {
	t.Parallel()

	programs := []*ts.Program{{Number: 1024, PMTPID: 0x1F0, PCRPID: 0x100, Version: 0, Streams: []*ts.Stream{
		{PID: 0x100, StreamType: 0x02, Kind: ts.StreamVideo, ComponentTag: nil},
		{PID: 0x110, StreamType: 0x0F, Kind: ts.StreamAudio, ComponentTag: nil},
		{PID: 0x130, StreamType: 0x06, Kind: ts.StreamOther, ComponentTag: nil},
	}}}
	stats := func(video, audio, other, psi, unknown, drops int64) *ts.Stats {
		return &ts.Stats{
			Packets:  video + audio + other + psi + unknown,
			Programs: programs,
			PIDs: []*ts.PIDStats{
				{PID: 0x0000, Packets: psi, Drops: 0, Errors: 0, Scrambled: 0},
				{PID: 0x0100, Packets: video, Drops: drops, Errors: 1, Scrambled: 0},
				{PID: 0x0110, Packets: audio, Drops: 0, Errors: 0, Scrambled: 0},
				{PID: 0x0130, Packets: other, Drops: 0, Errors: 0, Scrambled: 2},
				{PID: 0x0901, Packets: unknown, Drops: 0, Errors: 0, Scrambled: 0},
			},
		}
	}

	tests := []struct {
		name     string
		previous *ts.Stats
		current  *ts.Stats
		elapsed  time.Duration
		want     *LiveRates
	}{
		{
			name:     "the bytes of each kind over the interval",
			previous: stats(0, 0, 0, 0, 0, 0),
			current:  stats(10000, 1000, 100, 10, 1, 3),
			elapsed:  time.Second,
			want: &LiveRates{
				BPS:       KindBytes{Video: 15_040_000, Audio: 1_504_000, Other: 150_400, PSI: 15_040, Unknown: 1_504},
				TotalBPS:  16_710_944,
				Drops:     3,
				Errors:    1,
				Scrambled: 2,
			},
		},
		{
			name:     "half a second doubles the rate",
			previous: stats(10000, 0, 0, 0, 0, 0),
			current:  stats(15000, 0, 0, 0, 0, 0),
			elapsed:  500 * time.Millisecond,
			want: &LiveRates{
				BPS:       KindBytes{Video: 15_040_000, Audio: 0, Other: 0, PSI: 0, Unknown: 0},
				TotalBPS:  15_040_000,
				Drops:     0,
				Errors:    1,
				Scrambled: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, liveRates(tt.previous, tt.current, tt.elapsed))
		})
	}
}

// fakeOpener は、TS を渡し終えたら、ctx が終わるまで待つストリームを返す。チューナーサーバーの、終わらない配信の代わり。
type fakeOpener struct {
	data []byte
	opts *mirakurun.StreamOptions
}

type blockingReader struct {
	r io.Reader
	// done は、開いた ctx の Done。
	done <-chan struct{}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err == io.EOF {
		<-b.done

		return n, io.EOF
	}

	return n, err
}

func (f *fakeOpener) OpenServiceStream(ctx context.Context, _ int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	f.opts = opts

	return io.NopCloser(&blockingReader{r: bytes.NewReader(f.data), done: ctx.Done()}), nil
}

func TestLiveRatesWatcher(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	opener := &fakeOpener{data: data, opts: nil}
	watcher := NewLiveRatesWatcher(opener, clock.Real{}, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(t.Context())

	rates, err := watcher.Watch(ctx, 3273601024)
	require.NoError(t, err)
	// チューナーを奪わない優先度で開く。
	assert.Equal(t, &mirakurun.StreamOptions{Decode: true, Priority: ratesPriority}, opener.opts)
	// 最初の間隔で、TS の全体を読み終えているとは限らないので、映像と音声が数えられるまで読む。
	var seen *LiveRates
	for r := range rates {
		seen = r
		if r.BPS.Video > 0 && r.BPS.Audio > 0 {
			break
		}
	}
	require.NotNil(t, seen)
	assert.Positive(t, seen.BPS.Video)
	assert.Positive(t, seen.BPS.Audio)

	cancel()
	// ctx が終わったら、チャネルを閉じる。
	for range rates {
	}
}

func TestLiveRatesWatcherWithoutTuner(t *testing.T) {
	t.Parallel()

	_, err := NewLiveRatesWatcher(nil, clock.Real{}, 0).Watch(t.Context(), 1)
	assert.ErrorIs(t, err, ErrNoTuner)
}
