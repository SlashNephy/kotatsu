package animerecord_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
)

// TestImportToken は、作品 ID を引くための Annict のトークンを暗号化して保存し、取り出せること、
// 空にすれば消えること、監査ログに値を残さないことを確かめる (#1223)。
func TestImportToken(t *testing.T) {
	t.Parallel()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(testNow())
	service, err := animerecord.NewService(&animerecord.Options{
		Clock: clk, Recorder: audit.NewRecorder(clk), AppSecret: strings.Repeat("k", 32), Destinations: nil,
		Logger: slog.New(slog.DiscardHandler), Inserter: nil, WishlistInterval: 0,
	})
	require.NoError(t, err)
	actor := audit.User(uuid.New())

	token, err := service.AnnictImportToken(t.Context(), pool)
	require.NoError(t, err)
	assert.Empty(t, token)

	require.NoError(t, service.UpdateImportToken(t.Context(), pool, actor, "  annict-token  "))
	token, err = service.AnnictImportToken(t.Context(), pool)
	require.NoError(t, err)
	assert.Equal(t, "annict-token", token)
	status, err := service.ImportToken(t.Context(), pool)
	require.NoError(t, err)
	assert.True(t, status.Configured)
	var stored []byte
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT annict_token_encrypted FROM anime_import_tokens`).Scan(&stored))
	assert.NotContains(t, string(stored), "annict-token")

	err = service.UpdateImportToken(t.Context(), pool, actor, strings.Repeat("a", 257))
	require.ErrorIs(t, err, animerecord.ErrInvalidImportToken)

	require.NoError(t, service.UpdateImportToken(t.Context(), pool, actor, ""))
	status, err = service.ImportToken(t.Context(), pool)
	require.NoError(t, err)
	assert.False(t, status.Configured)

	var actions []string
	rows, err := pool.Query(t.Context(), `SELECT action || ' ' || summary::text FROM audit_events WHERE target_type = 'anime_import_token' ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"anime_import_token.update {}", "anime_import_token.delete {}"}, actions)
}
