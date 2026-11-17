package dramalibrary_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
	"github.com/SlashNephy/kotatsu/internal/lib/id"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/testutil/faketuner"
	"github.com/SlashNephy/kotatsu/internal/usecase/animerecord"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/dramalibrary"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	grNetworkID = 0x7FE0
	grServiceID = 0x0400
	// series は、テストの TMDb の作品の ID。unlinked は、どの番組にも録画にも結びつかない作品の ID。
	series   int32 = 9001
	unlinked int32 = 9002
	// stateCompleted は、録り終えた録画の状態。
	stateCompleted = "completed"
	// watchedByPlayback は、プレイヤーで見た割合から見た話にした、シーズン 1 第 4 話。
	watchedByPlayback = "1:4:playback"
)

// testNow は、テストの今。faketuner は、30 分ごとの番組を、今の 1 時間前から 6 時間後まで出す。
func testNow() time.Time {
	return time.Date(2026, 10, 10, 12, 10, 0, 0, time.UTC)
}

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 10, hour, minute, 0, 0, time.UTC)
}

func serviceID() int64 {
	return faketuner.ServiceItemID(grNetworkID, grServiceID)
}

type fixture struct {
	pool  *pgxpool.Pool
	alice uuid.UUID
	bob   uuid.UUID
}

// newFixture は、番組表を同期した DB を用意する。enabled なら、TMDb との連携を有効にする。
func newFixture(t *testing.T, enabled bool) *fixture {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	clk := clocktest.NewFake(testNow())
	tuner := httptest.NewServer(faketuner.New(faketuner.DefaultOptions(testNow(), nil)))
	t.Cleanup(tuner.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: tuner.URL})
	require.NoError(t, err)
	_, err = epg.NewReconciler(pool, client, clk, func(context.Context, pgx.Tx) error { return nil }).SyncAll(t.Context())
	require.NoError(t, err)

	users := auth.NewUsers(clk)
	f := &fixture{pool: pool}
	for _, user := range []struct {
		name string
		id   *uuid.UUID
	}{{"alice", &f.alice}, {"bob", &f.bob}} {
		created, err := users.Create(t.Context(), pool, &auth.NewUser{Username: user.name, DisplayName: user.name, Role: domain.RoleViewer, PasswordHash: nil})
		require.NoError(t, err)
		*user.id = created.ID
	}
	_, err = settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), pool, audit.User(f.bob), &settings.Changes{DramaLinkingEnabled: new(enabled)})
	require.NoError(t, err)

	return f
}

func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()

	_, err := f.pool.Exec(t.Context(), sql, args...)
	require.NoError(t, err)
}

// link は、hour 時 minute 分からの番組を、作品 series のシーズン 1 の話 episode に結びつける。episode が 0 なら、作品だけに結びつける。
func (f *fixture) link(t *testing.T, hour, minute int, episode int32) {
	t.Helper()

	var seasonNumber, episodeNumber, episodeSource *any
	if episode > 0 {
		seasonNumber, episodeNumber, episodeSource = new(any(int32(1))), new(any(episode)), new(any("air_date"))
	}
	f.exec(t, `INSERT INTO program_drama_matches (service_id, start_at, event_id, title_key, tmdb_series_id, season_number, episode_number,
		series_source, episode_source, matched_at)
		SELECT service_id, start_at, event_id, 'ドラマ', $3, $4, $5, 'search', $6, now() FROM programs WHERE service_id = $1 AND start_at = $2`,
		serviceID(), at(hour, minute), series, deref(seasonNumber), deref(episodeNumber), deref(episodeSource))
}

func deref(value *any) any {
	if value == nil {
		return nil
	}

	return *value
}

// addRecording は、作品 series のシーズン season の話 episode の録画を、recordedAt に録ったものとして足し、ID を返す。
func (f *fixture) addRecording(t *testing.T, season, episode int32, state string, recordedAt time.Time) uuid.UUID {
	t.Helper()

	recordingID := id.New()
	f.exec(t, `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, 'ドラマ', '', $3, $4, $5, $3, $3, $3)`,
		recordingID, serviceID(), recordedAt, recordedAt.Add(30*time.Minute), state)
	f.exec(t, `INSERT INTO recording_drama_matches (recording_id, tmdb_series_id, season_number, episode_number, series_source, episode_source,
		matched_at) VALUES ($1, $2, $3, $4, 'search', 'air_date', now())`, recordingID, series, season, episode)

	return recordingID
}

// cacheSeries は、作品 series の TMDb のキャッシュに、シーズンごとの話の番号 episodes を入れる。
func (f *fixture) cacheSeries(t *testing.T, episodes map[int32][]int32) {
	t.Helper()

	seasons := make([]tmdb.Season, 0, len(episodes))
	for season, numbers := range episodes {
		// 話の番号は 1 から続けて並べるので、最後の番号が話数になる。
		seasons = append(seasons, tmdb.Season{SeasonNumber: season, Name: "", AirDate: "", EpisodeCount: numbers[len(numbers)-1]})
		list := make([]tmdb.Episode, 0, len(numbers))
		for _, n := range numbers {
			list = append(list, tmdb.Episode{EpisodeNumber: n, AirDate: "", Name: fmt.Sprintf("話%d-%d", season, n)})
		}
		encoded, err := json.Marshal(list)
		require.NoError(t, err)
		f.exec(t, `INSERT INTO tmdb_seasons (series_id, season_number, episodes, fetched_at) VALUES ($1, $2, $3, now())`, series, season, encoded)
	}
	encoded, err := json.Marshal(seasons)
	require.NoError(t, err)
	f.exec(t, `INSERT INTO tmdb_series (id, name, original_name, first_air_date, seasons, fetched_at) VALUES ($1, 'ドラマA', 'Drama A', NULL, $2, now())`,
		series, encoded)
}

// watchedEpisodes は、ユーザー userID の見た話を、"シーズン:話:出どころ" の形で返す。
func (f *fixture) watchedEpisodes(t *testing.T, userID uuid.UUID) []string {
	t.Helper()

	rows, err := f.pool.Query(t.Context(), `SELECT season_number, episode_number, source FROM drama_watched_episodes WHERE user_id = $1
		ORDER BY tmdb_series_id, season_number, episode_number`, userID)
	require.NoError(t, err)
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
		var season, episode int32
		var source string
		err := row.Scan(&season, &episode, &source)

		return fmt.Sprintf("%d:%d:%s", season, episode, source), err
	})
	require.NoError(t, err)

	return got
}

// TestSetStatus は、見ている作品に、番組か録画に結びついた作品だけを足し、状態を変え、外せることを確かめる (#1652)。
func TestSetStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		series  int32
		// statuses は、順に設定する状態。空なら外す。
		statuses []string
		want     string
		wantErr  error
	}{
		{name: "adds a linked series", enabled: true, series: series, statuses: []string{dramalibrary.StatusWatching}, want: dramalibrary.StatusWatching, wantErr: nil},
		{
			name: "changes the status", enabled: true, series: series,
			statuses: []string{dramalibrary.StatusWatching, dramalibrary.StatusWannaWatch}, want: dramalibrary.StatusWannaWatch, wantErr: nil,
		},
		{name: "removes the series", enabled: true, series: series, statuses: []string{dramalibrary.StatusWatching, ""}, want: "", wantErr: nil},
		{name: "rejects a series without links", enabled: true, series: unlinked, statuses: []string{dramalibrary.StatusWatching}, want: "", wantErr: dramalibrary.ErrNotLinked},
		{name: "rejects an unknown status", enabled: true, series: series, statuses: []string{"dropped"}, want: "", wantErr: dramalibrary.ErrInvalidStatus},
		{name: "rejects while drama linking is disabled", enabled: false, series: series, statuses: []string{dramalibrary.StatusWatching}, want: "", wantErr: dramalibrary.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tt.enabled)
			f.link(t, 12, 30, 3)
			var err error
			for _, status := range tt.statuses {
				err = dramalibrary.SetStatus(t.Context(), f.pool, f.alice, tt.series, status, testNow())
			}
			require.ErrorIs(t, err, tt.wantErr)
			got, err := dramalibrary.Status(t.Context(), f.pool, f.alice, tt.series)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			// ほかのユーザーの見ている作品は変わらない。
			other, err := dramalibrary.Status(t.Context(), f.pool, f.bob, tt.series)
			require.NoError(t, err)
			assert.Empty(t, other)
		})
	}
}

// TestSetWatched は、見た話を手で付けて外せること、付けた話を後からプレイヤーで見ても付け直さないことを確かめる (#1652)。
func TestSetWatched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		episode *dramalibrary.Episode
		watched []bool
		want    []string
		wantErr error
	}{
		{name: "marks an episode watched", episode: &dramalibrary.Episode{SeriesID: series, Season: 1, Number: 3}, watched: []bool{true}, want: []string{"1:3:manual"}, wantErr: nil},
		{name: "marking twice keeps one row", episode: &dramalibrary.Episode{SeriesID: series, Season: 1, Number: 3}, watched: []bool{true, true}, want: []string{"1:3:manual"}, wantErr: nil},
		{name: "unmarks the episode", episode: &dramalibrary.Episode{SeriesID: series, Season: 1, Number: 3}, watched: []bool{true, false}, want: []string{}, wantErr: nil},
		{name: "marks a special", episode: &dramalibrary.Episode{SeriesID: series, Season: 0, Number: 1}, watched: []bool{true}, want: []string{"0:1:manual"}, wantErr: nil},
		{name: "rejects episode 0", episode: &dramalibrary.Episode{SeriesID: series, Season: 1, Number: 0}, watched: []bool{true}, want: []string{}, wantErr: dramalibrary.ErrInvalidEpisode},
		{
			name: "rejects a series without links", episode: &dramalibrary.Episode{SeriesID: unlinked, Season: 1, Number: 1},
			watched: []bool{true}, want: []string{}, wantErr: dramalibrary.ErrNotLinked,
		},
		{
			name: "unmarks a series without links", episode: &dramalibrary.Episode{SeriesID: unlinked, Season: 1, Number: 1},
			watched: []bool{false}, want: []string{}, wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, true)
			f.link(t, 12, 30, 3)
			var err error
			for _, watched := range tt.watched {
				err = dramalibrary.SetWatched(t.Context(), f.pool, f.alice, tt.episode, watched, testNow())
			}
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, f.watchedEpisodes(t, f.alice))
			assert.Empty(t, f.watchedEpisodes(t, f.bob))
		})
	}
}

// TestReport は、プレイヤーで見た区間が、番組の長さのうち決めた割合 (既定は 20%) に届いたら、番組に結びついた話を見た話にすることを
// 確かめる (#1652)。アニメの連携がなくても、TMDb との連携が有効なら数える。
func TestReport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		// episode が 0 なら、番組を作品だけに結びつける。
		episode   int32
		from, to  time.Time
		recording bool
		// savedPercent が 0 でなければ、ユーザーが見たアニメを記録する割合を変えておく。animeEnabled なら、アニメの連携を有効にする。
		savedPercent int
		animeEnabled bool
		want         []string
		wantOK       bool
	}{
		{name: "live viewing over the threshold", enabled: true, episode: 4, from: at(12, 0), to: at(12, 7), want: []string{watchedByPlayback}, wantOK: true},
		{name: "live viewing under the threshold", enabled: true, episode: 4, from: at(12, 0), to: at(12, 5), want: []string{}, wantOK: true},
		{name: "recording playback over the threshold", enabled: true, episode: 4, from: at(12, 0), to: at(12, 10), recording: true, want: []string{watchedByPlayback}, wantOK: true},
		{name: "program linked to the series only", enabled: true, episode: 0, from: at(12, 0), to: at(12, 10), want: []string{}, wantOK: true},
		{name: "drama linking disabled", enabled: false, episode: 4, from: at(12, 0), to: at(12, 10), want: []string{}, wantOK: false},
		{
			name: "a saved percent applies while anime linking is enabled", enabled: true, episode: 4, from: at(12, 0), to: at(12, 7),
			savedPercent: 80, animeEnabled: true, want: []string{}, wantOK: true,
		},
		{
			name: "a saved percent is ignored while anime linking is disabled", enabled: true, episode: 4, from: at(12, 0), to: at(12, 7),
			savedPercent: 80, animeEnabled: false, want: []string{watchedByPlayback}, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tt.enabled)
			f.link(t, 12, 0, tt.episode)
			if tt.savedPercent > 0 {
				f.exec(t, `INSERT INTO anime_record_settings (user_id, threshold_percent, updated_at) VALUES ($1, $2, now())`, f.alice, tt.savedPercent)
			}
			if tt.animeEnabled {
				f.exec(t, `UPDATE system_settings SET anime_linking_enabled = true`)
			}
			target := &animerecord.Target{ServiceID: new(serviceID()), RecordingID: nil}
			if tt.recording {
				recordingID := id.New()
				f.exec(t, `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
					VALUES ($1, $2, 'ドラマ', '', $3, $4, $5, $3, $3, $3)`, recordingID, serviceID(), at(12, 0), at(12, 30), stateCompleted)
				f.exec(t, `INSERT INTO recording_drama_matches (recording_id, tmdb_series_id, season_number, episode_number, series_source,
					episode_source, matched_at) VALUES ($1, $2, 1, $3, 'search', 'air_date', now())`, recordingID, series, tt.episode)
				target = &animerecord.Target{ServiceID: nil, RecordingID: &recordingID}
			}
			// 区間を 2 回に分けて送っても、重ねて送っても、合わせて数える。
			middle := tt.from.Add(tt.to.Sub(tt.from) / 2)
			for _, intervals := range [][]*animerecord.Interval{
				{{From: tt.from, To: middle}},
				{{From: tt.from, To: middle}, {From: middle, To: tt.to}},
			} {
				ok, err := dramalibrary.Report(t.Context(), f.pool, f.alice, target, intervals, testNow())
				require.NoError(t, err)
				assert.Equal(t, tt.wantOK, ok)
			}
			assert.Equal(t, tt.want, f.watchedEpisodes(t, f.alice))
		})
	}
}

// TestLibrary は、見ている作品ごとに、最後に見た話、次に見る話、次の放送、録画のある話を求めることを確かめる (#1652)。
func TestLibrary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// watched は、見た話 (シーズン, 話)。
		watched [][2]int32
		want    string
	}{
		{
			name:    "the next broadcast skips an episode already watched",
			watched: [][2]int32{{1, 1}, {1, 2}, {1, 3}},
			want:    "ドラマA|watching|last=1:3|next=1:4:話1-4|broadcast=13:00|0:1:unwatched:1,1:2:watched:1,1:3:watched:1,1:5:unwatched:2,2:1:unwatched:1",
		},
		{
			name:    "a special does not advance the progress",
			watched: [][2]int32{{1, 4}, {0, 1}},
			want:    "ドラマA|watching|last=1:4|next=1:5:話1-5|broadcast=12:30|0:1:watched:1,1:2:watched:1,1:3:watched:1,1:5:unwatched:2,2:1:unwatched:1",
		},
		{
			name:    "the last episode of a season leads to the next season",
			watched: [][2]int32{{1, 5}},
			want:    "ドラマA|watching|last=1:5|next=2:1:話2-1|broadcast=12:30|0:1:unwatched:1,1:2:watched:1,1:3:watched:1,1:5:watched:2,2:1:unwatched:1",
		},
		{
			name:    "a watched special is hidden even without regular episodes",
			watched: [][2]int32{{0, 1}},
			want:    "ドラマA|watching|last=-|next=-|broadcast=12:30|0:1:watched:1,1:2:unwatched:1,1:3:unwatched:1,1:5:unwatched:2,2:1:unwatched:1",
		},
		{
			name:    "without watched episodes, nothing is claimed as unwatched",
			watched: nil,
			want:    "ドラマA|watching|last=-|next=-|broadcast=12:30|0:1:unknown:1,1:2:unknown:1,1:3:unknown:1,1:5:unknown:2,2:1:unknown:1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, true)
			f.cacheSeries(t, map[int32][]int32{1: {1, 2, 3, 4, 5}, 2: {1, 2}})
			// 12:30 に第 3 話、13:00 に第 4 話、14:00 に作品だけに結びついた番組。11:30 の第 2 話はもう終わった。
			f.link(t, 11, 30, 2)
			f.link(t, 12, 30, 3)
			f.link(t, 13, 0, 4)
			f.link(t, 14, 0, 0)
			require.NoError(t, dramalibrary.SetStatus(t.Context(), f.pool, f.alice, series, dramalibrary.StatusWatching, testNow()))
			for _, episode := range tt.watched {
				require.NoError(t, dramalibrary.SetWatched(t.Context(), f.pool, f.alice,
					&dramalibrary.Episode{SeriesID: series, Season: episode[0], Number: episode[1]}, true, testNow()))
			}
			// 特別編の第 1 話を録り、第 5 話は 2 回録り、録れなかった第 6 話の録画は数えない。
			for i, recording := range []struct {
				season, episode int32
				state           string
			}{{0, 1, stateCompleted}, {1, 2, stateCompleted}, {1, 3, "partial"}, {1, 5, stateCompleted}, {1, 5, stateCompleted}, {2, 1, "recording"}, {1, 6, "failed"}} {
				f.addRecording(t, recording.season, recording.episode, recording.state, at(1, 0).Add(time.Duration(i)*time.Hour))
			}

			works, err := dramalibrary.Library(t.Context(), f.pool, f.alice, testNow())
			require.NoError(t, err)
			require.Len(t, works, 1)
			assert.Equal(t, tt.want, describe(works[0]))
			// ほかのユーザーの見ている作品は見えない。
			others, err := dramalibrary.Library(t.Context(), f.pool, f.bob, testNow())
			require.NoError(t, err)
			assert.Empty(t, others)
		})
	}
}

// describe は、作品を比べやすい文字列にする。
func describe(work *dramalibrary.Work) string {
	episode := func(e *dramalibrary.LibraryEpisode, withName bool) string {
		switch {
		case e == nil:
			return "-"
		case withName && e.Name != nil:
			return fmt.Sprintf("%d:%d:%s", e.Season, e.Number, *e.Name)
		default:
			return fmt.Sprintf("%d:%d", e.Season, e.Number)
		}
	}
	broadcast := "-"
	if work.NextBroadcast != nil {
		broadcast = work.NextBroadcast.StartAt.UTC().Format("15:04")
	}
	recorded := make([]string, 0, len(work.RecordedEpisodes))
	for _, e := range work.RecordedEpisodes {
		state := "unknown"
		if e.Watched != nil {
			state = map[bool]string{true: "watched", false: "unwatched"}[*e.Watched]
		}
		recorded = append(recorded, fmt.Sprintf("%d:%d:%s:%d", e.Season, e.Number, state, len(e.RecordingIDs)))
	}
	name := "-"
	if work.Name != nil {
		name = *work.Name
	}

	return strings.Join([]string{
		name, work.Status, "last=" + episode(work.LastWatched, false), "next=" + episode(work.NextEpisode, true), "broadcast=" + broadcast,
		strings.Join(recorded, ","),
	}, "|")
}

// TestNextEpisode は、最後に見た話の次に見る話を、TMDb のキャッシュのシーズンと話から求めることを確かめる (#1652)。
func TestNextEpisode(t *testing.T) {
	t.Parallel()

	twoSeasons := []*tmdb.Season{
		{SeasonNumber: 0, Name: "", AirDate: "", EpisodeCount: 2},
		{SeasonNumber: 1, Name: "", AirDate: "", EpisodeCount: 10},
		{SeasonNumber: 2, Name: "", AirDate: "", EpisodeCount: 8},
	}
	tests := []struct {
		name     string
		seasons  []*tmdb.Season
		episodes map[int32][]int32
		season   int32
		number   int32
		want     string
	}{
		{name: "the next episode in the season", seasons: twoSeasons, episodes: map[int32][]int32{1: {1, 2, 3}}, season: 1, number: 2, want: "1:3"},
		{name: "skips a missing episode number", seasons: twoSeasons, episodes: map[int32][]int32{1: {1, 2, 4}}, season: 1, number: 2, want: "1:4"},
		{name: "uses the episode count without the season cache", seasons: twoSeasons, episodes: nil, season: 1, number: 9, want: "1:10"},
		{name: "moves to the next season after the last episode", seasons: twoSeasons, episodes: map[int32][]int32{2: {1, 2}}, season: 1, number: 10, want: "2:1"},
		{name: "the first episode of the next season defaults to 1", seasons: twoSeasons, episodes: nil, season: 1, number: 10, want: "2:1"},
		{name: "the last episode of the last season ends the series", seasons: twoSeasons, episodes: nil, season: 2, number: 8, want: "-"},
		{name: "without any cache, the next number", seasons: nil, episodes: nil, season: 3, number: 4, want: "3:5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, dramalibrary.NextEpisode(tt.seasons, tt.episodes, tt.season, tt.number))
		})
	}
}
