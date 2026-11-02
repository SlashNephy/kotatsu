package graph_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

const animeLibraryQuery = `{ animeLibrary { enabled imports { destination entryCount }
	works { id tid title status progress nextEpisode { number } sources { destination workId url progress }
		nextBroadcast { countStart countEnd program { name } }
		recordedEpisodes { countStart recordingCount watched recording { id name } } } } }`

type animeLibraryResult struct {
	AnimeLibrary struct {
		Enabled bool `json:"enabled"`
		Imports []struct {
			Destination string `json:"destination"`
			EntryCount  int    `json:"entryCount"`
		} `json:"imports"`
		Works []struct {
			ID          string `json:"id"`
			Tid         *int   `json:"tid"`
			Title       string `json:"title"`
			Status      string `json:"status"`
			Progress    *int   `json:"progress"`
			NextEpisode *struct {
				Number *int `json:"number"`
			} `json:"nextEpisode"`
			Sources []struct {
				Destination string `json:"destination"`
				WorkID      int    `json:"workId"`
				URL         string `json:"url"`
				Progress    *int   `json:"progress"`
			} `json:"sources"`
			NextBroadcast *struct {
				CountStart *float64 `json:"countStart"`
				CountEnd   *float64 `json:"countEnd"`
				Program    struct {
					Name string `json:"name"`
				} `json:"program"`
			} `json:"nextBroadcast"`
			RecordedEpisodes []struct {
				CountStart     float64 `json:"countStart"`
				RecordingCount int     `json:"recordingCount"`
				Watched        *bool   `json:"watched"`
				Recording      struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"recording"`
			} `json:"recordedEpisodes"`
		} `json:"works"`
	} `json:"animeLibrary"`
}

// TestAnimeLibrary は、視聴管理の画面の作品を、ユーザーごとに自分の分だけ、次の放送と録画のある話と返すことを確かめる (#1231)。
func TestAnimeLibrary(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, auth.RoleAdmin)
	viewer := f.as(t, "viewer", auth.RoleViewer)
	other := f.as(t, "other", auth.RoleMember)
	linkWishlistProgram(t, f, 3273601024, time.Date(2026, 10, 10, 13, 0, 0, 0, time.UTC))
	_, err := f.pool.Exec(t.Context(), `INSERT INTO anime_works (source, syobocal_tid, position, annict_work_id, anilist_id, mal_id) VALUES ('arm', 6776, 0, 10, NULL, NULL)`)
	require.NoError(t, err)
	connectWishlist(t, f, viewer.userID)
	_, err = f.pool.Exec(t.Context(), `UPDATE anime_wishlist_entries SET status = 'watching', progress = 3 WHERE user_id = $1`, viewer.userID)
	require.NoError(t, err)
	// 第 3 話と第 4 話の録画。第 4 話は 2 局で録った。
	for i, episode := range []float64{3, 4, 4} {
		id := uuid.MustParse(fmt.Sprintf("0192f000-0000-7000-8000-%012d", i+1))
		start := testNow().Add(-time.Duration(7-i) * 24 * time.Hour)
		_, err := f.pool.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
			VALUES ($1, 3273601024, '作品A', '', $2, $3, 'completed', $2, now(), now())`, id, start, start.Add(30*time.Minute))
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_anime_matches (recording_id, matched_at) VALUES ($1, now())`, id)
		require.NoError(t, err)
		_, err = f.pool.Exec(t.Context(), `INSERT INTO recording_anime_episodes (recording_id, position, source, syobocal_tid, count_start, count_end, is_first, is_last)
			VALUES ($1, 0, 'syobocal', 6776, $2, $2, false, false)`, id, episode)
		require.NoError(t, err)
	}

	var got animeLibraryResult
	require.Empty(t, viewer.query(t, animeLibraryQuery, nil, &got))
	assert.False(t, got.AnimeLibrary.Enabled, "the linking is disabled by default")
	assert.Empty(t, got.AnimeLibrary.Works)
	require.Empty(t, admin.query(t, enableAnimeLinking, nil, nil))

	// 視聴者も、自分の作品は見られる。
	require.Empty(t, viewer.query(t, animeLibraryQuery, nil, &got))
	require.True(t, got.AnimeLibrary.Enabled)
	require.Len(t, got.AnimeLibrary.Imports, 1)
	assert.Equal(t, 1, got.AnimeLibrary.Imports[0].EntryCount)
	require.Len(t, got.AnimeLibrary.Works, 1)
	work := got.AnimeLibrary.Works[0]
	require.NotNil(t, work.Tid)
	assert.Equal(t, wishlistTID, *work.Tid)
	assert.NotEmpty(t, work.ID)
	assert.Equal(t, "作品A", work.Title)
	assert.Equal(t, "WATCHING", work.Status)
	require.NotNil(t, work.Progress)
	assert.Equal(t, 3, *work.Progress)
	require.NotNil(t, work.NextEpisode)
	require.NotNil(t, work.NextEpisode.Number)
	assert.Equal(t, 4, *work.NextEpisode.Number, "the next episode follows the progress")
	require.Len(t, work.Sources, 1)
	assert.Equal(t, "https://annict.com/works/10", work.Sources[0].URL)
	require.NotNil(t, work.NextBroadcast)
	assert.Equal(t, "作品A #5「話」", work.NextBroadcast.Program.Name)
	require.Len(t, work.RecordedEpisodes, 2)
	assert.InDelta(t, 3, work.RecordedEpisodes[0].CountStart, 0)
	assert.Equal(t, new(true), work.RecordedEpisodes[0].Watched)
	assert.InDelta(t, 4, work.RecordedEpisodes[1].CountStart, 0)
	assert.Equal(t, new(false), work.RecordedEpisodes[1].Watched)
	assert.Equal(t, 2, work.RecordedEpisodes[1].RecordingCount, "two recordings of the same episode are grouped")
	assert.Equal(t, "作品A", work.RecordedEpisodes[1].Recording.Name)

	// ほかのユーザーの見たいリストは見えない。
	var others animeLibraryResult
	require.Empty(t, other.query(t, animeLibraryQuery, nil, &others))
	assert.True(t, others.AnimeLibrary.Enabled)
	assert.Empty(t, others.AnimeLibrary.Imports)
	assert.Empty(t, others.AnimeLibrary.Works)
}
