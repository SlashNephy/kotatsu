package streamapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/web/streamapi"
)

// libraryName は、テストのライブラリの名前。
const libraryName = "anime"

// loadLibraryTS は、testdata/ts/test.ts を返す。
func loadLibraryTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

// TestLibraryHandler は、ライブラリのファイルを、インデックスのできた後は指定した位置の手前の I フレームから流し、流し始めた位置を返すことを確認する (#1734)。
func TestLibraryHandler(t *testing.T) {
	t.Parallel()

	libraryDir, dataDir := t.TempDir(), t.TempDir()
	data := loadLibraryTS(t)
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, "show.ts"), data, 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{libraryName: libraryDir})
	// 置いたばかりのファイルは書いている途中とみなされるので、時計を進めておく。
	indexer := filelib.NewIndexer(dataDir, libraries, clocktest.NewFake(time.Now().Add(time.Hour)), slog.New(slog.DiscardHandler), true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.NoError(t, indexer.Run(ctx))
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	source := filelib.NewSource(libraries, indexer)
	ref := filelib.Ref{Library: libraryName, Path: "show.ts"}
	// 開くと索引を作り始めるので、作り終えるまで待つ。
	var file *filelib.File
	require.Eventually(t, func() bool {
		var err error
		file, err = source.Open(t.Context(), ref)
		require.NoError(t, err)

		return file.Index.Complete
	}, 10*time.Second, 10*time.Millisecond)
	mux := http.NewServeMux()
	mux.Handle("GET /internal/streams/library/{id}", streamapi.NewLibraryHandler(source, slog.New(slog.DiscardHandler)))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	secondKeyframe := file.Index.Entries[1]

	tests := []struct {
		name       string
		method     string
		id         string
		query      string
		wantStatus int
		wantStart  string
		wantFrom   int64
	}{
		{name: "from the beginning", method: http.MethodGet, id: ref.Encode(), query: "", wantStatus: http.StatusOK, wantStart: "0.000", wantFrom: 0},
		// 2 つ目の I フレーム (0.5005 秒) と 3 つ目の間を求めたら、2 つ目から流す。
		{name: "seek", method: http.MethodGet, id: ref.Encode(), query: "?start=0.7", wantStatus: http.StatusOK, wantStart: "0.500", wantFrom: secondKeyframe.Offset},
		{name: "head", method: http.MethodHead, id: ref.Encode(), query: "?start=0.7", wantStatus: http.StatusOK, wantStart: "0.500", wantFrom: -1},
		{name: "invalid ID", method: http.MethodGet, id: "!!", wantStatus: http.StatusBadRequest},
		{name: "outside", method: http.MethodGet, id: filelib.Ref{Library: libraryName, Path: "../show.ts"}.Encode(), wantStatus: http.StatusBadRequest},
		{name: "missing", method: http.MethodGet, id: filelib.Ref{Library: libraryName, Path: "missing.ts"}.Encode(), wantStatus: http.StatusNotFound},
		{name: "invalid start", method: http.MethodGet, id: ref.Encode(), query: "?start=-1", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), tt.method, server.URL+"/internal/streams/library/"+tt.id+tt.query, nil)
			require.NoError(t, err)
			resp, err := httpclienttest.New(t).Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, resp.StatusCode, string(body))
			if tt.wantStatus != http.StatusOK {
				return
			}
			assert.Equal(t, tt.wantStart, resp.Header.Get(streamapi.StartHeader))
			if tt.wantFrom < 0 {
				assert.Empty(t, body)

				return
			}
			// 途中から流すときは、先に PAT と PMT を置くので、末尾が流し始めた位置からのファイルの中身と一致する。
			require.GreaterOrEqual(t, len(body), len(data)-int(tt.wantFrom))
			assert.Equal(t, data[tt.wantFrom:], body[len(body)-(len(data)-int(tt.wantFrom)):])
		})
	}
}
