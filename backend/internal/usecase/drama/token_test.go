package drama_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
)

// TestTokens は、管理者が入れた TMDB のトークンを暗号化して保存し、照合とログの報告のために取り出せること、
// 画面には末尾の数文字だけを出すこと、空にすれば消えること、監査ログに値を残さないことを確かめる (#1651)。
func TestTokens(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(jst(15, 0, 0))
	tokens := newTokens(t, clk)
	const entered = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ0ZXN0In0.signature-abcd"

	status, err := tokens.Status(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, &drama.TokenStatus{Configured: false, Hint: "", UpdatedAt: nil}, status)
	secrets, err := tokens.LogSecrets(t.Context(), db)
	require.NoError(t, err)
	assert.Empty(t, secrets)

	setToken(t, db, tokens, "  "+entered+"\n")
	got, err := tokens.Token(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, entered, got, "surrounding spaces are trimmed")
	status, err = tokens.Status(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, &drama.TokenStatus{Configured: true, Hint: "abcd", UpdatedAt: new(clk.Now())}, status)
	secrets, err = tokens.LogSecrets(t.Context(), db)
	require.NoError(t, err)
	assert.Equal(t, []string{entered}, secrets, "the log report hides the token")
	var stored []byte
	require.NoError(t, db.QueryRow(t.Context(), `SELECT token_encrypted FROM tmdb_tokens`).Scan(&stored))
	assert.NotContains(t, string(stored), entered)

	setToken(t, db, tokens, "short-token")
	status, err = tokens.Status(t.Context(), db)
	require.NoError(t, err)
	assert.True(t, status.Configured)
	assert.Empty(t, status.Hint, "a short token shows no characters")

	setToken(t, db, tokens, "")
	status, err = tokens.Status(t.Context(), db)
	require.NoError(t, err)
	assert.False(t, status.Configured)
	setToken(t, db, tokens, "")

	var actions []string
	rows, err := db.Query(t.Context(), `SELECT action || ' ' || summary::text FROM audit_events WHERE target_type = 'tmdb_token' ORDER BY id`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var action string
		require.NoError(t, rows.Scan(&action))
		actions = append(actions, action)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{
		"drama.tmdb_token_updated {}", "drama.tmdb_token_updated {}", "drama.tmdb_token_deleted {}",
	}, actions, "deleting a missing token is not recorded")
}

// TestTokensInvalid は、Authorization ヘッダーに入れられないトークンと、長すぎるトークンを断り、前のトークンを残すことを確かめる。
func TestTokensInvalid(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(jst(15, 0, 0))
	tokens := newTokens(t, clk)
	setToken(t, db, tokens, testTMDbKey)

	tests := []struct {
		name  string
		value string
	}{
		{name: "too long", value: strings.Repeat("a", 2049)},
		{name: "space inside", value: "test token"},
		{name: "line break inside", value: "test\r\nX-Injected: 1"},
		{name: "not ASCII", value: "テスト"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tokens.Update(t.Context(), db, audit.System("test"), tt.value)
			require.ErrorIs(t, err, drama.ErrInvalidToken)
			got, err := tokens.Token(t.Context(), db)
			require.NoError(t, err)
			assert.Equal(t, testTMDbKey, got, "the previous token is kept")
		})
	}
}
