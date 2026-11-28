package streamapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

// TestVODDataBroadcastHandler は、録画型の再生元のデータ放送の配信が、映像の配信の応答から PSI と、PCR だけにした
// 映像のパケットを流し、長さのヘッダーを外すことを確認する (#2270)。
// testdata/ts/test.ts は、PMT が 0x01F0、映像 (PCR を兼ねる) が 0x0111、音声が 0x0112 で、データカルーセルを持たない。
func TestVODDataBroadcastHandler(t *testing.T) {
	t.Parallel()

	data := loadTestTS(t)
	// inner は、録画の配信の代わりに、テスト用の TS を長さと読み始めた位置を付けて返す。
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("X-Kotatsu-Start", r.URL.Query().Get("start"))
		_, _ = w.Write(data)
	})

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "start is required", query: "", wantStatus: http.StatusBadRequest},
		{name: "empty start is rejected", query: "?start=", wantStatus: http.StatusBadRequest},
		{name: "download is rejected", query: "?start=10&download=1", wantStatus: http.StatusBadRequest},
		{name: "stream from start", query: "?start=10", wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/streams/recordings/x/data-broadcast"+tt.query, http.NoBody)
			streamapi.NewVODDataBroadcastHandler(inner).ServeHTTP(recorder, req)

			require.Equal(t, tt.wantStatus, recorder.Code)
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.Empty(t, recorder.Header().Get("Content-Length"))
			assert.Equal(t, "10", recorder.Header().Get("X-Kotatsu-Start"))
			body, err := io.ReadAll(recorder.Body)
			require.NoError(t, err)
			require.NotEmpty(t, body)
			require.Zero(t, len(body)%ts.PacketSize)
			assert.Less(t, len(body), len(data))
			pids := map[uint16]int{}
			for i := 0; i < len(body); i += ts.PacketSize {
				p := body[i : i+ts.PacketSize]
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
		})
	}
}

// TestVODDataBroadcastHandlerPassesErrors は、映像の配信が成功でない応答を返したら、本文をフィルターに通さずに返すことを確認する。
func TestVODDataBroadcastHandlerPassesErrors(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "recording not found", http.StatusNotFound)
	})
	recorder := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/streams/recordings/x/data-broadcast?start=0", http.NoBody)
	streamapi.NewVODDataBroadcastHandler(inner).ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Equal(t, "recording not found\n", recorder.Body.String())
}
