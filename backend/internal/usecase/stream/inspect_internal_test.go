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
)

// fakeChannelOpener は、TS を渡し終えたら、ctx が終わるまで待つチャンネルのストリームを返す。
type fakeChannelOpener struct {
	data    []byte
	channel string
	opts    *mirakurun.StreamOptions
}

func (f *fakeChannelOpener) OpenChannelStream(ctx context.Context, channelType, channel string, opts *mirakurun.StreamOptions) (io.ReadCloser, error) {
	f.channel, f.opts = channelType+"/"+channel, opts

	return io.NopCloser(&blockingReader{r: bytes.NewReader(f.data), done: ctx.Done()}), nil
}

// TestChannelInspector は、チャンネルの TS を、サービスごとの構成とビットレートにまとめることを確かめる (#830)。
func TestChannelInspector(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../testdata/ts/test.ts")
	require.NoError(t, err)
	opener := &fakeChannelOpener{data: data, channel: "", opts: nil}

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

func TestChannelInspectorWithoutTuner(t *testing.T) {
	t.Parallel()

	_, err := NewChannelInspector(nil, clock.Real{}).Inspect(t.Context(), "GR", "27", time.Second)
	assert.ErrorIs(t, err, ErrNoTuner)
}
