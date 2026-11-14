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
	iptvFeedQuery          = `{ iptvFeed { createdAt lastUsedAt } calendarFeed { createdAt } }`
	createIptvFeedMutation = `mutation { createIptvFeed { playlistPath guidePath iptvFeed { createdAt } } }`
	deleteIptvFeedMutation = `mutation { deleteIptvFeed }`
	// enableIptvFeeds と disableIptvFeeds は、管理者が IPTV のクライアント用の URL を有効にするか無効にする (#1755)。
	enableIptvFeeds  = `mutation { updateSystemSettings(input: {iptvFeedsEnabled: true}) { iptvFeedsEnabled } }`
	disableIptvFeeds = `mutation { updateSystemSettings(input: {iptvFeedsEnabled: false}) { iptvFeedsEnabled } }`
)

type feeds struct {
	IptvFeed     *calendarFeed `json:"iptvFeed"`
	CalendarFeed *calendarFeed `json:"calendarFeed"`
}

func (f *fixture) feeds(t *testing.T) *feeds {
	t.Helper()

	var got feeds
	require.Empty(t, f.query(t, iptvFeedQuery, nil, &got))

	return &got
}

// TestManageIptvFeed は、ログイン中のユーザーが、IPTV の配信の URL を、予約の iCalendar と別に発行し、発行し直し、取り消せることを確かめる (#1324)。
func TestManageIptvFeed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	require.Empty(t, f.as(t, "iptv-admin", domain.RoleAdmin).query(t, enableIptvFeeds, nil, nil))
	assert.Nil(t, f.feeds(t).IptvFeed, "nothing is issued yet")

	issue := func() (string, string) {
		t.Helper()

		var created struct {
			CreateIptvFeed struct {
				PlaylistPath string `json:"playlistPath"`
				GuidePath    string `json:"guidePath"`
			} `json:"createIptvFeed"`
		}
		require.Empty(t, f.query(t, createIptvFeedMutation, nil, &created))

		return created.CreateIptvFeed.PlaylistPath, created.CreateIptvFeed.GuidePath
	}
	playlist, guide := issue()
	token := strings.TrimSuffix(strings.TrimPrefix(playlist, "/api/iptv/"), "/channels.m3u")
	assert.True(t, strings.HasPrefix(token, "kotatsu_iptv_"), playlist)
	assert.Equal(t, "/api/iptv/"+token+"/guide.xml", guide)
	got := f.feeds(t)
	require.NotNil(t, got.IptvFeed)
	assert.Nil(t, got.CalendarFeed, "issuing an IPTV feed does not issue a calendar feed")
	// ほかのユーザーからは、自分の URL は見えない。
	assert.Nil(t, f.as(t, otherUser, domain.RoleMember).feeds(t).IptvFeed)
	// 視聴者も、ライブ視聴を見られるので発行できる。
	viewer := f.as(t, "iptv-viewer", domain.RoleViewer)
	require.Empty(t, viewer.query(t, createIptvFeedMutation, nil, nil))

	newPlaylist, _ := issue()
	assert.NotEqual(t, playlist, newPlaylist)

	var deleted struct {
		DeleteIptvFeed bool `json:"deleteIptvFeed"`
	}
	require.Empty(t, f.query(t, deleteIptvFeedMutation, nil, &deleted))
	assert.True(t, deleted.DeleteIptvFeed)
	assert.Nil(t, f.feeds(t).IptvFeed)
	assert.Equal(t, []string{codeNotFound}, f.query(t, deleteIptvFeedMutation, nil, nil), "nothing to revoke")
}

// TestCreateIptvFeedRequiresRecentAuthentication は、発行に直近の認証が要ることを確かめる (#1324)。
func TestCreateIptvFeedRequiresRecentAuthentication(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	require.Empty(t, f.as(t, "iptv-admin", domain.RoleAdmin).query(t, enableIptvFeeds, nil, nil))
	f.clock.Advance(auth.RecentAuthenticationWindow + time.Second)
	assert.Equal(t, []string{codeReauthRequired}, f.query(t, createIptvFeedMutation, nil, nil))
	assert.Nil(t, f.feeds(t).IptvFeed)
}

// TestIptvFeedWhileDisabled は、管理者が IPTV のクライアント用の URL を有効にしていなければ発行せず、
// 有効にしていた間に発行した URL は、無効の間も取り消せることを確かめる (#1755)。既定では無効にする。
func TestIptvFeedWhileDisabled(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	admin := f.as(t, "iptv-admin", domain.RoleAdmin)
	assert.Equal(t, []string{codeForbidden}, f.query(t, createIptvFeedMutation, nil, nil), "disabled by default")
	assert.Nil(t, f.feeds(t).IptvFeed)

	require.Empty(t, admin.query(t, enableIptvFeeds, nil, nil))
	require.Empty(t, f.query(t, createIptvFeedMutation, nil, nil))
	require.Empty(t, admin.query(t, disableIptvFeeds, nil, nil))
	assert.Equal(t, []string{codeForbidden}, f.query(t, createIptvFeedMutation, nil, nil))

	var deleted struct {
		DeleteIptvFeed bool `json:"deleteIptvFeed"`
	}
	require.Empty(t, f.query(t, deleteIptvFeedMutation, nil, &deleted))
	assert.True(t, deleted.DeleteIptvFeed)
	assert.Nil(t, f.feeds(t).IptvFeed)
}
