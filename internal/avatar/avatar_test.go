package avatar_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/avatar"
	"github.com/SlashNephy/kotatsu/internal/clock/clocktest"
	"github.com/SlashNephy/kotatsu/internal/database"
	"github.com/SlashNephy/kotatsu/internal/database/databasetest"
)

func pngImage(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	return buf.Bytes()
}

// newPool は、ユーザーを 1 人持つ DB と、そのユーザーの ID を返す。picture が空でなければ、IdP の picture として紐付ける。
func newPool(t *testing.T, picture string) (*pgxpool.Pool, uuid.UUID) {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	user, err := auth.NewUsers(clocktest.NewFake(now)).Create(t.Context(), pool, &auth.NewUser{
		Username: "kotatsu", DisplayName: "こたつ", Role: auth.RoleViewer, PasswordHash: nil,
	})
	require.NoError(t, err)
	if picture != "" {
		_, err = pool.Exec(t.Context(), `INSERT INTO user_oidc_identities (id, user_id, issuer, subject, picture, created_at, last_used_at)
			VALUES ($1, $2, 'https://idp.example', 'sub', $3, $4, $4)`, uuid.New(), user.ID, picture, now)
		require.NoError(t, err)
	}

	return pool, user.ID
}

// TestAvatarHandler は、IdP の画像を中継し、LAN の中の、IdP でないホストへは取りに行かないことを確かめる (#778)。
func TestAvatarHandler(t *testing.T) {
	t.Parallel()

	img := pngImage(t)
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/avatar.png":
			_, _ = w.Write(img)
		case "/page.html":
			_, _ = w.Write([]byte("<html><script>alert(1)</script></html>"))
		case "/large.png":
			_, _ = w.Write(append(img, make([]byte, 1<<20)...))
		case "/redirect":
			http.Redirect(w, r, "/avatar.png", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(idp.Close)

	tests := []struct {
		name    string
		picture string
		issuer  string
		want    int
	}{
		{name: "IdP と同じホスト", picture: idp.URL + "/avatar.png", issuer: idp.URL, want: http.StatusOK},
		{name: "LAN の中の、IdP でないホスト", picture: idp.URL + "/avatar.png", issuer: "https://idp.example", want: http.StatusBadGateway},
		{name: "画像でない", picture: idp.URL + "/page.html", issuer: idp.URL, want: http.StatusBadGateway},
		{name: "大きすぎる", picture: idp.URL + "/large.png", issuer: idp.URL, want: http.StatusBadGateway},
		{name: "リダイレクトには従わない", picture: idp.URL + "/redirect", issuer: idp.URL, want: http.StatusBadGateway},
		{name: "IdP が返さない", picture: idp.URL + "/missing.png", issuer: idp.URL, want: http.StatusBadGateway},
		{name: "picture がない", picture: "", issuer: idp.URL, want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			pool, userID := newPool(t, tt.picture)
			mux := http.NewServeMux()
			mux.Handle("GET /media/users/{id}/avatar", avatar.NewHandler(pool, tt.issuer, slog.New(slog.DiscardHandler)))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/media/users/"+userID.String()+"/avatar", http.NoBody))
			assert.Equal(t, tt.want, rec.Code)
			if tt.want == http.StatusOK {
				assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
				assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
				assert.Equal(t, img, rec.Body.Bytes())
			} else {
				assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
}
