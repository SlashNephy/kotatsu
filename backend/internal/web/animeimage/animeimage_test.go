package animeimage_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/animeimage"
)

// imagePath は、上流のサーバーが画像を返すパス。
const imagePath = "/image.png"

func pngImage(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

// TestHandler は、ログイン中のユーザーの見たいリストにある作品の画像だけを中継し、中身が画像でなければ断り、取ってきた画像を覚えることを確かめる (#1235)。
func TestHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// imagePath は、見たいリストの作品の画像の、上流のサーバーでのパス。空なら http でも https でもない URL にする。
		imagePath string
		// workID は、頼む作品。見たいリストにあるのは 10 だけ。
		workID     string
		signedIn   bool
		wantStatus int
		// wantFetches は、2 回頼んだときに、上流のサーバーへ問い合わせた回数。
		wantFetches int64
	}{
		{name: "画像を中継し、2 回目は覚えた画像を返す", imagePath: imagePath, workID: "10", signedIn: true, wantStatus: http.StatusOK, wantFetches: 1},
		{name: "リダイレクトをたどる", imagePath: "/moved", workID: "10", signedIn: true, wantStatus: http.StatusOK, wantFetches: 2},
		{name: "見たいリストにない作品は返さない", imagePath: imagePath, workID: "11", signedIn: true, wantStatus: http.StatusNotFound, wantFetches: 0},
		{name: "SVG は画像とみなさずに断る", imagePath: "/image.svg", workID: "10", signedIn: true, wantStatus: http.StatusBadGateway, wantFetches: 2},
		{name: "中身が画像でなければ断る", imagePath: "/page.html", workID: "10", signedIn: true, wantStatus: http.StatusBadGateway, wantFetches: 2},
		{name: "http でも https でもない URL は取りに行かない", imagePath: "", workID: "10", signedIn: true, wantStatus: http.StatusBadGateway, wantFetches: 0},
		{name: "ログインしていなければ返さない", imagePath: imagePath, workID: "10", signedIn: false, wantStatus: http.StatusUnauthorized, wantFetches: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var fetches atomic.Int64
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fetches.Add(1)
				if r.URL.Path == "/moved" {
					http.Redirect(w, r, imagePath, http.StatusMovedPermanently)

					return
				}
				if r.URL.Path == imagePath {
					_, _ = w.Write(pngImage(t))

					return
				}
				w.Header().Set("Content-Type", "image/svg+xml")
				if r.URL.Path == "/image.svg" {
					_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))

					return
				}
				_, _ = w.Write([]byte("<html><body>not an image</body></html>"))
			}))
			t.Cleanup(upstream.Close)
			imageURL := "ftp://example.com/image.png"
			if tt.imagePath != "" {
				imageURL = upstream.URL + tt.imagePath
			}
			pool, err := database.Open(t.Context(), databasetest.New(t))
			require.NoError(t, err)
			t.Cleanup(pool.Close)
			require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			user, err := auth.NewUsers(clocktest.NewFake(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))).Create(t.Context(), pool, &auth.NewUser{
				Username: "kotatsu", DisplayName: "こたつ", Role: auth.RoleViewer, PasswordHash: nil,
			})
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO anime_record_connections (user_id, destination, access_token_encrypted, account_name, connected_at)
				VALUES ($1, 'anilist', '\x00', 'user', now())`, user.ID)
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO anime_wishlist_entries (user_id, destination, work_id, status, title, image_url, fetched_at)
				VALUES ($1, 'anilist', 10, 'watching', '作品', $2, now())`, user.ID, imageURL)
			require.NoError(t, err)
			handler := animeimage.NewHandlerWithClient(pool, slog.New(slog.DiscardHandler), upstream.Client())
			mux := http.NewServeMux()
			mux.Handle("GET /media/anime-works/{destination}/{workId}/image", handler)

			for range 2 {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/media/anime-works/anilist/"+tt.workID+"/image", http.NoBody)
				if tt.signedIn {
					req = req.WithContext(auth.WithPrincipal(req.Context(), &auth.Principal{
						UserID: user.ID, Role: auth.RoleViewer, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: "",
					}))
				}
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				require.Equal(t, tt.wantStatus, rec.Code)
				if tt.wantStatus == http.StatusOK {
					assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
					assert.Equal(t, pngImage(t), rec.Body.Bytes())
				} else {
					assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"), "failures are not cached by the browser")
				}
			}
			assert.Equal(t, tt.wantFetches, fetches.Load())
		})
	}
}

// TestServeFollowingAvatar は、ログイン中のユーザーが取り込んだ視聴状況にあるアバターだけを中継することを確かめる (#1372)。
func TestServeFollowingAvatar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		username   string
		signedIn   bool
		wantStatus int
	}{
		{name: "取り込んだ人のアバターを中継する", username: "someone", signedIn: true, wantStatus: http.StatusOK},
		{name: "取り込んでいない人のアバターは返さない", username: "stranger", signedIn: true, wantStatus: http.StatusNotFound},
		{name: "保持する期間を過ぎたアバターは返さない", username: "expired", signedIn: true, wantStatus: http.StatusNotFound},
		{name: "ログインしていなければ返さない", username: "someone", signedIn: false, wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(pngImage(t))
			}))
			t.Cleanup(upstream.Close)
			pool, err := database.Open(t.Context(), databasetest.New(t))
			require.NoError(t, err)
			t.Cleanup(pool.Close)
			require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
			user, err := auth.NewUsers(clocktest.NewFake(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))).Create(t.Context(), pool, &auth.NewUser{
				Username: "kotatsu", DisplayName: "こたつ", Role: auth.RoleViewer, PasswordHash: nil,
			})
			require.NoError(t, err)
			for _, statement := range []string{
				`INSERT INTO anime_record_connections (user_id, destination, access_token_encrypted, account_name, connected_at) VALUES ($1, 'annict', '\x00', 'user', now())`,
				`INSERT INTO anime_following_fetches (user_id, destination, syobocal_tid, checked_at, fetched_at) VALUES ($1, 'annict', 1, now(), now())`,
			} {
				_, err = pool.Exec(t.Context(), statement, user.ID)
				require.NoError(t, err)
			}
			_, err = pool.Exec(t.Context(), `INSERT INTO anime_following_viewings (user_id, destination, syobocal_tid, username, name, avatar_url, status)
				VALUES ($1, 'annict', 1, 'someone', '人', $2, 'watching')`, user.ID, upstream.URL+imagePath)
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO anime_following_fetches (user_id, destination, syobocal_tid, checked_at, fetched_at)
				VALUES ($1, 'annict', 2, now() - interval '25 hours', now() - interval '25 hours')`, user.ID)
			require.NoError(t, err)
			_, err = pool.Exec(t.Context(), `INSERT INTO anime_following_viewings (user_id, destination, syobocal_tid, username, name, avatar_url, status)
				VALUES ($1, 'annict', 2, 'expired', '人', $2, 'watching')`, user.ID, upstream.URL+imagePath)
			require.NoError(t, err)
			handler := animeimage.NewHandlerWithClient(pool, slog.New(slog.DiscardHandler), upstream.Client())
			mux := http.NewServeMux()
			mux.HandleFunc("GET /media/anime-following/{destination}/{username}/avatar", handler.ServeFollowingAvatar)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/media/anime-following/annict/"+tt.username+"/avatar", http.NoBody)
			if tt.signedIn {
				req = req.WithContext(auth.WithPrincipal(req.Context(), &auth.Principal{
					UserID: user.ID, Role: auth.RoleViewer, SessionID: uuid.Nil(), AuthenticatedAt: time.Time{}, OAuthClientID: "",
				}))
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestPath は、画像の URL が変わったら、中継の URL も変わることを確かめる。
func TestPath(t *testing.T) {
	t.Parallel()
	a := animeimage.Path("anilist", 10, "https://example.com/a.png")
	assert.Equal(t, a, animeimage.Path("anilist", 10, "https://example.com/a.png"))
	assert.NotEqual(t, a, animeimage.Path("anilist", 10, "https://example.com/b.png"))
	assert.Contains(t, a, "/media/anime-works/anilist/10/image?v=")
}

// TestCache は、覚えた画像が上限を超えたら、古く覚えたものから忘れ、上限より大きい画像は覚えないことを確かめる。
func TestCache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		sizes []int
		want  []string
	}{
		{name: "上限までは覚える", sizes: []int{3, 3, 4}, want: []string{"a", "b", "c"}},
		{name: "超えたら古いものから忘れる", sizes: []int{3, 3, 4, 5}, want: []string{"c", "d"}},
		{name: "上限より大きい画像は覚えない", sizes: []int{3, 11}, want: []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, animeimage.CacheKeys(10, tt.sizes))
		})
	}
}
