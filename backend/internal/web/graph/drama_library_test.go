package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/web/graph"
)

const (
	dramaLibraryQuery = `{ dramaLibrary { enabled works { id status series { tmdbId name url }
		lastWatched { seasonNumber episodeNumber url } nextEpisode { seasonNumber episodeNumber }
		nextBroadcast { seasonNumber episodeNumber program { name } } } } }`
	setDramaWatchStatus   = `mutation($series: Int!, $status: DramaWatchStatus) { setDramaWatchStatus(seriesId: $series, status: $status) { tmdbId name watchStatus } }`
	setDramaWatched       = `mutation($series: Int!, $season: Int!, $episode: Int!, $watched: Boolean!) { setDramaEpisodeWatched(seriesId: $series, seasonNumber: $season, episodeNumber: $episode, watched: $watched) }`
	enableDramaLinking    = `mutation { updateSystemSettings(input: {dramaLinkingEnabled: true}) { dramaLinkingEnabled } }`
	reportPlayback        = `mutation($service: ID, $intervals: [PlaybackIntervalInput!]!) { reportAnimePlayback(serviceId: $service, intervals: $intervals) }`
	dramaTestSeries       = 9001
	dramaTestServiceID    = "3273601024"
	varDramaSeries        = "series"
	varDramaWatchedStatus = "status"
	varIntervals          = "intervals"
	varDramaEpisode       = "episode"
	varDramaSeason        = "season"
	varDramaWatched       = "watched"
	dramaWatching         = "WATCHING"
)

type dramaLibraryResult struct {
	DramaLibrary struct {
		Enabled bool `json:"enabled"`
		Works   []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Series struct {
				TmdbID int     `json:"tmdbId"`
				Name   *string `json:"name"`
				URL    string  `json:"url"`
			} `json:"series"`
			LastWatched *struct {
				SeasonNumber  int    `json:"seasonNumber"`
				EpisodeNumber int    `json:"episodeNumber"`
				URL           string `json:"url"`
			} `json:"lastWatched"`
			NextEpisode *struct {
				SeasonNumber  int `json:"seasonNumber"`
				EpisodeNumber int `json:"episodeNumber"`
			} `json:"nextEpisode"`
			NextBroadcast *struct {
				SeasonNumber  *int `json:"seasonNumber"`
				EpisodeNumber *int `json:"episodeNumber"`
				Program       struct {
					Name string `json:"name"`
				} `json:"program"`
			} `json:"nextBroadcast"`
		} `json:"works"`
	} `json:"dramaLibrary"`
}

// TestDramaLibrary は、ドラマの視聴管理の画面の作品を、ユーザーごとに自分の分だけ返し、見ている作品と見た話を変えられることを
// 確認する (#1652)。見た話は、アニメの連携がなくても、プレイヤーで見た区間から付く。
func TestDramaLibrary(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.syncEPG(t)
	admin := f.as(t, adminName, domain.RoleAdmin)
	viewer := f.as(t, "viewer", domain.RoleViewer)
	other := f.as(t, "other", domain.RoleMember)
	// 12:00 の番組を第 1 話に、13:00 の番組を第 2 話に結びつける。
	for _, link := range []struct {
		hour    int
		episode int
	}{{12, 1}, {13, 2}} {
		tag, err := f.pool.Exec(t.Context(), `INSERT INTO program_drama_matches (service_id, start_at, event_id, title_key, tmdb_series_id, season_number,
			episode_number, series_source, episode_source, matched_at)
			SELECT service_id, start_at, event_id, 'ドラマ', $3, 1, $4, 'search', 'air_date', now() FROM programs
			WHERE service_id = $1 AND start_at = $2`,
			3273601024, time.Date(2026, 10, 10, link.hour, 0, 0, 0, time.UTC), dramaTestSeries, link.episode)
		require.NoError(t, err)
		require.Equal(t, int64(1), tag.RowsAffected())
	}
	_, err := f.pool.Exec(t.Context(), `INSERT INTO tmdb_series (id, name, original_name, first_air_date, seasons, fetched_at)
		VALUES ($1, 'ドラマA', 'Drama A', NULL, '[{"seasonNumber": 1, "name": "", "airDate": "", "episodeCount": 10}]', now())`, dramaTestSeries)
	require.NoError(t, err)

	var got dramaLibraryResult
	require.Empty(t, viewer.query(t, dramaLibraryQuery, nil, &got))
	assert.False(t, got.DramaLibrary.Enabled, "the linking is disabled by default")
	assert.Equal(t, []string{codeConflict}, viewer.query(t, setDramaWatchStatus, map[string]any{varDramaSeries: dramaTestSeries, varDramaWatchedStatus: dramaWatching}, nil))
	require.Empty(t, admin.query(t, enableDramaLinking, nil, nil))

	// 視聴者も、自分の見ている作品を変えられる。番組にも録画にも結びつかない作品は足せない。
	var status struct {
		SetDramaWatchStatus struct {
			TmdbID      int     `json:"tmdbId"`
			Name        *string `json:"name"`
			WatchStatus *string `json:"watchStatus"`
		} `json:"setDramaWatchStatus"`
	}
	require.Empty(t, viewer.query(t, setDramaWatchStatus, map[string]any{varDramaSeries: dramaTestSeries, varDramaWatchedStatus: dramaWatching}, &status))
	assert.Equal(t, new("ドラマA"), status.SetDramaWatchStatus.Name)
	assert.Equal(t, new(dramaWatching), status.SetDramaWatchStatus.WatchStatus)
	assert.Equal(t, []string{codeNotFound}, viewer.query(t, setDramaWatchStatus, map[string]any{varDramaSeries: 9999, varDramaWatchedStatus: dramaWatching}, nil))
	assert.Equal(t, []string{codeBadUserInput}, viewer.query(t, setDramaWatched,
		map[string]any{varDramaSeries: dramaTestSeries, varDramaSeason: 1, varDramaEpisode: 0, varDramaWatched: true}, nil))

	// アニメの連携がなくても、12:00 の番組 (第 1 話) を 10 分見たら、見た話になり、次の放送は第 2 話になる。
	var reported struct {
		ReportAnimePlayback bool `json:"reportAnimePlayback"`
	}
	intervals := []map[string]any{{"from": noon, "to": "2026-10-10T12:10:00Z"}}
	require.Empty(t, viewer.query(t, reportPlayback, map[string]any{varService: graph.GlobalID("Service", dramaTestServiceID), varIntervals: intervals}, &reported))
	assert.True(t, reported.ReportAnimePlayback, "the player keeps reporting for dramas")

	require.Empty(t, viewer.query(t, dramaLibraryQuery, nil, &got))
	require.True(t, got.DramaLibrary.Enabled)
	require.Len(t, got.DramaLibrary.Works, 1)
	work := got.DramaLibrary.Works[0]
	assert.NotEmpty(t, work.ID)
	assert.Equal(t, dramaWatching, work.Status)
	assert.Equal(t, dramaTestSeries, work.Series.TmdbID)
	assert.Equal(t, "https://www.themoviedb.org/tv/9001", work.Series.URL)
	require.NotNil(t, work.LastWatched)
	assert.Equal(t, 1, work.LastWatched.EpisodeNumber)
	assert.Equal(t, "https://www.themoviedb.org/tv/9001/season/1/episode/1", work.LastWatched.URL)
	require.NotNil(t, work.NextEpisode)
	assert.Equal(t, 2, work.NextEpisode.EpisodeNumber)
	require.NotNil(t, work.NextBroadcast)
	assert.Equal(t, new(2), work.NextBroadcast.EpisodeNumber)

	// 手で第 2 話を見たことにすると、次の話は第 3 話に進む。外すと戻る。
	require.Empty(t, viewer.query(t, setDramaWatched, map[string]any{varDramaSeries: dramaTestSeries, varDramaSeason: 1, varDramaEpisode: 2, varDramaWatched: true}, nil))
	require.Empty(t, viewer.query(t, dramaLibraryQuery, nil, &got))
	require.NotNil(t, got.DramaLibrary.Works[0].NextEpisode)
	assert.Equal(t, 3, got.DramaLibrary.Works[0].NextEpisode.EpisodeNumber)
	require.Empty(t, viewer.query(t, setDramaWatched, map[string]any{varDramaSeries: dramaTestSeries, varDramaSeason: 1, varDramaEpisode: 2, varDramaWatched: false}, nil))
	require.Empty(t, viewer.query(t, dramaLibraryQuery, nil, &got))
	require.NotNil(t, got.DramaLibrary.Works[0].NextEpisode)
	assert.Equal(t, 2, got.DramaLibrary.Works[0].NextEpisode.EpisodeNumber)

	// ほかのユーザーの見ている作品は見えない。
	var others dramaLibraryResult
	require.Empty(t, other.query(t, dramaLibraryQuery, nil, &others))
	assert.True(t, others.DramaLibrary.Enabled)
	assert.Empty(t, others.DramaLibrary.Works)

	// 外すと、一覧から消える。
	require.Empty(t, viewer.query(t, setDramaWatchStatus, map[string]any{varDramaSeries: dramaTestSeries, varDramaWatchedStatus: nil}, &status))
	assert.Nil(t, status.SetDramaWatchStatus.WatchStatus)
	require.Empty(t, viewer.query(t, dramaLibraryQuery, nil, &got))
	assert.Empty(t, got.DramaLibrary.Works)
}
