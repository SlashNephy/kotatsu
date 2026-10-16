package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/auth"
)

// TestPruneSessions は、失効させた、絶対的な期限を過ぎた、無操作の期限を過ぎたセッションを消し、使えるセッションは残すことを確かめる。
func TestPruneSessions(t *testing.T) {
	t.Parallel()

	f := newLoginFixture(t)
	create := func() string {
		_, session, err := f.sessions.Create(t.Context(), f.db, f.user.ID, f.client)
		require.NoError(t, err)

		return session.ID.String()
	}
	// 無操作の期限を過ぎるもの。
	idle := create()
	f.clock.Advance(auth.DefaultIdleTimeout - time.Hour)
	active := create()
	revoked := create()
	_, err := f.db.Exec(t.Context(), "UPDATE sessions SET revoked_at = $1 WHERE id = $2", f.clock.Now(), revoked)
	require.NoError(t, err)
	expired := create()
	_, err = f.db.Exec(t.Context(), "UPDATE sessions SET expires_at = $1 WHERE id = $2", f.clock.Now(), expired)
	require.NoError(t, err)
	f.clock.Advance(2 * time.Hour)

	deleted, err := auth.PruneSessions(t.Context(), f.db, f.clock)
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	rows, err := f.db.Query(t.Context(), "SELECT id::text FROM sessions ORDER BY id")
	require.NoError(t, err)
	remaining := []string{}
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		remaining = append(remaining, id)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{active}, remaining)
	assert.NotContains(t, remaining, idle)
}
