package workids_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/workids"
)

// cursorAfter は、続きを引くときに渡す cursor。
const cursorAfter = "after"

// graphQLRequest は、偽の API が受け取った GraphQL のリクエスト。
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// newServer は、/graphql に status と body を返す偽の API を立て、受け取ったリクエストとヘッダーを記録する。
func newServer(t *testing.T, status int, header map[string]string, body string) (*httptest.Server, *graphQLRequest, *http.Header) {
	t.Helper()

	received := &graphQLRequest{Query: "", Variables: nil}
	headers := &http.Header{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/graphql", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		*headers = r.Header.Clone()
		assert.NoError(t, json.NewDecoder(r.Body).Decode(received))
		for key, value := range header {
			w.Header().Set(key, value)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server, received, headers
}

func TestAnnictWorks(t *testing.T) {
	t.Parallel()

	server, received, headers := newServer(t, http.StatusOK, nil, `{"data":{"searchWorks":{
		"nodes":[
			{"annictId":1,"syobocalTid":274,"malAnimeId":" 318 "},
			{"annictId":2,"syobocalTid":0,"malAnimeId":"abc"},
			{"annictId":3,"syobocalTid":null,"malAnimeId":null},
			null
		],
		"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}`)
	client := workids.NewClient(&workids.Options{HTTPClient: nil, AnnictBaseURL: server.URL + "/", AniListBaseURL: ""})

	page, err := client.AnnictWorks(t.Context(), "secret", []string{"2026-autumn", "2027-winter"}, cursorAfter)
	require.NoError(t, err)
	assert.Equal(t, &workids.AnnictPage{
		Works: []*workids.AnnictWork{
			{AnnictID: 1, SyobocalTID: new(int32(274)), MALAnimeID: new(int32(318))},
			// 0 の TID と、数でない MyAnimeList の ID は、ないものとする。
			{AnnictID: 2, SyobocalTID: nil, MALAnimeID: nil},
			{AnnictID: 3, SyobocalTID: nil, MALAnimeID: nil},
		},
		Next: "next",
	}, page)
	assert.Equal(t, "Bearer secret", headers.Get("Authorization"))
	assert.NotEmpty(t, headers.Get("User-Agent"))
	assert.Equal(t, map[string]any{"seasons": []any{"2026-autumn", "2027-winter"}, "first": float64(workids.AnnictPageSize), "after": cursorAfter}, received.Variables)
}

func TestAnnictWorksLastPage(t *testing.T) {
	t.Parallel()

	server, received, _ := newServer(t, http.StatusOK, nil, `{"data":{"searchWorks":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":"x"}}}}`)
	client := workids.NewClient(&workids.Options{HTTPClient: nil, AnnictBaseURL: server.URL, AniListBaseURL: ""})

	page, err := client.AnnictWorks(t.Context(), "secret", nil, "")
	require.NoError(t, err)
	assert.Equal(t, &workids.AnnictPage{Works: []*workids.AnnictWork{}, Next: ""}, page)
	// 時期も cursor も渡さなければ、すべての作品の最初のページを引く。
	assert.Equal(t, map[string]any{"seasons": nil, "first": float64(workids.AnnictPageSize), "after": nil}, received.Variables)
}

func TestAnnictWorksErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		header     map[string]string
		body       string
		wantErr    error
		retryAfter time.Duration
	}{
		{name: "429 は Retry-After を返す", status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "30"}, body: "", wantErr: workids.ErrRateLimited, retryAfter: 30 * time.Second},
		{name: "読めない Retry-After は 0", status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "soon"}, body: "", wantErr: workids.ErrRateLimited, retryAfter: 0},
		{name: "401 はトークンを受け付けなかった", status: http.StatusUnauthorized, header: nil, body: "", wantErr: workids.ErrUnauthorized, retryAfter: 0},
		{name: "GraphQL のエラー", status: http.StatusOK, header: nil, body: `{"errors":[{"message":"boom"}]}`, wantErr: nil, retryAfter: 0},
		{name: "500", status: http.StatusInternalServerError, header: nil, body: "", wantErr: nil, retryAfter: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server, _, _ := newServer(t, tt.status, tt.header, tt.body)
			client := workids.NewClient(&workids.Options{HTTPClient: nil, AnnictBaseURL: server.URL, AniListBaseURL: ""})

			_, err := client.AnnictWorks(t.Context(), "secret", nil, "")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "secret")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			if errors.Is(tt.wantErr, workids.ErrRateLimited) {
				var rateLimit *workids.RateLimitError
				require.ErrorAs(t, err, &rateLimit)
				assert.Equal(t, tt.retryAfter, rateLimit.RetryAfter)
			}
		})
	}
}

func TestAniListIDsByMAL(t *testing.T) {
	t.Parallel()

	server, received, headers := newServer(t, http.StatusOK, nil, `{"data":{"Page":{"media":[
		{"id":318,"idMal":318},
		{"id":9000,"idMal":5114},
		{"id":5114,"idMal":5114},
		{"id":1,"idMal":null},
		null
	]}}}`)
	client := workids.NewClient(&workids.Options{HTTPClient: nil, AnnictBaseURL: "", AniListBaseURL: server.URL})

	found, err := client.AniListIDsByMAL(t.Context(), []int32{318, 5114, 7})
	require.NoError(t, err)
	// 1 つの MyAnimeList の作品に複数の作品が対応すれば、小さい ID を使い、見つからない ID は含めない。
	assert.Equal(t, map[int32]int32{318: 318, 5114: 5114}, found)
	assert.Empty(t, headers.Get("Authorization"))
	assert.Equal(t, []any{float64(318), float64(5114), float64(7)}, received.Variables["ids"])
}

// TestAniListIDsByMALPages は、対応する作品が 1 ページに収まらなければ、続きのページも引くことを確かめる。
func TestAniListIDsByMALPages(t *testing.T) {
	t.Parallel()

	pages := []string{
		`{"data":{"Page":{"media":[{"id":1,"idMal":10}],"pageInfo":{"hasNextPage":true}}}}`,
		`{"data":{"Page":{"media":[{"id":2,"idMal":20}],"pageInfo":{"hasNextPage":false}}}}`,
	}
	requested := []any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request graphQLRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		requested = append(requested, request.Variables["page"])
		_, _ = w.Write([]byte(pages[len(requested)-1]))
	}))
	t.Cleanup(server.Close)
	client := workids.NewClient(&workids.Options{HTTPClient: nil, AnnictBaseURL: "", AniListBaseURL: server.URL})

	found, err := client.AniListIDsByMAL(t.Context(), []int32{10, 20})
	require.NoError(t, err)
	assert.Equal(t, map[int32]int32{10: 1, 20: 2}, found)
	assert.Equal(t, []any{float64(1), float64(2)}, requested)
}

func TestAniListIDsByMALTooMany(t *testing.T) {
	t.Parallel()

	client := workids.NewClient(&workids.Options{HTTPClient: nil, AnnictBaseURL: "", AniListBaseURL: "http://127.0.0.1:1"})
	_, err := client.AniListIDsByMAL(t.Context(), make([]int32, workids.AniListBatchSize+1))
	require.Error(t, err)
}
