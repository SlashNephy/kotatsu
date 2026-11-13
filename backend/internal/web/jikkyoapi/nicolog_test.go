package jikkyoapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/nicolog"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/jikkyoapi"
)

const (
	sourceNicolog = "nicolog"
	finishedCache = "private, max-age=600"
	noStore       = "no-store"
)

type fakeNicologFetcher struct {
	calls int
	err   error
}

func (f *fakeNicologFetcher) Comments(context.Context, string) ([]*nicolog.Comment, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}

	return []*nicolog.Comment{{Time: 12.5, Mail: "184", Content: "わこつ", UserID: "a:x"}}, nil
}

// nicologHandler は、ログイン中のユーザーとして NicologCommentsHandler を呼ぶハンドラーを返す。
func (f *commentsFixture) nicologHandler(t *testing.T, fetcher jikkyoapi.NicologFetcher) http.Handler {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("GET /internal/media/recordings/{id}/"+jikkyoapi.NicologCommentsFileName, jikkyoapi.NewNicologCommentsHandler(f.comments, fetcher))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), &auth.Principal{UserID: f.userID, Role: auth.RoleAdmin})))
	})
}

func TestNicologCommentsHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		sources    []string
		link       bool
		fetchErr   error
		wantStatus int
		wantCount  int
		wantCalls  int
		wantCache  string
	}{
		{name: "両方でオプトインしていれば、コメントを返す", sources: []string{sourceNicolog}, link: true, fetchErr: nil, wantStatus: http.StatusOK, wantCount: 1, wantCalls: 1, wantCache: finishedCache},
		{name: "オプトインしていなければ、問い合わせない", sources: []string{"nx_jikkyo"}, link: true, fetchErr: nil, wantStatus: http.StatusNotFound, wantCount: 0, wantCalls: 0, wantCache: ""},
		{name: "ファイルがなければ、空を返す", sources: []string{sourceNicolog}, link: false, fetchErr: nil, wantStatus: http.StatusOK, wantCount: 0, wantCalls: 0, wantCache: noStore},
		{name: "取得に失敗したら、502 を返す", sources: []string{sourceNicolog}, link: true, fetchErr: errors.New("down"), wantStatus: http.StatusBadGateway, wantCount: 0, wantCalls: 1, wantCache: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			finished := recordedFrom().Add(30 * time.Minute)
			f := newCommentsFixture(t, `{"packet":[]}`)
			recordingID := f.recording(t, grServiceID, recordedFrom(), &finished)
			fetcher := &fakeNicologFetcher{calls: 0, err: tt.fetchErr}
			handler := f.nicologHandler(t, fetcher)
			_, err := f.pool.Exec(t.Context(), `UPDATE system_settings SET comment_sources = $1`, tt.sources)
			require.NoError(t, err)
			_, err = f.pool.Exec(t.Context(), `UPDATE user_settings SET comment_sources = $1 WHERE user_id = $2`, tt.sources, f.userID)
			require.NoError(t, err)
			if tt.link {
				_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_nicolog_files (recording_id, path, checked_at) VALUES ($1, '/a.xml', now())`, recordingID)
				require.NoError(t, err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/internal/media/recordings/"+recordingID.String()+"/"+jikkyoapi.NicologCommentsFileName, nil)
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCalls, fetcher.calls)
			assert.Equal(t, tt.wantCache, rec.Header().Get("Cache-Control"))
			if tt.wantStatus == http.StatusOK {
				var body struct {
					Comments []struct {
						Time float64 `json:"time"`
					} `json:"comments"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Len(t, body.Comments, tt.wantCount)
			}
		})
	}
}
