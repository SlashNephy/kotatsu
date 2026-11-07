package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// TestCalendarFeeds は、予約の購読用の URL の秘密の値を発行し、そのユーザーのロールのまま認証し、
// 発行し直すと前の値が使えなくなり、取り消すと使えなくなることを確かめる (#1255)。
func TestCalendarFeeds(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	users := auth.NewUsers(clk)
	member, err := users.Create(t.Context(), db, &auth.NewUser{Username: "calendar", DisplayName: "calendar", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	feeds := auth.NewCalendarFeeds(db, clk, audit.NewRecorder(clk))
	actor := audit.User(member.ID)

	_, err = feeds.Get(t.Context(), member.ID)
	require.ErrorIs(t, err, auth.ErrCalendarFeedNotFound, "nothing is issued yet")

	first, feed, err := feeds.Create(t.Context(), actor, member.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(first, "kotatsu_cal_"))
	assert.Equal(t, testNow(), feed.CreatedAt)

	principal, err := feeds.Authenticate(t.Context(), first)
	require.NoError(t, err)
	assert.Equal(t, member.ID, principal.UserID)
	assert.Equal(t, auth.RoleMember, principal.Role)
	// セッションを持たないので、再認証の要る操作には使えない。
	require.ErrorIs(t, auth.RequireRecentAuthentication(auth.WithPrincipal(t.Context(), principal), clk), auth.ErrReauthenticationRequired)
	got, err := feeds.Get(t.Context(), member.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.Equal(t, testNow(), got.LastUsedAt.UTC())

	// 発行し直すと、前の値は使えなくなる。
	clk.Advance(time.Hour)
	second, _, err := feeds.Create(t.Context(), actor, member.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	_, err = feeds.Authenticate(t.Context(), first)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	got, err = feeds.Get(t.Context(), member.ID)
	require.NoError(t, err)
	assert.Equal(t, testNow().Add(time.Hour), got.CreatedAt.UTC())
	assert.Nil(t, got.LastUsedAt, "the reissued feed has not been read yet")

	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "API token mark", token: "kotatsu_" + strings.TrimPrefix(second, "kotatsu_cal_")},
		{name: "never issued", token: "kotatsu_cal_" + strings.ToUpper(strings.TrimPrefix(second, "kotatsu_cal_"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := feeds.Authenticate(t.Context(), tt.token)
			require.ErrorIs(t, err, auth.ErrUnauthenticated)
		})
	}

	// 取り消すと使えなくなる。2 回目は見つからない。
	require.NoError(t, feeds.Delete(t.Context(), actor, member.ID))
	_, err = feeds.Authenticate(t.Context(), second)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
	require.ErrorIs(t, feeds.Delete(t.Context(), actor, member.ID), auth.ErrCalendarFeedNotFound)
}

// TestCalendarFeedsDisabledUser は、無効にしたユーザーの URL は使えないことを確かめる (#1255)。
func TestCalendarFeedsDisabledUser(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	user, err := auth.NewUsers(clk).Create(t.Context(), db, &auth.NewUser{Username: "gone", DisplayName: "gone", Role: auth.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	feeds := auth.NewCalendarFeeds(db, clk, audit.NewRecorder(clk))
	token, _, err := feeds.Create(t.Context(), audit.User(user.ID), user.ID)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "UPDATE users SET disabled_at = $1 WHERE id = $2", testNow(), user.ID)
	require.NoError(t, err)

	_, err = feeds.Authenticate(t.Context(), token)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
}
