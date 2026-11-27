package tmdb_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
)

// testToken は、テストで使うトークン。エラーの文や URL に出ないことを確認する。
const testToken = "test-read-access-token-0123456789"

// request は、モックの TMDB が受けた問い合わせ 1 回。
type request struct {
	path          string
	rawQuery      string
	authorization string
	userAgent     string
}

// newServer は、handler で応えるモックの TMDB を立て、受けた問い合わせを記録する。
func newServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func() []request) {
	t.Helper()

	var (
		mu       sync.Mutex
		requests []request
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, request{
			path: r.URL.Path, rawQuery: r.URL.RawQuery, authorization: r.Header.Get("Authorization"), userAgent: r.Header.Get("User-Agent"),
		})
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	return server, func() []request {
		mu.Lock()
		defer mu.Unlock()

		return append([]request{}, requests...)
	}
}

// newClient は、server に問い合わせる Client を返す。
// 通信には server ごとの Transport を使う。httptest.Server の Close は http.DefaultTransport の待機中の接続を閉じるので、
// 並列のテストで DefaultTransport を共有すると、ほかのテストの Close で問い合わせが切れることがある。
func newClient(server *httptest.Server) *tmdb.Client {
	httpClient := tmdb.NewHTTPClient(nil)
	httpClient.Transport = server.Client().Transport

	return tmdb.NewClient(&tmdb.Options{HTTPClient: httpClient, URL: server.URL, Interval: time.Millisecond})
}

// TestClientRequests は、問い合わせの形と、応答の読み方を確認する。
// トークンは Authorization ヘッダーで送り、URL には入れない。言語は日本語にし、User-Agent で Kotatsu だと分かるようにする。
func TestClientRequests(t *testing.T) {
	t.Parallel()

	server, requests := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/3/search/tv":
			_, _ = w.Write([]byte(`{"page":1,"results":[
				{"id":331304,"name":"もうパパ！","original_name":"もうパパ！","first_air_date":"2026-10-04","genre_ids":[18],"origin_country":["JP"]},
				{"id":0,"name":"broken"}
			]}`))
		case "/3/tv/202396":
			_, _ = w.Write([]byte(`{"id":202396,"name":"晩酌の流儀","original_name":"晩酌の流儀","first_air_date":"2022-07-02","seasons":[
				{"season_number":0,"name":"特別編","air_date":"2022-12-31","episode_count":1},
				{"season_number":7,"name":"シーズン5 ~秋冬編~","air_date":"2026-10-03","episode_count":8}
			]}`))
		case "/3/tv/202396/season/7":
			_, _ = w.Write([]byte(`{"episodes":[
				{"episode_number":7,"air_date":"2026-11-14","name":"ブーリンチー"},
				{"episode_number":8,"air_date":"2026-11-21","name":"第8話"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	})
	client := newClient(server)

	results, err := client.SearchTV(t.Context(), testToken, "もうパパ!")
	require.NoError(t, err)
	assert.Equal(t, []*tmdb.SearchResult{{
		ID: 331304, Name: "もうパパ！", OriginalName: "もうパパ！", FirstAirDate: "2026-10-04", GenreIDs: []int32{18},
	}}, results, "results without an ID are dropped")

	series, err := client.TV(t.Context(), testToken, 202396)
	require.NoError(t, err)
	assert.Equal(t, &tmdb.Series{ID: 202396, Name: "晩酌の流儀", OriginalName: "晩酌の流儀", FirstAirDate: "2022-07-02", Seasons: []*tmdb.Season{
		{SeasonNumber: 0, Name: "特別編", AirDate: "2022-12-31", EpisodeCount: 1},
		{SeasonNumber: 7, Name: "シーズン5 ~秋冬編~", AirDate: "2026-10-03", EpisodeCount: 8},
	}}, series)

	episodes, err := client.Season(t.Context(), testToken, 202396, 7)
	require.NoError(t, err)
	assert.Equal(t, []*tmdb.Episode{
		{EpisodeNumber: 7, AirDate: "2026-11-14", Name: "ブーリンチー"},
		{EpisodeNumber: 8, AirDate: "2026-11-21", Name: "第8話"},
	}, episodes)

	got := requests()
	require.Len(t, got, 3)
	assert.Equal(t, "/3/search/tv", got[0].path)
	assert.Equal(t, "include_adult=false&language=ja-JP&query=%E3%82%82%E3%81%86%E3%83%91%E3%83%91%21", got[0].rawQuery)
	assert.Equal(t, "language=ja-JP", got[1].rawQuery)
	for _, r := range got {
		assert.Equal(t, "Bearer "+testToken, r.authorization)
		assert.NotContains(t, r.rawQuery, testToken, "the token is never put in the URL")
		assert.True(t, strings.HasPrefix(r.userAgent, "Kotatsu/"), r.userAgent)
	}
}

// TestClientErrors は、TMDB が拒否したときのエラーを確認する。エラーの文にはトークンを出さない。
func TestClientErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		check   func(t *testing.T, err error)
	}{
		{
			name: "rate limited with seconds",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusTooManyRequests)
			},
			check: func(t *testing.T, err error) {
				t.Helper()
				var limited *tmdb.RateLimitedError
				require.ErrorAs(t, err, &limited)
				assert.Equal(t, 7*time.Second, limited.RetryAfter)
			},
		},
		{
			name: "rate limited too long",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "9223372036854775807")
				w.WriteHeader(http.StatusTooManyRequests)
			},
			check: func(t *testing.T, err error) {
				t.Helper()
				var limited *tmdb.RateLimitedError
				require.ErrorAs(t, err, &limited)
				assert.Equal(t, time.Hour, limited.RetryAfter, "a long Retry-After is capped")
			},
		},
		{
			name:    "unauthorized",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			check:   func(t *testing.T, err error) { t.Helper(); assert.ErrorIs(t, err, tmdb.ErrUnauthorized) },
		},
		{
			name:    "not found",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			check:   func(t *testing.T, err error) { t.Helper(); assert.ErrorIs(t, err, tmdb.ErrNotFound) },
		},
		{
			name: "redirect is not followed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "https://elsewhere.example/steal", http.StatusFound)
			},
			check: func(t *testing.T, err error) { t.Helper(); assert.ErrorIs(t, err, tmdb.ErrUnexpectedResponse) },
		},
		{
			name: "broken JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":`))
			},
			check: func(t *testing.T, err error) { t.Helper(); assert.ErrorIs(t, err, tmdb.ErrUnexpectedResponse) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newServer(t, tt.handler)
			_, err := newClient(server).TV(t.Context(), testToken, 1)
			require.Error(t, err)
			tt.check(t, err)
			assert.NotContains(t, err.Error(), testToken)
			assert.Len(t, requests(), 1, "redirects are not followed")
		})
	}
}

// TestClientWithoutToken は、トークンが空なら問い合わせないことを確認する。
func TestClientWithoutToken(t *testing.T) {
	t.Parallel()

	server, requests := newServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	_, err := newClient(server).SearchTV(t.Context(), "", "もうパパ!")
	require.ErrorIs(t, err, tmdb.ErrUnauthorized)
	assert.Empty(t, requests())
}
