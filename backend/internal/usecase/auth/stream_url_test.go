package auth_test

import (
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestStreamURLsKeepStreamingURLs は、使われた日時を残し、期限を過ぎても流れ続けている URL の記録は消さずに一覧に出し、
// 期限が過ぎて使われなくなった記録は、次に発行するときに消すことを確かめる (#1757)。
func TestStreamURLsKeepStreamingURLs(t *testing.T) {
	t.Parallel()

	f := newSessionFixture(t, newDB(t))
	users := auth.NewURLUsers(f.db, f.clock, slog.New(slog.DiscardHandler))
	urls := auth.NewStreamURLs(f.db, f.clock, audit.NewRecorder(f.clock), users)
	listed := func(t *testing.T) map[uuid.UUID]*auth.IssuedStreamURL {
		t.Helper()

		issued, err := urls.List(t.Context())
		require.NoError(t, err)
		byID := map[uuid.UUID]*auth.IssuedStreamURL{}
		for _, url := range issued {
			byID[url.ID] = url
		}

		return byID
	}

	streaming := issueStreamURL(t, f)
	idle := issueStreamURL(t, f)
	// 開いたときに、使われた日時を残す。
	_, err := users.Role(t.Context(), streaming, f.userID)
	require.NoError(t, err)
	opened := f.clock.Now()
	got := listed(t)
	require.Contains(t, got, streaming)
	require.NotNil(t, got[streaming].LastUsedAt)
	assert.True(t, opened.Equal(*got[streaming].LastUsedAt))
	assert.Nil(t, got[idle].LastUsedAt, "the idle URL is not opened")

	// 期限 (1 時間) を過ぎても流れ続け、確かめ直すたびに、使われた日時を書き直す。
	f.clock.Advance(time.Hour + time.Minute)
	_, err = users.Role(t.Context(), streaming, f.userID)
	require.NoError(t, err)
	next := issueStreamURL(t, f)
	got = listed(t)
	assert.Contains(t, got, streaming, "the URL still streaming after it expired is listed")
	assert.Contains(t, got, next)
	assert.NotContains(t, got, idle, "the expired idle URL is not listed")
	_, err = users.Role(t.Context(), idle, f.userID)
	require.ErrorIs(t, err, auth.ErrUserUnusable, "the expired idle URL is deleted when a new URL is issued")

	// 流すのをやめて StreamURLActiveWindow が過ぎたら、次に発行するときに消す。
	f.clock.Advance(auth.StreamURLActiveWindow + time.Minute)
	issueStreamURL(t, f)
	assert.NotContains(t, listed(t), streaming)
	_, err = users.Role(t.Context(), streaming, f.userID)
	require.ErrorIs(t, err, auth.ErrUserUnusable)
}
