package stream

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// fakeChannelOpener は、TS を渡し終えたら、ctx が終わるまで待つチャンネルのストリームを返す。
type fakeChannelOpener struct {
	data    []byte
	channel string
	opts    *mirakurun.StreamOptions
	// endEarly は、TS を渡し終えたら、長さを待たずに終えるかどうか。チューナーを奪われたときの代わり。
	endEarly bool
}

func (f *fakeChannelOpener) OpenChannelStream(_ context.Context, channelType, channel string, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	f.channel, f.opts = channelType+"/"+channel, opts
	done := make(chan struct{})
	if f.endEarly {
		close(done)
	}

	return &closingReader{blockingReader: blockingReader{r: bytes.NewReader(f.data), done: done}, done: done}, nil
}

// closingReader は、閉じたら読み込みの待ちを終える blockingReader。
type closingReader struct {
	blockingReader

	done chan struct{}
	once sync.Once
}

func (c *closingReader) Close() error {
	c.once.Do(func() {
		select {
		case <-c.done:
		default:
			close(c.done)
		}
	})

	return nil
}

// TestChannelInspector は、チャンネルの TS を、サービスごとの構成とビットレートにまとめることを確かめる (#830)。
func TestChannelInspector(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	opener := &fakeChannelOpener{data: data, channel: "", opts: nil, endEarly: false}

	got, err := NewChannelInspector(opener, clock.Real{}).Inspect(t.Context(), "GR", "27", 200*time.Millisecond)
	require.NoError(t, err)

	// チューナーを奪わない優先度で、チャンネルのストリームを開く。
	assert.Equal(t, "GR/27", opener.channel)
	assert.Equal(t, &mirakurun.StreamOptions{Decode: true, Priority: inspectPriority}, opener.opts)
	require.NotNil(t, got.TransportStreamID)
	require.Len(t, got.Programs, 1)
	kinds := make([]string, 0, len(got.Programs[0].Streams))
	for _, s := range got.Programs[0].Streams {
		kinds = append(kinds, s.Kind)
		assert.Positive(t, s.BPS)
	}
	// PMT と、映像、音声の順に並ぶ。
	assert.Equal(t, []string{"psi", "video", "audio"}, kinds)
	assert.Positive(t, got.TotalBPS)
	assert.GreaterOrEqual(t, got.TotalBPS, got.Programs[0].BPS)
}

// TestChannelInspectorInterrupted は、調べる長さより前にストリームが終わったら、欠けた結果を返さないことを確かめる。
func TestChannelInspectorInterrupted(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	opener := &fakeChannelOpener{data: data, channel: "", opts: nil, endEarly: true}

	_, err = NewChannelInspector(opener, clock.Real{}).Inspect(t.Context(), "GR", "27", 5*time.Second)
	assert.ErrorIs(t, err, ErrInspectionInterrupted)
}

func TestChannelInspectorWithoutTuner(t *testing.T) {
	t.Parallel()

	_, err := NewChannelInspector(nil, clock.Real{}).Inspect(t.Context(), "GR", "27", time.Second)
	assert.ErrorIs(t, err, ErrNoTuner)
}
