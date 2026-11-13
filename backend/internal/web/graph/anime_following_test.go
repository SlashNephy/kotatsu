package graph_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const animeFollowingQuery = `query ($work: Int!) {
  animeFollowingViewings(tid: $work) {
    enabled needsRefresh failed
    viewings { destination username name avatarUrl profileUrl status progress score notes }
  }
}`

type animeFollowingResult struct {
	AnimeFollowingViewings struct {
		Enabled      bool
		NeedsRefresh bool
		Failed       bool
		Viewings     []struct {
			Destination string
			Username    string
			Name        string
			AvatarURL   *string
			ProfileURL  string
			Status      string
			Progress    *int
			Score       *float64
			Notes       *string
		}
	}
}

// TestAnimeFollowingViewings は、取り込んだ視聴状況を、取り込んだ本人にだけ返すことを確かめる (#1372、設計書 3.8)。
func TestAnimeFollowingViewings(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	admin := f.as(t, adminName, auth.RoleAdmin)
	viewer := f.as(t, "viewer", auth.RoleViewer)
	other := f.as(t, "other", auth.RoleViewer)
	tid := map[string]any{"work": wishlistTID}

	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 6776, 0, 10, 20, NULL)`)
	require.NoError(t, err)
	connectWishlist(t, f, viewer.userID)
	connectWishlist(t, f, other.userID)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO anime_following_fetches (user_id, destination, syobocal_tid, checked_at, fetched_at) VALUES ($1, 'annict', 6776, $2, $2)`, viewer.userID, testNow())
	require.NoError(t, err)
	_, err = f.pool.Exec(t.Context(), `INSERT INTO anime_following_viewings (user_id, destination, syobocal_tid, username, name, avatar_url, status)
		VALUES ($1, 'annict', 6776, 'a b', '名前', 'https://example.com/a.png', 'on_hold')`, viewer.userID)
	require.NoError(t, err)

	var got animeFollowingResult
	require.Empty(t, viewer.query(t, animeFollowingQuery, tid, &got))
	assert.False(t, got.AnimeFollowingViewings.Enabled, "the linking is disabled by default")
	require.Empty(t, admin.query(t, enableAnimeLinking, nil, nil))

	got = animeFollowingResult{}
	require.Empty(t, viewer.query(t, animeFollowingQuery, tid, &got))
	require.True(t, got.AnimeFollowingViewings.Enabled)
	assert.False(t, got.AnimeFollowingViewings.NeedsRefresh)
	require.Len(t, got.AnimeFollowingViewings.Viewings, 1)
	viewing := got.AnimeFollowingViewings.Viewings[0]
	assert.Equal(t, "ANNICT", viewing.Destination)
	assert.Equal(t, "ON_HOLD", viewing.Status)
	assert.Equal(t, "https://annict.com/@a%20b", viewing.ProfileURL)
	require.NotNil(t, viewing.AvatarURL)
	assert.Contains(t, *viewing.AvatarURL, "/internal/media/anime-following/annict/a%20b/avatar?v=")

	got = animeFollowingResult{}
	require.Empty(t, other.query(t, animeFollowingQuery, tid, &got))
	assert.True(t, got.AnimeFollowingViewings.NeedsRefresh)
	assert.Empty(t, got.AnimeFollowingViewings.Viewings, "other users' results are not shown")

	for _, invalid := range []int{0, -1} {
		assert.Equal(t, []string{"BAD_USER_INPUT"}, viewer.query(t, animeFollowingQuery, map[string]any{"work": invalid}, nil))
	}
}
