package anime_test

import (
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

// testTitle は、番組名の解析のテストに使う作品名。
const testTitle = "テスト作品"

// TestParseTitle は、番組名から作品名と話数を読み取ることを確かめる。
func TestParseTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want *anime.ParsedTitle
	}{
		{
			name: "single episode",
			in:   "テスト作品 #12",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 12, CountEnd: 12, Subtitle: ""},
		},
		{
			name: "full-width marks and subtitle",
			in:   "[無]テスト作品　＃５「はじまりの日」[字]",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 5, CountEnd: 5, Subtitle: "はじまりの日"},
		},
		{
			name: "enclosed marks",
			in:   "🈟テスト作品 #1",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 1, CountEnd: 1, Subtitle: ""},
		},
		{
			name: "multiple episodes",
			in:   "テスト作品 #3-#4",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 3, CountEnd: 4, Subtitle: ""},
		},
		{
			name: "multiple episodes with a middle dot",
			in:   "テスト作品 #3・4",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 3, CountEnd: 4, Subtitle: ""},
		},
		{
			name: "episode number in kanji style",
			in:   "テスト作品 第7話「つづき」",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 7, CountEnd: 7, Subtitle: "つづき"},
		},
		{
			name: "half episode",
			in:   "テスト作品 #12.5",
			want: &anime.ParsedTitle{Title: testTitle, CountStart: 12.5, CountEnd: 12.5, Subtitle: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := anime.ParseTitle(tt.in)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}

	for _, in := range []string{"テスト作品", "#12", "ニュース 7"} {
		_, ok := anime.ParseTitle(in)
		assert.False(t, ok, in)
	}
}

// TestTitleKey は、作品名を、全角と半角、空白、大文字と小文字によらずに引けるようにすることを確かめる。
func TestTitleKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, anime.TitleKey("リピート 作品 ABC"), anime.TitleKey("リピート作品　ａｂｃ"))
	assert.NotEqual(t, anime.TitleKey("リピート作品"), anime.TitleKey("リピート作品2"))
}

// TestImportTitles は、しょぼいカレンダーのタイトルの一覧を、2 回目からは前に取り込んだ後の更新だけ引くことを確かめる。
func TestImportTitles(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	titles := newFakeTitles()
	importer := anime.NewImporter(db, clk, newFakeFetcher(), titles, nil, slog.New(slog.DiscardHandler))
	_, err := importer.Import(t.Context(), false)
	require.NoError(t, err)
	_, err = importer.Import(t.Context(), true)
	require.NoError(t, err)

	require.Len(t, titles.since, 2)
	assert.True(t, titles.since[0].IsZero(), "the first import reads every title")
	assert.True(t, titles.since[1].Equal(time.Date(2026, 9, 30, 1, 0, 0, 0, time.UTC)), "%v", titles.since[1])
}

// TestMatchTitleFallback は、しょぼいカレンダーで引けない番組を、番組名を解析して話に結びつけることを確かめる。
// ChID のないサービスの番組 (アニメ専門チャンネルのリピート放送など) も、しょぼいカレンダーに問い合わせずに結びつける。
// 作品名が 1 つのタイトルだけに一致すれば TID を付け、複数に一致すれば付けない。
func TestMatchTitleFallback(t *testing.T) {
	t.Parallel()

	db, _, client, matcher := newMatchFixture(t)
	insertNamedProgram(t, db, csService, 1, jst(2, 3, 0), jst(2, 3, 30), "[字][再]リピート作品 #12「おわり」")
	insertNamedProgram(t, db, grService, 9, jst(2, 4, 0), jst(2, 4, 30), "同名作品 #3")
	// 囲み文字の 🈞 も、再放送の印として読む。
	insertNamedProgram(t, db, csService, 2, jst(2, 5, 0), jst(2, 5, 30), "🈞リピート作品 #13")
	_, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	for _, call := range client.takeCalls() {
		assert.NotContains(t, call.channelIDs, int32(0), "services without a channel ID are not looked up")
	}

	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(30)), CountStart: new(12.0), CountEnd: new(12.0), Subtitle: new("おわり"), First: false, Last: false, Rerun: true},
	}, episodesOfProgram(t, db, csService, jst(2, 3, 0)), "the rerun mark is read from the name")
	assert.Equal(t, []episodeRow{
		{Position: 0, TID: nil, CountStart: new(3.0), CountEnd: new(3.0), Subtitle: nil, First: false, Last: false},
	}, episodesOfProgram(t, db, grService, jst(2, 4, 0)), "an ambiguous title gets no TID")
	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(30)), CountStart: new(13.0), CountEnd: new(13.0), Subtitle: nil, First: false, Last: false, Rerun: true},
	}, episodesOfProgram(t, db, csService, jst(2, 5, 0)), "the enclosed rerun mark is read")
	var source string
	require.NoError(t, db.QueryRow(t.Context(), `SELECT source FROM program_anime_episodes WHERE service_id = $1`, csService).Scan(&source))
	assert.Equal(t, anime.SourceTitle, source)
}

// TestMatchSeriesFallback は、番組名に話数がなければ、シリーズの記述子の話数で話に結びつけることを確かめる (#1311)。
// 記述子の再放送の印も読む。番組名に話数があれば、記述子の話数と食い違っても、番組名の話数を使う。
func TestMatchSeriesFallback(t *testing.T) {
	t.Parallel()

	db, _, _, matcher := newMatchFixture(t)
	insertNamedProgram(t, db, csService, 1, jst(2, 3, 0), jst(2, 3, 30), "[字]リピート作品")
	insertNamedProgram(t, db, csService, 2, jst(2, 4, 0), jst(2, 4, 30), "リピート作品 #12")
	insertNamedProgram(t, db, csService, 3, jst(2, 5, 0), jst(2, 5, 30), "リピート作品")
	for _, series := range []struct {
		eventID int64
		episode int32
		repeat  bool
	}{{eventID: 1, episode: 7, repeat: true}, {eventID: 2, episode: 99, repeat: false}} {
		_, err := db.Exec(t.Context(), `UPDATE programs SET series_episode = $1, series_repeat = $2 WHERE service_id = $3 AND event_id = $4`,
			series.episode, series.repeat, csService, series.eventID)
		require.NoError(t, err)
	}
	_, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(30)), CountStart: new(7.0), CountEnd: new(7.0), Subtitle: nil, First: false, Last: false, Rerun: true},
	}, episodesOfProgram(t, db, csService, jst(2, 3, 0)), "the episode and the rerun mark are read from the series descriptor")
	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(30)), CountStart: new(12.0), CountEnd: new(12.0), Subtitle: nil, First: false, Last: false, Rerun: false},
	}, episodesOfProgram(t, db, csService, jst(2, 4, 0)), "the episode in the name wins")
	assert.Empty(t, episodesOfProgram(t, db, csService, jst(2, 5, 0)), "no episode without the name or the descriptor")
}

// insertNamedProgram は、番組名 name のアニメの番組を足す。
func insertNamedProgram(t *testing.T, db *pgxpool.Pool, serviceID, eventID int64, start, end time.Time, name string) {
	t.Helper()

	_, err := db.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		VALUES ($1, $2, $2, $3, $4, false, true, $5, '', $5, '', '{}', $6::jsonb, '[]', now(), now())`,
		serviceID, eventID, start, end, name, genreAnime)
	require.NoError(t, err)
}

// insertRecording は、録画を足す。imported なら、EPGStation から取り込んだ録画にする。
func insertRecording(t *testing.T, db *pgxpool.Pool, serviceID int64, start, end time.Time, imported bool) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, 'テスト', '', $3, $4, 'completed', $3, now(), now())`, id, serviceID, start, end)
	require.NoError(t, err)
	if imported {
		_, err = db.Exec(t.Context(), `INSERT INTO import_sources (source, kind, source_id, target_id, created_at)
			VALUES ('epgstation', 'recorded', (SELECT count(*) FROM import_sources) + 1, $1, now())`, id)
		require.NoError(t, err)
	}

	return id
}

// recordingEpisodes は、録画に結びついた話の、話数の始まりを並べて返す。照合していなければ nil を返す。
func recordingEpisodes(t *testing.T, db *pgxpool.Pool, recordingID uuid.UUID) []float64 {
	t.Helper()

	var matched bool
	require.NoError(t, db.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM recording_anime_matches WHERE recording_id = $1)`, recordingID).Scan(&matched))
	if !matched {
		return nil
	}
	rows, err := db.Query(t.Context(), `SELECT count_start FROM recording_anime_episodes WHERE recording_id = $1 ORDER BY position`, recordingID)
	require.NoError(t, err)
	defer rows.Close()
	counts := []float64{}
	for rows.Next() {
		var count float64
		require.NoError(t, rows.Scan(&count))
		counts = append(counts, count)
	}
	require.NoError(t, rows.Err())

	return counts
}

// TestCopyToRecording は、録画を始めたときに番組の結びつきを写し、放送が終わった後の確定で直った結びつきも写し直すことを確かめる。
func TestCopyToRecording(t *testing.T) {
	t.Parallel()

	db, clk, client, matcher := newMatchFixture(t)
	_, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	recordingID := insertRecording(t, db, grService, jst(2, 0, 30), jst(2, 1, 30), false)
	require.NoError(t, anime.CopyToRecording(t.Context(), db, recordingID, grService, jst(2, 0, 30), clk.Now()))
	assert.Equal(t, []float64{1, 2}, recordingEpisodes(t, db, recordingID))

	// 再放送の印も写す (#1233)。
	rerun := insertRecording(t, db, bsService, jst(2, 22, 0), jst(2, 23, 0), false)
	require.NoError(t, anime.CopyToRecording(t.Context(), db, rerun, bsService, jst(2, 22, 0), clk.Now()))
	var copied bool
	require.NoError(t, db.QueryRow(t.Context(), `SELECT bool_and(is_rerun) FROM recording_anime_episodes WHERE recording_id = $1`, rerun).Scan(&copied))
	assert.True(t, copied)

	// 照合していない番組の録画には、何も写さない。
	other := insertRecording(t, db, grService, jst(2, 1, 30), jst(2, 2, 0), false)
	require.NoError(t, anime.CopyToRecording(t.Context(), db, other, grService, jst(2, 1, 30), clk.Now()))
	assert.Nil(t, recordingEpisodes(t, db, other))

	clk.Set(jst(2, 1, 40))
	client.set([]*syobocal.Program{
		syobocalProgram(101, 11, grChannel, jst(2, 0, 30), jst(2, 1, 0), "1", "", "", 0),
		syobocalProgram(102, 11, grChannel, jst(2, 1, 0), jst(2, 1, 30), "3", "", "", 0),
	}, false)
	_, err = matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []float64{1, 3}, recordingEpisodes(t, db, recordingID), "the corrected link is copied again")
}

// TestMatchImported は、EPGStation から取り込んだ録画を、録画のサービスと開始時刻で照合し、照合した録画は照合し直さないことを確かめる。
// 番組を照合していれば、しょぼいカレンダーに問い合わせずに写す。
func TestMatchImported(t *testing.T) {
	t.Parallel()

	db, clk, client, matcher := newMatchFixture(t)
	_, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	client.takeCalls()

	// 番組を照合した録画と、番組表にない古い録画。
	copied := insertRecording(t, db, grService, jst(2, 0, 0), jst(2, 0, 30), true)
	old := insertRecording(t, db, bsService, time.Date(2024, 4, 1, 13, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 13, 30, 0, 0, time.UTC), true)
	notImported := insertRecording(t, db, bsService, time.Date(2024, 4, 1, 13, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 13, 30, 0, 0, time.UTC), false)
	client.set([]*syobocal.Program{
		syobocalProgram(300, 40, bsChannel, time.Date(2024, 4, 1, 13, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 13, 30, 0, 0, time.UTC), "8", "", "", 0),
	}, false)
	clk.Advance(time.Minute)
	result, err := matcher.MatchImported(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, result.Programs)
	calls := client.takeCalls()
	require.Len(t, calls, 1, "only the old recording is looked up")
	assert.Equal(t, []int32{bsChannel}, calls[0].channelIDs)
	assert.Equal(t, []float64{5}, recordingEpisodes(t, db, copied))
	assert.Equal(t, []float64{8}, recordingEpisodes(t, db, old))
	assert.Nil(t, recordingEpisodes(t, db, notImported), "recordings made by Kotatsu are not matched by this job")

	result, err = matcher.MatchImported(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs, "matched recordings are not matched again")
	assert.Empty(t, client.takeCalls())
}

// TestMatchImportedLongRecording は、それだけで問い合わせの上限 (14 の ChID と日の組) を超える、15 日以上にまたがる取り込み録画が
// 古い順の先頭にあっても、後の録画の照合を続けることを確かめる (#1417)。そのような録画は、しょぼいカレンダーに問い合わせず、
// 番組名の解析で照合する。終わりが遠い未来の録画でも、問い合わせる日を並べない。
func TestMatchImportedLongRecording(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		end  time.Time
	}{
		{name: "20 日にまたがる録画", end: time.Date(2024, 3, 21, 13, 0, 0, 0, time.UTC)},
		{name: "終わりが遠い未来の録画", end: time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, clk, client, matcher := newMatchFixture(t)
			_, err := matcher.MatchHorizon(t.Context())
			require.NoError(t, err)
			client.takeCalls()

			long := insertRecording(t, db, bsService, time.Date(2024, 3, 1, 13, 0, 0, 0, time.UTC), tt.end, true)
			later := insertRecording(t, db, bsService, time.Date(2024, 4, 1, 13, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 13, 30, 0, 0, time.UTC), true)
			client.set([]*syobocal.Program{
				syobocalProgram(300, 40, bsChannel, time.Date(2024, 4, 1, 13, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 13, 30, 0, 0, time.UTC), "8", "", "", 0),
			}, false)
			clk.Advance(time.Minute)

			result, err := matcher.MatchImported(t.Context())
			require.NoError(t, err)

			assert.Equal(t, 2, result.Programs, "both recordings are matched")
			calls := client.takeCalls()
			require.Len(t, calls, 1, "only the later recording is looked up")
			assert.Equal(t, []int32{bsChannel}, calls[0].channelIDs)
			assert.Equal(t, []float64{8}, recordingEpisodes(t, db, later))
			assert.NotNil(t, recordingEpisodes(t, db, long), "the long recording is matched by its title")
			result, err = matcher.MatchImported(t.Context())
			require.NoError(t, err)
			assert.Zero(t, result.Programs, "the long recording is not matched again")
		})
	}
}
