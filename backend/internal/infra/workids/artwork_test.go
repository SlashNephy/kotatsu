package workids_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/workids"
	"github.com/SlashNephy/kotatsu/internal/testutil/httpclienttest"
)

// TestAniListCoversByMAL は、MyAnimeList の ID に対応する AniList の作品の高画質の画像を、extraLarge を先に選んで返すことを確かめる (#1315)。
func TestAniListCoversByMAL(t *testing.T) {
	t.Parallel()

	server, received, headers := newServer(t, http.StatusOK, nil, `{"data":{"Page":{"media":[
		{"id":1,"idMal":10,"coverImage":{"extraLarge":"https://example.com/xl-10.jpg","large":"https://example.com/l-10.jpg"}},
		{"id":2,"idMal":10,"coverImage":{"extraLarge":"https://example.com/xl-10b.jpg","large":null}},
		{"id":3,"idMal":20,"coverImage":{"extraLarge":" ","large":"https://example.com/l-20.jpg"}},
		{"id":4,"idMal":30,"coverImage":{"extraLarge":null,"large":null}},
		{"id":5,"idMal":null,"coverImage":{"extraLarge":"https://example.com/none.jpg","large":null}},
		null
	],"pageInfo":{"hasNextPage":false}}}}`)
	client := workids.NewClient(&workids.Options{HTTPClient: httpclienttest.New(t), AnnictBaseURL: "", AniListBaseURL: server.URL, JikanBaseURL: ""})

	covers, err := client.AniListCoversByMAL(t.Context(), []int32{10, 20, 30, 40})
	require.NoError(t, err)
	assert.Equal(t, map[int32]string{
		// 1 つの MyAnimeList の作品に複数の作品が対応すれば、ID の小さい作品の画像を使う。
		10: "https://example.com/xl-10.jpg",
		// extraLarge がなければ large を使う。
		20: "https://example.com/l-20.jpg",
	}, covers)
	assert.Empty(t, headers.Get("Authorization"))
	assert.Equal(t, []any{float64(10), float64(20), float64(30), float64(40)}, received.Variables["ids"])

	_, err = client.AniListCoversByMAL(t.Context(), make([]int32, workids.AniListBatchSize+1))
	require.Error(t, err)
}

// TestJikanImage は、Jikan の作品の大きい画像を、JPEG を先に選んで返し、作品がなければ ErrNotFound を返すことを確かめる (#1315)。
func TestJikanImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr error
	}{
		{
			name: "large JPEG", status: http.StatusOK,
			body: `{"data":{"images":{"jpg":{"image_url":"https://cdn.example.com/1.jpg","large_image_url":"https://cdn.example.com/1l.jpg"},` +
				`"webp":{"large_image_url":"https://cdn.example.com/1l.webp"}}}}`,
			want: "https://cdn.example.com/1l.jpg",
		},
		{
			name: "only WebP", status: http.StatusOK,
			body: `{"data":{"images":{"webp":{"image_url":"https://cdn.example.com/1.webp","large_image_url":null}}}}`,
			want: "https://cdn.example.com/1.webp",
		},
		{name: "no images", status: http.StatusOK, body: `{"data":{"images":{}}}`, want: ""},
		{name: "not found", status: http.StatusNotFound, body: `{"status":404}`, wantErr: workids.ErrNotFound},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{}`, wantErr: workids.ErrRateLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v4/anime/52991", r.URL.Path)
				assert.NotEmpty(t, r.Header.Get("User-Agent"))
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)
			client := workids.NewClient(&workids.Options{HTTPClient: httpclienttest.New(t), AnnictBaseURL: "", AniListBaseURL: "", JikanBaseURL: server.URL + "/v4/"})

			got, err := client.JikanImage(t.Context(), 52991)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
