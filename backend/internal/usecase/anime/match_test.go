package anime_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/anime"
)

const (
	// genreAnime は、アニメと特撮 (大分類 0x7) のジャンル。
	genreAnime = `[{"lv1": 7, "lv2": 0, "un1": 15, "un2": 15}]`
	// genreNews は、ニュース (大分類 0x0) のジャンル。
	genreNews = `[{"lv1": 0, "lv2": 0, "un1": 15, "un2": 15}]`
	// grChannel と bsChannel は、newFakeFetcher が grService と bsService に対応づける ChID。
	grChannel = 1
	bsChannel = 9
)

// jst は、2026 年 11 月 day 日 hour 時 minute 分 (日本時間) を返す。
func jst(day, hour, minute int) time.Time {
	return time.Date(2026, time.November, day, hour, minute, 0, 0, syobocal.Tokyo())
}

// lookupCall は、偽のしょぼいカレンダーへの問い合わせ 1 回。
type lookupCall struct {
	channelIDs []int32
	from, to   time.Time
}

// fakeSyobocal は、持っている放送時間データのうち、問い合わせと重なるものを返す偽物。
type fakeSyobocal struct {
	mu       sync.Mutex
	programs []*syobocal.Program
	calls    []lookupCall
	fail     bool
}

var _ anime.SyobocalClient = new(fakeSyobocal)

func (f *fakeSyobocal) ProgLookup(_ context.Context, channelIDs []int32, from, to time.Time) ([]*syobocal.Program, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, lookupCall{channelIDs: slices.Clone(channelIDs), from: from, to: to})
	if f.fail {
		return nil, errors.New("syobocal is down")
	}
	found := []*syobocal.Program{}
	for _, program := range f.programs {
		if slices.Contains(channelIDs, program.ChannelID) && from.Before(program.EndAt) && program.StartAt.Before(to) {
			found = append(found, program)
		}
	}

	return found, nil
}

func (f *fakeSyobocal) set(programs []*syobocal.Program, fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.programs, f.fail = programs, fail
}

func (f *fakeSyobocal) takeCalls() []lookupCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = []lookupCall{}

	return calls
}

func syobocalProgram(pid, tid, channelID int32, start, end time.Time, count, subTitle, stSubTitle string, flag int32) *syobocal.Program {
	return &syobocal.Program{
		PID: pid, TID: tid, ChannelID: channelID, StartAt: start, EndAt: end, OffsetSeconds: 0,
		Count: count, SubTitle: subTitle, STSubTitle: stSubTitle, Flag: flag, Deleted: false, LastUpdate: time.Time{},
	}
}

// defaultSyobocal は、次の放送時間データを持つ偽物を返す。
//   - 地上波 (ChID 1)：11 月 2 日 0:00 から第 5 話、0:30 から第 1 話と第 2 話 (最終話)、削除された放送時間データ
//   - BS (ChID 9)：11 月 2 日 22:00 から、第 3 話と第 4 話をまとめた放送時間データ
func defaultSyobocal() *fakeSyobocal {
	deleted := syobocalProgram(103, 99, grChannel, jst(2, 0, 0), jst(2, 0, 30), "1", "", "", 0)
	deleted.Deleted = true

	return &fakeSyobocal{
		mu: sync.Mutex{},
		programs: []*syobocal.Program{
			syobocalProgram(100, 10, grChannel, jst(2, 0, 0), jst(2, 0, 30), "5", "", "第五話", syobocal.FlagNew),
			syobocalProgram(101, 11, grChannel, jst(2, 0, 30), jst(2, 1, 0), "1", "", "はじまり", 0),
			syobocalProgram(102, 11, grChannel, jst(2, 1, 0), jst(2, 1, 30), "2", "", "", syobocal.FlagFinal),
			deleted,
			syobocalProgram(200, 20, bsChannel, jst(2, 22, 0), jst(2, 23, 0), "3", "#3～#4", "", 0),
		},
		calls: []lookupCall{},
		fail:  false,
	}
}

// insertProgram は、番組を足す。
func insertProgram(t *testing.T, db *pgxpool.Pool, serviceID, eventID int64, start, end time.Time, genres string) {
	t.Helper()

	_, err := db.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		VALUES ($1, $2, $2, $3, $4, false, true, 'テスト', '', 'テスト', '', '{}', $5::jsonb, '[]', now(), now())`,
		serviceID, eventID, start, end, genres)
	require.NoError(t, err)
}

// episodeRow は、番組に結びついた話の行。
type episodeRow struct {
	Position   int16
	TID        *int32
	CountStart *float64
	CountEnd   *float64
	Subtitle   *string
	First      bool
	Last       bool
}

func episodesOfProgram(t *testing.T, db *pgxpool.Pool, serviceID int64, start time.Time) []episodeRow {
	t.Helper()

	rows, err := db.Query(t.Context(), `SELECT position, syobocal_tid, count_start, count_end, subtitle, is_first, is_last
		FROM program_anime_episodes WHERE service_id = $1 AND start_at = $2 ORDER BY position`, serviceID, start)
	require.NoError(t, err)
	defer rows.Close()
	result := []episodeRow{}
	for rows.Next() {
		var row episodeRow
		require.NoError(t, rows.Scan(&row.Position, &row.TID, &row.CountStart, &row.CountEnd, &row.Subtitle, &row.First, &row.Last))
		result = append(result, row)
	}
	require.NoError(t, rows.Err())

	return result
}

// matchRecord は、番組を照合した記録。
type matchRecord struct {
	found     bool
	finalized *time.Time
}

// matchOf は、番組を照合した記録を返す。照合していなければ found は false。
func matchOf(t *testing.T, db *pgxpool.Pool, serviceID int64, start time.Time) matchRecord {
	t.Helper()

	record := matchRecord{found: false, finalized: nil}
	err := db.QueryRow(t.Context(), `SELECT finalized_at FROM program_anime_matches WHERE service_id = $1 AND start_at = $2`,
		serviceID, start).Scan(&record.finalized)
	record.found = err == nil

	return record
}

// newMatchFixture は、連携を有効にし、チャンネルの対応を取り込んだ DB と、偽のしょぼいカレンダーを使う Matcher を用意する。
// 番組は、地上波に 11 月 2 日 0:00 からのアニメ (30 分)、0:30 からのアニメ (2 話連続の 60 分)、1:30 からのニュースを、
// BS に 22:00 からのアニメ (60 分) を置く。
func newMatchFixture(t *testing.T) (*pgxpool.Pool, *clocktest.Fake, *fakeSyobocal, *anime.Matcher) {
	t.Helper()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	enable(t, db, clk)
	_, err := anime.NewImporter(db, clk, newFakeFetcher(), newFakeTitles(), nil, slog.New(slog.DiscardHandler)).Import(t.Context(), false)
	require.NoError(t, err)
	insertProgram(t, db, grService, 1, jst(2, 0, 0), jst(2, 0, 30), genreAnime)
	insertProgram(t, db, grService, 2, jst(2, 0, 30), jst(2, 1, 30), genreAnime)
	insertProgram(t, db, grService, 3, jst(2, 1, 30), jst(2, 2, 0), genreNews)
	insertProgram(t, db, bsService, 1, jst(2, 22, 0), jst(2, 23, 0), genreAnime)
	client := defaultSyobocal()

	return db, clk, client, anime.NewMatcher(db, clk, client, slog.New(slog.DiscardHandler))
}

// TestMatchHorizon は、これから放送されるアニメの番組を、しょぼいカレンダーの放送時間データで話に結びつけることを確かめる。
// 同じ期間の ChID は 1 回の問い合わせにまとめ、引いた放送時間データは 6 時間使い回す。
func TestMatchHorizon(t *testing.T) {
	t.Parallel()

	db, clk, client, matcher := newMatchFixture(t)
	result, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, result.Programs, "the news program is not matched")
	assert.Equal(t, 3, result.Linked)
	calls := client.takeCalls()
	require.Len(t, calls, 1, "the channels of the same day are looked up at once")
	assert.Equal(t, []int32{grChannel, bsChannel}, calls[0].channelIDs)
	assert.True(t, calls[0].from.Equal(jst(2, 0, 0)), calls[0].from)
	assert.True(t, calls[0].to.Equal(jst(3, 0, 0)), calls[0].to)

	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(10)), CountStart: new(5.0), CountEnd: new(5.0), Subtitle: new("第五話"), First: true, Last: false},
	}, episodesOfProgram(t, db, grService, jst(2, 0, 0)), "the deleted program is ignored")
	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(11)), CountStart: new(1.0), CountEnd: new(1.0), Subtitle: new("はじまり"), First: false, Last: false},
		{Position: 1, TID: new(int32(11)), CountStart: new(2.0), CountEnd: new(2.0), Subtitle: nil, First: false, Last: true},
	}, episodesOfProgram(t, db, grService, jst(2, 0, 30)), "a program of two episodes links to both")
	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(20)), CountStart: new(3.0), CountEnd: new(4.0), Subtitle: nil, First: false, Last: false},
	}, episodesOfProgram(t, db, bsService, jst(2, 22, 0)), "the range of episodes is read from the subtitle")
	assert.False(t, matchOf(t, db, grService, jst(2, 1, 30)).found, "programs other than anime are not matched")

	// 6 時間たつまでは、引いた放送時間データを使い回す。
	clk.Advance(time.Hour)
	result, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Requests)
	assert.Empty(t, client.takeCalls())

	// 6 時間たったら引き直し、しょぼいカレンダーの訂正を反映する。消えた放送時間データは、キャッシュからも消す。
	clk.Advance(anime.CacheMaxAge)
	client.set([]*syobocal.Program{
		syobocalProgram(100, 10, grChannel, jst(2, 0, 0), jst(2, 0, 30), "6", "", "第六話", 0),
	}, false)
	_, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Len(t, client.takeCalls(), 1)
	assert.Equal(t, []episodeRow{
		{Position: 0, TID: new(int32(10)), CountStart: new(6.0), CountEnd: new(6.0), Subtitle: new("第六話"), First: false, Last: false},
	}, episodesOfProgram(t, db, grService, jst(2, 0, 0)))
	assert.Empty(t, episodesOfProgram(t, db, grService, jst(2, 0, 30)), "the episodes removed from syobocal are unlinked")
	assert.True(t, matchOf(t, db, grService, jst(2, 0, 30)).found, "a program without episodes is still recorded as matched")
}

// TestMatchDisabled は、連携が無効なら、しょぼいカレンダーに問い合わせないことを確かめる (設計書 0.3)。
func TestMatchDisabled(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(testNow())
	client := defaultSyobocal()
	insertProgram(t, db, grService, 1, jst(2, 0, 0), jst(2, 0, 30), genreAnime)
	matcher := anime.NewMatcher(db, clk, client, slog.New(slog.DiscardHandler))
	for _, run := range []func(context.Context) (*anime.MatchResult, error){matcher.MatchHorizon, matcher.MatchChanges, matcher.Finalize} {
		result, err := run(t.Context())
		require.NoError(t, err)
		assert.True(t, result.Disabled)
	}
	assert.Empty(t, client.takeCalls())
}

// recordEPGChange は、EPG の同期と同じく、版数を上げて、変わったサービスと時間帯を記録する。
func recordEPGChange(t *testing.T, db *pgxpool.Pool, serviceID int64, start, end time.Time) {
	t.Helper()

	_, err := db.Exec(t.Context(), `WITH bumped AS (
			INSERT INTO change_versions (channel, version, updated_at) VALUES ('epg', 1, now())
			ON CONFLICT (channel) DO UPDATE SET version = change_versions.version + 1 RETURNING version
		)
		INSERT INTO epg_changes (version, service_id, start_at, end_at, created_at) SELECT version, $1, $2, $3, now() FROM bumped`,
		serviceID, start, end)
	require.NoError(t, err)
}

// TestMatchChanges は、EPG の同期が通知したサービスと時間帯の番組だけを照合し、読んだ変更は読み直さないことを確かめる。
// 初めて動いたときは、今の版数を覚えるだけにする。
func TestMatchChanges(t *testing.T) {
	t.Parallel()

	db, _, client, matcher := newMatchFixture(t)
	recordEPGChange(t, db, grService, jst(1, 0, 0), jst(3, 0, 0))
	result, err := matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs, "the first run only remembers the version")

	recordEPGChange(t, db, bsService, jst(2, 21, 0), jst(2, 23, 0))
	result, err = matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Programs)
	calls := client.takeCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, []int32{bsChannel}, calls[0].channelIDs)
	assert.Len(t, episodesOfProgram(t, db, bsService, jst(2, 22, 0)), 1)
	assert.Empty(t, episodesOfProgram(t, db, grService, jst(2, 0, 0)), "services without changes are not matched")

	result, err = matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs, "changes already read are not read again")
}

// TestFinalize は、放送が終わった番組を、放送が終わった後に引いた放送時間データで照合し直し、確定させることを確かめる。
// 確定させた番組は照合し直さず、しょぼいカレンダーに問い合わせられなかった番組は確定させない。
func TestFinalize(t *testing.T) {
	t.Parallel()

	db, clk, client, matcher := newMatchFixture(t)
	_, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	client.takeCalls()

	// 地上波の 2 つのアニメが終わった。放送時間データは放送の前に引いたものなので、引き直す。
	clk.Set(jst(2, 1, 40))
	client.set(nil, true)
	result, err := matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Failed)
	assert.Zero(t, result.Finalized, "programs are not finalized while syobocal is down")
	assert.Nil(t, matchOf(t, db, grService, jst(2, 0, 0)).finalized)

	client.set([]*syobocal.Program{
		syobocalProgram(100, 10, grChannel, jst(2, 0, 0), jst(2, 0, 30), "5", "", "第五話 (訂正)", 0),
	}, false)
	client.takeCalls()
	result, err = matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, result.Finalized)
	calls := client.takeCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, []int32{grChannel}, calls[0].channelIDs)
	require.NotNil(t, matchOf(t, db, grService, jst(2, 0, 0)).finalized)
	assert.Equal(t, new("第五話 (訂正)"), episodesOfProgram(t, db, grService, jst(2, 0, 0))[0].Subtitle)

	// 確定させた番組は、照合し直さない。
	clk.Advance(anime.CacheMaxAge)
	result, err = matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs)
	result, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Programs, "only the BS program, which has not ended, is matched again")
}

// archiveProgram は、アーカイブへ移す処理と同じく、番組を過去の番組へ写す。
func archiveProgram(t *testing.T, db *pgxpool.Pool, serviceID, eventID int64) {
	t.Helper()

	_, err := db.Exec(t.Context(), `INSERT INTO program_archives (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided,
		is_free, name, description, name_normalized, description_normalized, extended_normalized, extended, genres, related_items,
		audios, video_resolution, created_at, updated_at, archived_at)
		SELECT service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free, name, description, name_normalized,
		description_normalized, extended_normalized, extended, genres, related_items, audios, video_resolution, created_at, updated_at, now()
		FROM programs WHERE service_id = $1 AND event_id = $2`, serviceID, eventID)
	require.NoError(t, err)
}

// TestMatchMovedProgram は、放送時間が変わった番組 (設計書 4.6) の、前の放送時間の結びつきを消すことを確かめる。
// 前の放送時間の番組をアーカイブへ移していれば、同じイベント ID でも別の放送なので、その結びつきは残す。
func TestMatchMovedProgram(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		archived bool
		message  string
	}{
		{name: "moved", archived: false, message: "the link of the old time is deleted"},
		{name: "archived", archived: true, message: "the link of the archived program is kept"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db, clk, client, matcher := newMatchFixture(t)
			_, err := matcher.MatchHorizon(t.Context())
			require.NoError(t, err)
			if tt.archived {
				archiveProgram(t, db, bsService, 1)
			}

			_, err = db.Exec(t.Context(), `UPDATE programs SET start_at = $3, end_at = $4 WHERE service_id = $1 AND event_id = $2`,
				bsService, 1, jst(2, 22, 30), jst(2, 23, 30))
			require.NoError(t, err)
			client.set([]*syobocal.Program{
				syobocalProgram(200, 20, bsChannel, jst(2, 22, 30), jst(2, 23, 30), "3", "#3～#4", "", 0),
			}, false)
			clk.Advance(anime.CacheMaxAge)
			_, err = matcher.MatchHorizon(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tt.archived, matchOf(t, db, bsService, jst(2, 22, 0)).found, tt.message)
			assert.Len(t, episodesOfProgram(t, db, bsService, jst(2, 22, 30)), 1)
		})
	}
}

// exists は、query に当たる行があるかを返す。
func exists(t *testing.T, db *pgxpool.Pool, query string, args ...any) bool {
	t.Helper()

	var found bool
	require.NoError(t, db.QueryRow(t.Context(), `SELECT EXISTS (`+query+`)`, args...).Scan(&found))

	return found
}

// TestMatchPrune は、放送が終わって CacheRetention を過ぎた放送時間データと、その前の日の問い合わせの記録を消し、
// 番組表からも過去の番組からもなくなって OrphanGrace を過ぎた結びつきを消すことを確かめる。それより新しいものは残す。
func TestMatchPrune(t *testing.T) {
	t.Parallel()

	db, clk, _, matcher := newMatchFixture(t)
	now := clk.Now()
	expired, kept := now.Add(-anime.CacheRetention-time.Minute), now.Add(-anime.CacheRetention+time.Minute)
	for pid, end := range map[int32]time.Time{900: expired, 901: kept} {
		_, err := db.Exec(t.Context(), `INSERT INTO syobocal_programs (pid, tid, channel_id, start_at, end_at, offset_seconds, count,
			sub_title, st_sub_title, flag, deleted, fetched_at) VALUES ($1, 90, $2, $3, $4, 0, '1', '', '', 0, false, $4)`,
			pid, grChannel, end.Add(-30*time.Minute), end)
		require.NoError(t, err)
	}
	expiredDay, keptDay := expired.Add(-24*time.Hour), kept.Add(-24*time.Hour)
	for _, day := range []time.Time{expiredDay, keptDay} {
		_, err := db.Exec(t.Context(), `INSERT INTO syobocal_lookups (channel_id, day_start, fetched_at) VALUES ($1, $2, $2)`,
			grChannel, day)
		require.NoError(t, err)
	}
	// 番組のない結びつき。古いもの、照合したばかりのもの、アーカイブへ移した番組のものを置く。
	insertProgram(t, db, bsService, 9, jst(1, 0, 0), jst(1, 0, 30), genreAnime)
	archiveProgram(t, db, bsService, 9)
	_, err := db.Exec(t.Context(), `DELETE FROM programs WHERE service_id = $1 AND event_id = 9`, bsService)
	require.NoError(t, err)
	for start, matchedAt := range map[time.Time]time.Time{
		jst(1, 1, 0): now.Add(-anime.OrphanGrace - time.Minute),
		jst(1, 2, 0): now.Add(-anime.OrphanGrace + time.Minute),
		jst(1, 0, 0): now.Add(-anime.OrphanGrace - time.Minute),
	} {
		_, err := db.Exec(t.Context(), `INSERT INTO program_anime_matches (service_id, start_at, event_id, matched_at) VALUES ($1, $2, 9, $3)`,
			bsService, start, matchedAt)
		require.NoError(t, err)
	}

	result, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.Pruned)
	assert.False(t, exists(t, db, `SELECT 1 FROM syobocal_programs WHERE pid = 900`), "the expired program is deleted")
	assert.True(t, exists(t, db, `SELECT 1 FROM syobocal_programs WHERE pid = 901`), "the recent program is kept")
	lookup := `SELECT 1 FROM syobocal_lookups WHERE channel_id = $1 AND day_start = $2`
	assert.False(t, exists(t, db, lookup, grChannel, expiredDay), "the expired lookup is deleted")
	assert.True(t, exists(t, db, lookup, grChannel, keptDay), "the recent lookup is kept")
	assert.False(t, matchOf(t, db, bsService, jst(1, 1, 0)).found, "the orphaned link is deleted")
	assert.True(t, matchOf(t, db, bsService, jst(1, 2, 0)).found, "the link just matched is kept")
	assert.True(t, matchOf(t, db, bsService, jst(1, 0, 0)).found, "the link of the archived program is kept")
	assert.True(t, matchOf(t, db, grService, jst(2, 0, 0)).found, "the link of the program in the EPG is kept")
}
