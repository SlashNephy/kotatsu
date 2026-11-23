package kyusu_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

// libraryName は、テストのライブラリの名前。
const libraryName = "lib"

// runIndexer は、テストを終えるときに、インデックスを作る処理を止めてから一時ディレクトリを消させる。
func runIndexer(t *testing.T, indexer *filelib.Indexer) {
	t.Helper()

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
}

// TestVODLibraryPlaylist は、ライブラリのファイルの仮想 HLS のプレイリストを、tenban が作ったインデックスから作ることを確認する (#1734)。
// インデックスがまだなければ 503 を返す。kyusu はインデックスを作らない。
func TestVODLibraryPlaylist(t *testing.T) {
	t.Parallel()

	libraryDir, dataDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, "show.ts"), loadTestTS(t), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, "fresh.ts"), loadTestTS(t), 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{libraryName: libraryDir})
	// 置いたばかりのファイルは書いている途中とみなされるので、時計を進めておく。
	clk := clocktest.NewFake(time.Now().Add(time.Hour))
	logger := slog.New(slog.DiscardHandler)
	// tenban の Indexer で、show.ts の索引を作っておく。
	indexer := filelib.NewIndexer(dataDir, libraries, clk, logger, true)
	runIndexer(t, indexer)
	tenban := filelib.NewSource(libraries, indexer)
	shown := filelib.Ref{Library: libraryName, Path: "show.ts"}
	require.Eventually(t, func() bool {
		file, err := tenban.Open(t.Context(), shown)
		require.NoError(t, err)

		return file.Index.Complete
	}, 10*time.Second, 10*time.Millisecond)
	f := newVODFixture(t, new(fakeVODEncoder), func(opts *kyusu.VODOptions) {
		opts.Library = filelib.NewSource(libraries, filelib.NewIndexer(dataDir, libraries, clk, logger, false))
	})
	get := func(ref filelib.Ref) (int, string) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			f.server.URL+kyusu.LibraryPathPrefix+ref.Encode()+"/360p/1/playlist.m3u8", nil)
		require.NoError(t, err)
		f.token.Authorize(req)
		resp, err := httpclienttest.New(t).Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return resp.StatusCode, string(body)
	}

	tests := []struct {
		name       string
		ref        filelib.Ref
		wantStatus int
		wantEnd    bool
	}{
		{name: "indexed", ref: shown, wantStatus: http.StatusOK, wantEnd: true},
		{name: "not indexed yet", ref: filelib.Ref{Library: libraryName, Path: "fresh.ts"}, wantStatus: http.StatusServiceUnavailable},
		{name: "missing", ref: filelib.Ref{Library: libraryName, Path: "missing.ts"}, wantStatus: http.StatusNotFound},
		{name: "outside", ref: filelib.Ref{Library: libraryName, Path: "../show.ts"}, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			status, body := get(tt.ref)
			require.Equal(t, tt.wantStatus, status, body)
			if status == http.StatusOK {
				assert.Contains(t, body, "#EXT-X-PLAYLIST-TYPE:VOD\n")
				assert.Equal(t, tt.wantEnd, strings.HasSuffix(body, "#EXT-X-ENDLIST\n"))
			}
		})
	}
}
