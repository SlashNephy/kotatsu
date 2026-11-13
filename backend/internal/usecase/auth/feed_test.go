package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
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
	member, err := users.Create(t.Context(), db, &auth.NewUser{Username: "calendar", DisplayName: "calendar", Role: domain.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	feeds := auth.NewCalendarFeeds(db, clk, audit.NewRecorder(clk))
	actor := audit.User(member.ID)

	_, err = feeds.Get(t.Context(), member.ID)
	require.ErrorIs(t, err, auth.ErrFeedNotFound, "nothing is issued yet")

	first, feed, err := feeds.Create(t.Context(), actor, member.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(first, "kotatsu_cal_"))
	assert.Equal(t, testNow(), feed.CreatedAt)

	principal, err := feeds.Authenticate(t.Context(), first)
	require.NoError(t, err)
	assert.Equal(t, member.ID, principal.UserID)
	assert.Equal(t, domain.RoleMember, principal.Role)
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
	require.ErrorIs(t, feeds.Delete(t.Context(), actor, member.ID), auth.ErrFeedNotFound)
}

// TestCalendarFeedsDisabledUser は、無効にしたユーザーの URL は使えないことを確かめる (#1255)。
func TestCalendarFeedsDisabledUser(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	user, err := auth.NewUsers(clk).Create(t.Context(), db, &auth.NewUser{Username: "gone", DisplayName: "gone", Role: domain.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	feeds := auth.NewCalendarFeeds(db, clk, audit.NewRecorder(clk))
	token, _, err := feeds.Create(t.Context(), audit.User(user.ID), user.ID)
	require.NoError(t, err)
	_, err = db.Exec(t.Context(), "UPDATE users SET disabled_at = $1 WHERE id = $2", testNow(), user.ID)
	require.NoError(t, err)

	_, err = feeds.Authenticate(t.Context(), token)
	require.ErrorIs(t, err, auth.ErrUnauthenticated)
}

// TestFeedKinds は、予約の iCalendar と IPTV の配信で、秘密の値を分けることを確かめる (#1324)。
// 一方の値で、もう一方を読めず、一方を取り消しても、もう一方は使える。
func TestFeedKinds(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	member, err := auth.NewUsers(clk).Create(t.Context(), db, &auth.NewUser{Username: "iptv", DisplayName: "iptv", Role: domain.RoleMember, PasswordHash: nil})
	require.NoError(t, err)
	calendar := auth.NewCalendarFeeds(db, clk, audit.NewRecorder(clk))
	iptv := auth.NewIPTVFeeds(db, clk, audit.NewRecorder(clk))
	actor := audit.User(member.ID)

	calendarToken, _, err := calendar.Create(t.Context(), actor, member.ID)
	require.NoError(t, err)
	_, err = iptv.Get(t.Context(), member.ID)
	require.ErrorIs(t, err, auth.ErrFeedNotFound, "issuing a calendar feed does not issue an IPTV feed")
	iptvToken, _, err := iptv.Create(t.Context(), actor, member.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(iptvToken, "kotatsu_iptv_"))

	principal, err := iptv.Authenticate(t.Context(), iptvToken)
	require.NoError(t, err)
	assert.Equal(t, member.ID, principal.UserID)
	for name, check := range map[string]func() error{
		"calendar token for IPTV": func() error {
			_, err := iptv.Authenticate(t.Context(), calendarToken)

			return err
		},
		"IPTV token for calendar": func() error {
			_, err := calendar.Authenticate(t.Context(), iptvToken)

			return err
		},
		// 印を付け替えても、ほかの種類の値は受け付けない。
		"calendar token with the IPTV mark": func() error {
			_, err := iptv.Authenticate(t.Context(), "kotatsu_iptv_"+strings.TrimPrefix(calendarToken, "kotatsu_cal_"))

			return err
		},
	} {
		require.ErrorIs(t, check(), auth.ErrUnauthenticated, name)
	}

	require.NoError(t, iptv.Delete(t.Context(), actor, member.ID))
	_, err = calendar.Authenticate(t.Context(), calendarToken)
	require.NoError(t, err, "revoking the IPTV feed keeps the calendar feed")
}
