package jikkyoapi_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
	"github.com/SlashNephy/kotatsu/internal/web/jikkyoapi"
)

// testLibrary は、テストのライブラリの名前。
const testLibrary = "lib"

// readTestTS は、testdata/ts/test.ts を返す。2026-01-01 0:00 (JST) の TOT と、こたつテスト1 (32736、1024) の SDT と EIT を持つ。
func readTestTS(t *testing.T) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "ts", "test.ts"))
	require.NoError(t, err)

	return data
}

// TestLibraryCommentsHandler は、ライブラリのファイルの過去ログを、TS から読んだ局と時間帯で引き、TOT で読んだ録り始めを基準にすることを確かめる (#1734)。
// Kotatsu のサービスの行がなくても、ネットワークとサービス ID で実況のチャンネルを引く。
func TestLibraryCommentsHandler(t *testing.T) {
	t.Parallel()

	f := newCommentsFixture(t, `{"packet":[]}`)
	libraryDir, dataDir := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(libraryDir, "show.ts"), readTestTS(t), 0o600))
	libraries := medialib.NewSet(map[string]string{}, []string{}, map[string]string{testLibrary: libraryDir})
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
	ref := filelib.Ref{Library: testLibrary, Path: "show.ts"}
	require.Eventually(t, func() bool {
		file, err := source.Open(t.Context(), ref)
		require.NoError(t, err)

		return file.Index.Complete
	}, 10*time.Second, 10*time.Millisecond)
	mux := http.NewServeMux()
	mux.Handle("GET /internal/media/library/{id}/"+jikkyoapi.CommentsFileName, jikkyoapi.NewLibraryCommentsHandler(f.comments, source))
	_, err := f.pool.Exec(t.Context(), "DELETE FROM services")
	require.NoError(t, err)

	tests := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "file", id: ref.Encode(), wantStatus: http.StatusOK},
		{name: "missing", id: filelib.Ref{Library: testLibrary, Path: "missing.ts"}.Encode(), wantStatus: http.StatusNotFound},
		{name: "invalid", id: "!!", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/media/library/"+tt.id+"/"+jikkyoapi.CommentsFileName, nil)
			f.serve(mux, rec, req)
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantStatus != http.StatusOK {
				return
			}
			var body commentsBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.NotNil(t, body.Channel)
			assert.Equal(t, "jk1", body.Channel.ID)
			require.NotNil(t, body.Origin)
			assert.True(t, body.Origin.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("JST", 9*3600))))
			assert.NotNil(t, f.fake.query.Load(), "過去ログ API に問い合わせる")
		})
	}
}

// serve は、ログイン中のユーザーの要求として、h に渡す。
func (f *commentsFixture) serve(h http.Handler, w http.ResponseWriter, r *http.Request) {
	h.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), &auth.Principal{
		UserID: f.userID, Role: domain.RoleMember, SessionID: uuid.Nil(), AuthenticatedAt: recordedFrom(), OAuthClientID: "",
	})))
}
