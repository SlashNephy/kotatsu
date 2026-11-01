package animerecordapi_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/animerecordapi"
)

// TestCallbackHandler は、記録先から戻ってきたリクエストを、設定の画面へ戻し、失敗の理由だけを伝えることを確かめる (#927)。
// state を知らない、ログインしていない、などで連携できなければ、トークンなどの中身を入れずに error=<理由> を付ける。
func TestCallbackHandler(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	service, err := animerecord.NewService(&animerecord.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32),
		Destinations: []animerecord.Destination{animerecord.NewAnnict(animerecord.NewHTTPClient(), "http://127.0.0.1:9")},
		Logger:       slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	client := func(*http.Request) *auth.ClientInfo {
		return &auth.ClientInfo{UserAgent: "test", SourceIP: netip.MustParseAddr("127.0.0.1"), Host: "kotatsu.test", Secure: false}
	}
	handler := animerecordapi.CallbackHandler(service, pool, client, animerecord.DestinationAnnict, slog.New(slog.DiscardHandler))

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "no state", query: "code=abc", want: "/settings/account?error=anime_record_expired"},
		{name: "an unknown state", query: "state=forged&code=abc", want: "/settings/account?error=anime_record_expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/auth/anime-records/annict/callback?"+tt.query, nil))
			assert.Equal(t, http.StatusSeeOther, recorder.Code)
			assert.Equal(t, tt.want, recorder.Header().Get("Location"))
		})
	}
}
