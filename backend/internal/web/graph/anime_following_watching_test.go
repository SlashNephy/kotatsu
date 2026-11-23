package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
)

const (
	animeFollowingWatchingQuery = `{ animeFollowingWatching { enabled
		works { tid title covered watchers { destination username name avatarUrl profileUrl } broadcasts { program { name } } } } }`
	createAnimeFollowingWatchingRule = `mutation($tid: Int!) { createAnimeFollowingWatchingRule(tid: $tid) { name } }`
)

type animeFollowingWatchingResult struct {
	AnimeFollowingWatching struct {
		Enabled bool `json:"enabled"`
		Works   []struct {
			Tid      int    `json:"tid"`
			Title    string `json:"title"`
			Covered  bool   `json:"covered"`
			Watchers []struct {
				Destination string  `json:"destination"`
				Username    string  `json:"username"`
				Name        string  `json:"name"`
				AvatarURL   *string `json:"avatarUrl"`
				ProfileURL  string  `json:"profileUrl"`
			} `json:"watchers"`
			Broadcasts []struct {
				Program struct {
					Name string `json:"name"`
				} `json:"program"`
			} `json:"broadcasts"`
		} `json:"works"`
	} `json:"animeFollowingWatching"`
}

// TestAnimeFollowingWatching は、フォロー中のユーザーが見ている作品のこれからの放送を、取り込んだ本人にだけ返し、
// 予約できるユーザーだけが、その作品からルールを作れることを確認する (#1968、設計書 9.7)。
func TestAnimeFollowingWatching(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, domain.RoleAdmin)
	viewer := f.as(t, "viewer", domain.RoleViewer)
	other := f.as(t, "other", domain.RoleMember)
	linkWishlistProgram(t, f, 3273601024, time.Date(2026, 10, 10, 13, 0, 0, 0, time.UTC))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 6776, 0, 10, NULL, NULL)`)
	require.NoError(t, err)
	for _, userID := range []any{f.userID, viewer.userID} {
		_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_record_connections (user_id, destination, access_token_encrypted, account_name, connected_at)
			VALUES ($1, 'annict', '\x00', 'annict-user', $2)`, userID, testNow())
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), `INSERT INTO anime_following_watching (user_id, destination, username, name, avatar_url, work_id, title, fetched_at)
			VALUES ($1, 'annict', 'hana', 'Hana', 'https://example.com/hana.png', 10, '作品A', $2)`, userID, testNow())
		require.NoError(t, err)
	}
	tid := map[string]any{"tid": wishlistTID}

	var got animeFollowingWatchingResult
	require.Empty(t, viewer.query(t, animeFollowingWatchingQuery, nil, &got))
	assert.False(t, got.AnimeFollowingWatching.Enabled, "the linking is disabled by default")
	assert.Empty(t, got.AnimeFollowingWatching.Works)
	assert.Equal(t, []string{codeConflict}, f.query(t, createAnimeFollowingWatchingRule, tid, nil))
	require.Empty(t, admin.query(t, enableAnimeLinking, nil, nil))

	// 視聴者も、自分の分は見られる。
	require.Empty(t, viewer.query(t, animeFollowingWatchingQuery, nil, &got))
	require.True(t, got.AnimeFollowingWatching.Enabled)
	require.Len(t, got.AnimeFollowingWatching.Works, 1)
	work := got.AnimeFollowingWatching.Works[0]
	assert.Equal(t, wishlistTID, work.Tid)
	assert.Equal(t, "作品A", work.Title)
	require.Len(t, work.Watchers, 1)
	assert.Equal(t, "Hana", work.Watchers[0].Name)
	assert.Equal(t, "https://annict.com/@hana", work.Watchers[0].ProfileURL)
	require.NotNil(t, work.Watchers[0].AvatarURL, "the avatar is relayed by tenban")
	require.Len(t, work.Broadcasts, 1)
	assert.Equal(t, "作品A #5「話」", work.Broadcasts[0].Program.Name)

	// ほかのユーザーには見えず、視聴者はルールを作れない。
	var others animeFollowingWatchingResult
	require.Empty(t, other.query(t, animeFollowingWatchingQuery, nil, &others))
	assert.Empty(t, others.AnimeFollowingWatching.Works)
	assert.Equal(t, []string{codeNotFound}, other.query(t, createAnimeFollowingWatchingRule, tid, nil))
	assert.Equal(t, []string{codeForbidden}, viewer.query(t, createAnimeFollowingWatchingRule, tid, nil))

	// 予約できるユーザーは、ルールを作れ、作った後は予約済みになる。
	var created struct {
		CreateAnimeFollowingWatchingRule struct {
			Name string `json:"name"`
		} `json:"createAnimeFollowingWatchingRule"`
	}
	require.Empty(t, f.query(t, createAnimeFollowingWatchingRule, tid, &created))
	assert.Equal(t, "作品A", created.CreateAnimeFollowingWatchingRule.Name)
	assert.Equal(t, []string{codeConflict}, f.query(t, createAnimeFollowingWatchingRule, tid, nil))
}
