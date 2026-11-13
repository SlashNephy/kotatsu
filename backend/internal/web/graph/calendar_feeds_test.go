package graph_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const (
	calendarFeedQuery  = `{ calendarFeed { createdAt lastUsedAt } }`
	createFeedMutation = `mutation { createCalendarFeed { path calendarFeed { createdAt } } }`
	deleteFeedMutation = `mutation { deleteCalendarFeed }`
)

type calendarFeed struct {
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func (f *fixture) calendarFeed(t *testing.T) *calendarFeed {
	t.Helper()

	var got struct {
		CalendarFeed *calendarFeed `json:"calendarFeed"`
	}
	require.Empty(t, f.query(t, calendarFeedQuery, nil, &got))

	return got.CalendarFeed
}

// TestManageCalendarFeed は、ログイン中のユーザーが、予約の購読用の URL を発行し、発行し直し、取り消せることを確かめる (#1255)。
func TestManageCalendarFeed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	assert.Nil(t, f.calendarFeed(t), "nothing is issued yet")

	issue := func() string {
		t.Helper()

		var created struct {
			CreateCalendarFeed struct {
				Path string `json:"path"`
			} `json:"createCalendarFeed"`
		}
		require.Empty(t, f.query(t, createFeedMutation, nil, &created))

		return created.CreateCalendarFeed.Path
	}
	first := issue()
	assert.True(t, strings.HasPrefix(first, "/api/calendar/kotatsu_cal_"), first)
	assert.True(t, strings.HasSuffix(first, "/reservations.ics"), first)
	require.NotNil(t, f.calendarFeed(t))
	// ほかのユーザーからは、自分の URL は見えない。
	assert.Nil(t, f.as(t, otherUser, domain.RoleMember).calendarFeed(t))

	// 発行し直すと、別の URL になる。
	assert.NotEqual(t, first, issue())

	var deleted struct {
		DeleteCalendarFeed bool `json:"deleteCalendarFeed"`
	}
	require.Empty(t, f.query(t, deleteFeedMutation, nil, &deleted))
	assert.True(t, deleted.DeleteCalendarFeed)
	assert.Nil(t, f.calendarFeed(t))
	assert.Equal(t, []string{codeNotFound}, f.query(t, deleteFeedMutation, nil, nil), "nothing to revoke")
}

// TestCreateCalendarFeedRequiresRecentAuthentication は、発行に直近の認証が要ることを確かめる (#1255)。
func TestCreateCalendarFeedRequiresRecentAuthentication(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, f.query(t, createFeedMutation, nil, nil))
	assert.Nil(t, f.calendarFeed(t))
}
