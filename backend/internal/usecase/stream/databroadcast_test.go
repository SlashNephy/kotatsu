package stream_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// TestDataBroadcastHandlerFiltersPackets は、データ放送用のエンドポイントが、サービスストリームから
// PSI と、PCR だけにした映像のパケットを流し、音声と映像のペイロードを流さないことを確かめる (設計書 7.4)。
// testdata/ts/test.ts は、PMT が 0x01F0、映像 (PCR を兼ねる) が 0x0111、音声が 0x0112 で、データカルーセルを持たない。
func TestDataBroadcastHandlerFiltersPackets(t *testing.T) {
	t.Parallel()

	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	upstream := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(clk.Now(), loadTestTS(t))))
	t.Cleanup(upstream.Close)
	client, err := mirakurun.New(&mirakurun.Options{
		URL: upstream.URL, Username: "", Password: "", UserAgent: "", HTTPClient: nil, RequestTimeout: 0, DetectInterval: 0, Clock: clk,
	})
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("GET /streams/services/{id}/data-broadcast", stream.NewDataBroadcastHandler(client, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/streams/services/"+serviceID+"/data-broadcast", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	packets := make([]byte, 40*ts.PacketSize)
	_, err = io.ReadFull(resp.Body, packets)
	require.NoError(t, err)
	pids := map[uint16]int{}
	for i := 0; i < len(packets); i += ts.PacketSize {
		p := packets[i : i+ts.PacketSize]
		require.Equal(t, byte(0x47), p[0])
		pid := uint16(p[1]&0x1F)<<8 | uint16(p[2])
		pids[pid]++
		if pid == 0x0111 {
			assert.Equal(t, byte(0x20), p[3]&0x30, "video packets carry only the adaptation field")
		}
	}
	assert.Positive(t, pids[0x0000], "PAT")
	assert.Positive(t, pids[0x01F0], "PMT")
	assert.Positive(t, pids[0x0111], "PCR")
	assert.Zero(t, pids[0x0112], "audio")
}
