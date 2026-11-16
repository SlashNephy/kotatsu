package drama_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/drama"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// grService と bsService は、テストの地上波と BS のサービス。
	grService = 32736*100000 + 1024
	bsService = 4*100000 + 171
	// genreDrama は、ドラマ (大分類 0x3) のジャンル。genreInfoDrama は、情報番組の後にドラマを持つジャンル。
	genreDrama     = `[{"lv1": 3, "lv2": 0, "un1": 15, "un2": 15}]`
	genreInfoDrama = `[{"lv1": 2, "lv2": 7, "un1": 15, "un2": 15}, {"lv1": 3, "lv2": 0, "un1": 15, "un2": 15}]`
	// TMDb の作品の ID。
	moPapa      = 331304
	banshaku    = 202396
	hagure      = 133940
	sukeban     = 89352
	sanada      = 332649
	brave10     = 42502
	testTMDbKey = "test-token"
	// moPapaTitle は、もうパパ！の番組名から読む作品名。
	moPapaTitle = "もうパパ!"
	// banshakuEpisode7 は、晩酌の流儀５ (TMDb のシーズン 7) の第 7 話の番組名。
	banshakuEpisode7 = "晩酌の流儀５〜秋冬篇〜第７話「ブーリンチー」🈑"
)

// jst は、2026 年 11 月 day 日 hour 時 minute 分 (日本時間) を返す。
func jst(day, hour, minute int) time.Time {
	return time.Date(2026, time.November, day, hour, minute, 0, 0, time.FixedZone("JST", 9*60*60))
}

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := database.Open(t.Context(), databasetest.New(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, database.Migrate(t.Context(), slog.New(slog.DiscardHandler), pool))
	_, err = pool.Exec(t.Context(), `INSERT INTO channel_types (type, display_name, sort_order) VALUES ('GR', '地上波', 0), ('BS', 'BS', 1)`)
	require.NoError(t, err)
	for _, service := range []struct {
		id, networkID, sid int64
		channelType        string
	}{
		{id: grService, networkID: 32736, sid: 1024, channelType: "GR"},
		{id: bsService, networkID: 4, sid: 171, channelType: "BS"},
	} {
		_, err := pool.Exec(t.Context(), `INSERT INTO services (id, network_id, sid, name, name_normalized, service_type, channel_type, channel, created_at, updated_at)
			VALUES ($1, $2, $3, 'テスト', 'テスト', 1, $4, '27', now(), now())`, service.id, service.networkID, service.sid, service.channelType)
		require.NoError(t, err)
	}

	return pool
}

// enable は、TMDb との連携を有効にする。
// setToken は、管理者が連携の画面で入れたのと同じく、TMDb のトークンを token にする。空なら消す。
func setToken(t *testing.T, db *pgxpool.Pool, tokens *drama.Tokens, token string) {
	t.Helper()

	require.NoError(t, tokens.Update(t.Context(), db, audit.System("test"), token))
}

// newTokens は、テストのアプリの秘密キーで TMDb のトークンを持つ Tokens を返す。
func newTokens(t *testing.T, clk *clocktest.Fake) *drama.Tokens {
	t.Helper()

	tokens, err := drama.NewTokens(clk, audit.NewRecorder(clk), strings.Repeat("k", 32))
	require.NoError(t, err)

	return tokens
}

// newClient は、偽の TMDb に問い合わせる Client を返す。
func newClient(t *testing.T, fake *fakeTMDb) *tmdb.Client {
	t.Helper()

	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	return tmdb.NewClient(&tmdb.Options{HTTPClient: tmdb.NewHTTPClient(nil), URL: server.URL, Interval: time.Millisecond})
}

func enable(t *testing.T, db *pgxpool.Pool, clk *clocktest.Fake) {
	t.Helper()

	_, err := settings.NewStore(clk, audit.NewRecorder(clk)).Update(t.Context(), db, audit.System("test"), &settings.Changes{
		Recording: nil, ArchiveRetentionDays: nil, NotificationRetentionDays: nil, Rule: nil, MCPEnabled: nil,
		DramaLinkingEnabled: new(true), DRCSReplacements: nil,
	})
	require.NoError(t, err)
}

// insertProgram は、番組を足す。
func insertProgram(t *testing.T, db *pgxpool.Pool, serviceID, eventID int64, start, end time.Time, name, genres string) {
	t.Helper()

	_, err := db.Exec(t.Context(), `INSERT INTO programs (service_id, event_id, mirakurun_id, start_at, end_at, end_undecided, is_free,
		name, description, name_normalized, description_normalized, extended, genres, related_items, created_at, updated_at)
		VALUES ($1, $2, $2, $3, $4, false, true, $5, '', $5, '', '{}', $6::jsonb, '[]', now(), now())`,
		serviceID, eventID, start, end, name, genres)
	require.NoError(t, err)
}

// insertRecording は、番組 (serviceID, start) を録った録画を足す。
func insertRecording(t *testing.T, db *pgxpool.Pool, serviceID int64, start, end time.Time) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := db.Exec(t.Context(), `INSERT INTO recordings (id, service_id, name, description, start_at, end_at, state, recorded_from, created_at, updated_at)
		VALUES ($1, $2, 'テスト', '', $3, $4, 'recording', $3, now(), now())`, id, serviceID, start, end)
	require.NoError(t, err)

	return id
}

// fakeTMDb は、決めた検索の結果、作品、シーズンを返す偽の TMDb。受けた問い合わせの数を数える。
type fakeTMDb struct {
	mu       sync.Mutex
	searches map[string]string
	series   map[string]string
	seasons  map[string]string
	requests []string
	// limited なら、すべての問い合わせに 429 を返す。
	limited bool
}

func (f *fakeTMDb) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.Path+"?"+r.URL.Query().Get("query"))
	if r.Header.Get("Authorization") != "Bearer "+testTMDbKey {
		w.WriteHeader(http.StatusUnauthorized)

		return
	}
	if f.limited {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)

		return
	}
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/3/")
	switch {
	case path == "search/tv":
		results, ok := f.searches[r.URL.Query().Get("query")]
		if !ok {
			results = ""
		}
		_, _ = w.Write([]byte(`{"page":1,"results":[` + results + `]}`))
	case strings.Contains(path, "/season/"):
		body, ok := f.seasons[strings.TrimPrefix(path, "tv/")]
		if !ok {
			http.NotFound(w, r)

			return
		}
		_, _ = w.Write([]byte(`{"episodes":[` + body + `]}`))
	default:
		body, ok := f.series[strings.TrimPrefix(path, "tv/")]
		if !ok {
			http.NotFound(w, r)

			return
		}
		_, _ = w.Write([]byte(body))
	}
}

func (f *fakeTMDb) takeRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	requests := f.requests
	f.requests = []string{}

	return requests
}

func (f *fakeTMDb) setLimited(limited bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limited = limited
}

// searchItem と seasonItem と episodeItem は、偽の TMDb の応答の JSON の形。
type searchItem struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
	GenreIDs     []int  `json:"genre_ids"`
}

type seasonItem struct {
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	AirDate      string `json:"air_date"`
	EpisodeCount int    `json:"episode_count"`
}

type seriesBody struct {
	ID           int          `json:"id"`
	Name         string       `json:"name"`
	OriginalName string       `json:"original_name"`
	FirstAirDate string       `json:"first_air_date"`
	Seasons      []seasonItem `json:"seasons"`
}

type episodeItem struct {
	EpisodeNumber int    `json:"episode_number"`
	AirDate       string `json:"air_date"`
	Name          string `json:"name"`
}

// mustJSON は、v を JSON にする。テストの表を作るのに使うので、失敗したら止める。
func mustJSON(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return string(encoded)
}

// result は、検索の結果の作品 1 つを JSON にする。
func result(id int, title, firstAirDate string, genres ...int) string {
	return mustJSON(searchItem{ID: id, Name: title, OriginalName: title, FirstAirDate: firstAirDate, GenreIDs: genres})
}

// seriesJSON は、作品を JSON にする。seasons は、シーズンの番号と、始まった日を @ でつないだもの。
func seriesJSON(id int, title, firstAirDate string, seasons ...string) string {
	list := make([]seasonItem, 0, len(seasons))
	for _, season := range seasons {
		number, airDate, _ := strings.Cut(season, "@")
		n, _ := strconv.Atoi(number)
		list = append(list, seasonItem{SeasonNumber: n, Name: "", AirDate: airDate, EpisodeCount: 10})
	}

	return mustJSON(seriesBody{ID: id, Name: title, OriginalName: title, FirstAirDate: firstAirDate, Seasons: list})
}

// episodesJSON は、話の一覧を JSON にする。airDates は、1 話から順の放送日。
func episodesJSON(airDates ...string) string {
	list := make([]string, 0, len(airDates))
	for i, airDate := range airDates {
		list = append(list, mustJSON(episodeItem{EpisodeNumber: i + 1, AirDate: airDate, Name: "第" + strconv.Itoa(i+1) + "話の名前"}))
	}

	return strings.Join(list, ",")
}

// newFakeTMDb は、次の作品を持つ偽の TMDb を返す (作品と放送日は、2026 年 11 月の実際の TMDb の載り方に合わせた)。
//   - もうパパ！：シーズン 1 の第 7 話が 11 月 15 日。
//   - 晩酌の流儀：TMDb ではシーズン 7 が「晩酌の流儀5 秋冬篇」。第 7 話が 11 月 14 日、第 8 話が 11 月 21 日。
//   - はぐれ刑事純情派：シーズンが 18 ある昔の作品。
//   - スケバン刑事II 少女鉄仮面伝説：シーズンが 1 つだけの昔の作品。
//   - 真田十勇士 SHADOWS OF THE TEN NINJA：作品名の一部 (真田十勇士) で探すと、アニメの BRAVE 10 と並んで出る。第 5 話が 11 月 20 日。
func newFakeTMDb() *fakeTMDb {
	hagureSeasons := []string{}
	for i := 1; i <= 18; i++ {
		hagureSeasons = append(hagureSeasons, strconv.Itoa(i)+"@"+strconv.Itoa(1987+i)+"-04-06")
	}

	return &fakeTMDb{
		mu: sync.Mutex{},
		searches: map[string]string{
			moPapaTitle: result(moPapa, "もうパパ！", "2026-10-04", 18) + "," + result(228278, "もう一度パパと呼ばれる日", "2023-06-06", 18),
			"晩酌の流儀":     result(banshaku, "晩酌の流儀", "2022-07-02", 18),
			"はぐれ刑事純情派":  result(hagure, "はぐれ刑事純情派", "1988-04-06", 80, 18),
			"スケバン刑事II 少女鉄仮面伝説": result(sukeban, "スケバン刑事II 少女鉄仮面伝説", "1985-11-08", 80, 18),
			"真田十勇士": result(sanada, "真田十勇士 SHADOWS OF THE TEN NINJA", "2026-10-23", 10759) + "," +
				result(brave10, "BRAVE 10", "2012-01-08", 16, 10759),
		},
		series: map[string]string{
			strconv.Itoa(moPapa):   seriesJSON(moPapa, "もうパパ！", "2026-10-04", "1@2026-10-04"),
			strconv.Itoa(banshaku): seriesJSON(banshaku, "晩酌の流儀", "2022-07-02", "0@2022-12-31", "6@2026-07-03", "7@2026-10-03"),
			strconv.Itoa(hagure):   seriesJSON(hagure, "はぐれ刑事純情派", "1988-04-06", hagureSeasons...),
			strconv.Itoa(sukeban):  seriesJSON(sukeban, "スケバン刑事II 少女鉄仮面伝説", "1985-11-08", "1@1985-11-08"),
			strconv.Itoa(sanada):   seriesJSON(sanada, "真田十勇士 SHADOWS OF THE TEN NINJA", "2026-10-23", "1@2026-10-23"),
		},
		seasons: map[string]string{
			strconv.Itoa(moPapa) + "/season/1":   episodesJSON("2026-10-04", "2026-10-11", "2026-10-18", "2026-10-25", "2026-11-01", "2026-11-08", "2026-11-15"),
			strconv.Itoa(banshaku) + "/season/0": episodesJSON("2022-12-31"),
			strconv.Itoa(banshaku) + "/season/6": episodesJSON("2026-07-03"),
			strconv.Itoa(banshaku) + "/season/7": episodesJSON(
				"2026-10-03", "2026-10-10", "2026-10-17", "2026-10-24", "2026-10-31", "2026-11-07", "2026-11-14", "2026-11-21"),
			strconv.Itoa(sukeban) + "/season/1": episodesJSON(func() []string {
				dates := make([]string, 0, 42)
				for range 42 {
					dates = append(dates, "1986-01-01")
				}

				return dates
			}()...),
			strconv.Itoa(sanada) + "/season/1": episodesJSON("2026-10-23", "2026-10-30", "2026-11-06", "2026-11-13", "2026-11-20"),
		},
		requests: []string{},
		limited:  false,
	}
}

// matchRow は、番組の結びつきの行。
type matchRow struct {
	SeriesID      *int32
	Season        *int32
	Episode       *int32
	SeriesSource  *string
	EpisodeSource *string
	Finalized     bool
}

func matchOf(t *testing.T, db *pgxpool.Pool, serviceID int64, start time.Time) (matchRow, bool) {
	t.Helper()

	var row matchRow
	err := db.QueryRow(t.Context(), `SELECT tmdb_series_id, season_number, episode_number, series_source, episode_source, finalized_at IS NOT NULL
		FROM program_drama_matches WHERE service_id = $1 AND start_at = $2`, serviceID, start).
		Scan(&row.SeriesID, &row.Season, &row.Episode, &row.SeriesSource, &row.EpisodeSource, &row.Finalized)

	return row, err == nil
}

func linkedTo(series, season, episode int32, source string) matchRow {
	return matchRow{
		SeriesID: new(series), Season: new(season), Episode: new(episode), SeriesSource: new(drama.SeriesSourceSearch),
		EpisodeSource: new(source), Finalized: false,
	}
}

func seriesOnly(series int32) matchRow {
	return matchRow{SeriesID: new(series), Season: nil, Episode: nil, SeriesSource: new(drama.SeriesSourceSearch), EpisodeSource: nil, Finalized: false}
}

// fixture は、照合のテストの DB と、偽の TMDb、Matcher。
type fixture struct {
	db      *pgxpool.Pool
	clk     *clocktest.Fake
	fake    *fakeTMDb
	matcher *drama.Matcher
}

// newFixture は、連携を有効にした DB に、2026 年 11 月 15 日から 1 週間のドラマの番組を置き、偽の TMDb を使う Matcher を用意する。
func newFixture(t *testing.T, maxRequests int) *fixture {
	t.Helper()

	db := newDB(t)
	clk := clocktest.NewFake(jst(15, 0, 0))
	enable(t, db, clk)
	fake := newFakeTMDb()
	tokens := newTokens(t, clk)
	setToken(t, db, tokens, testTMDbKey)

	return &fixture{db: db, clk: clk, fake: fake, matcher: drama.NewMatcher(db, clk, newClient(t, fake), tokens, slog.New(slog.DiscardHandler), maxRequests)}
}

// TestMatchHorizon は、ドラマの番組を、TMDb の作品と話に結びつけることを確かめる (#1651)。
func TestMatchHorizon(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	programs := []struct {
		name       string
		serviceID  int64
		start, end time.Time
		title      string
		genres     string
		want       *matchRow
	}{
		{
			name: "air date of the episode", serviceID: grService, start: jst(15, 22, 15), end: jst(15, 23, 9),
			title: "もうパパ！　＃７　【心底ムカついています】脚本・吉田恵里香×原菜乃華 堤真一🈑🈖", genres: genreDrama,
			want: new(linkedTo(moPapa, 1, 7, drama.EpisodeSourceAirDate)),
		},
		{
			// 番組名の「5」は続編の番号で、TMDb のシーズン (7) と合わない。BS は地上波より遅れて放送するので、放送日ではなく話数で選ぶ。
			name: "delayed broadcast uses the episode number", serviceID: bsService, start: jst(17, 0, 0), end: jst(17, 0, 30),
			title: banshakuEpisode7, genres: genreDrama,
			want: new(linkedTo(banshaku, 7, 7, drama.EpisodeSourceTitle)),
		},
		{
			// 1 週遅れて放送する局では、TMDb の放送日に次の話 (第 8 話) が当たる。番組名の話数と食い違うので、番組名の話数で選ぶ。
			name: "delayed by a week", serviceID: bsService, start: jst(21, 2, 0), end: jst(21, 2, 30),
			title: banshakuEpisode7, genres: genreDrama,
			want: new(linkedTo(banshaku, 7, 7, drama.EpisodeSourceTitle)),
		},
		{
			// 深夜の番組でも、TMDb の放送日 (暦の日) で選ぶ。
			name: "late night broadcast", serviceID: grService, start: jst(21, 0, 52), end: jst(21, 1, 23),
			title: "晩酌の流儀５〜秋冬篇〜第８話「すき焼き」🈑", genres: genreDrama,
			want: new(linkedTo(banshaku, 7, 8, drama.EpisodeSourceAirDate)),
		},
		{
			// 再放送は放送日では選べない。シーズンがいくつもある作品は、どのシーズンの話か決められないので、作品だけに結びつける。
			name: "rerun of a series with many seasons", serviceID: grService, start: jst(16, 12, 0), end: jst(16, 13, 0),
			title: "懐ドラ「はぐれ刑事純情派２」　＃３🈞", genres: genreDrama,
			want: new(seriesOnly(hagure)),
		},
		{
			name: "rerun of a series with one season", serviceID: grService, start: jst(16, 16, 0), end: jst(16, 17, 0),
			title: "スケバン刑事Ⅱ　少女鉄仮面伝説　★第２６話🈞", genres: genreDrama,
			want: new(linkedTo(sukeban, 1, 26, drama.EpisodeSourceTitle)),
		},
		{
			// 再放送の印がなくても、今放送しているシーズンのない昔の作品は、再放送と同じく番組名の話数で選ぶ。
			name: "unmarked rerun of an old series", serviceID: bsService, start: jst(17, 16, 0), end: jst(17, 17, 0),
			title: "スケバン刑事Ⅱ　少女鉄仮面伝説　★第２７話", genres: genreDrama,
			want: new(linkedTo(sukeban, 1, 27, drama.EpisodeSourceTitle)),
		},
		{
			// 作品名の一部で探すと、作品名の一致しない候補しか出ない。放送日の話がある候補だけに結びつけ、アニメは候補にしない。
			name: "partial title confirmed by the air date", serviceID: grService, start: jst(20, 23, 0), end: jst(20, 23, 54),
			title: "真田十勇士　＃５", genres: genreDrama,
			want: new(linkedTo(sanada, 1, 5, drama.EpisodeSourceAirDate)),
		},
		{
			name: "promotion is not matched", serviceID: grService, start: jst(15, 23, 52), end: jst(16, 0, 7),
			title: "【ドラマ】夜ドラ「青の花　器の森」ＰＲ　（第４週）", genres: genreDrama,
			want: &matchRow{SeriesID: nil, Season: nil, Episode: nil, SeriesSource: nil, EpisodeSource: nil, Finalized: false},
		},
		{
			name: "short program is not matched", serviceID: grService, start: jst(15, 23, 9), end: jst(15, 23, 15),
			title: "もうパパ！　＃８", genres: genreDrama,
			want: &matchRow{SeriesID: nil, Season: nil, Episode: nil, SeriesSource: nil, EpisodeSource: nil, Finalized: false},
		},
		{
			name: "drama genre after another genre is not a target", serviceID: grService, start: jst(16, 1, 30), end: jst(16, 2, 0),
			title: "ライターズ！", genres: genreInfoDrama,
			want: nil,
		},
	}
	for i, p := range programs {
		insertProgram(t, f.db, p.serviceID, int64(i+1), p.start, p.end, p.title, p.genres)
	}
	recordingID := insertRecording(t, f.db, grService, jst(15, 22, 15), jst(15, 23, 9))

	result, err := f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 10, result.Programs)
	assert.Equal(t, 8, result.Linked)
	assert.Equal(t, 7, result.Episodes)
	assert.Zero(t, result.Failed)
	for _, p := range programs {
		got, found := matchOf(t, f.db, p.serviceID, p.start)
		if p.want == nil {
			assert.False(t, found, p.name)

			continue
		}
		if assert.True(t, found, p.name) {
			assert.Equal(t, *p.want, got, p.name)
		}
	}
	requests := f.fake.takeRequests()
	assert.NotContains(t, requests, "/3/search/tv?夜ドラ", "promotions are not looked up")
	assert.Contains(t, requests, "/3/search/tv?晩酌の流儀5〜秋冬篇〜", "the full title is tried first")

	// 録画にも、番組の結びつきを写す。
	episode, found, err := drama.RecordingEpisode(t.Context(), f.db, recordingID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, &drama.LinkedEpisode{
		SeriesID: moPapa, SeriesName: new("もうパパ！"), SeriesOriginalName: new("もうパパ！"), SeasonNumber: new(int32(1)), EpisodeNumber: new(int32(7)),
		EpisodeName: new("第7話の名前"), SeriesSource: drama.SeriesSourceSearch, EpisodeSource: drama.EpisodeSourceAirDate,
	}, episode)

	// キャッシュが新しい間は、問い合わせ直さない。
	f.clk.Advance(time.Hour)
	result, err = f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Requests)
	assert.Empty(t, f.fake.takeRequests())
}

// TestMatchStopsWhenRateLimited は、TMDb が 429 を返したら、残りの番組を書かずに次の機会に回すことを確かめる。
func TestMatchStopsWhenRateLimited(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	insertProgram(t, f.db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)
	insertProgram(t, f.db, grService, 2, jst(16, 16, 0), jst(16, 17, 0), "スケバン刑事Ⅱ　少女鉄仮面伝説　★第２６話🈞", genreDrama)
	f.fake.setLimited(true)

	result, err := f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Requests, "no more requests after a 429")
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, 2, result.Deferred)
	assert.True(t, result.Stopped)
	assert.Equal(t, time.Minute, result.ContinueAfter, "the job continues after the Retry-After of TMDb")
	_, found := matchOf(t, f.db, grService, jst(15, 22, 15))
	assert.False(t, found, "the deferred program is matched later")

	// Retry-After の間は、ほかの照合も問い合わせない。
	f.fake.setLimited(false)
	_, err = f.matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	result, err = f.matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.True(t, result.Paused)
	assert.Empty(t, f.fake.takeRequests()[1:], "no requests while TMDb asks to wait")

	f.clk.Advance(time.Minute)
	_, err = f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	got, found := matchOf(t, f.db, grService, jst(15, 22, 15))
	require.True(t, found)
	assert.Equal(t, linkedTo(moPapa, 1, 7, drama.EpisodeSourceAirDate), got)
}

// TestMatchRequestBudget は、1 回の照合で問い合わせる回数に上限があることを確かめる。
func TestMatchRequestBudget(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 2)
	insertProgram(t, f.db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)
	insertProgram(t, f.db, grService, 2, jst(16, 16, 0), jst(16, 17, 0), "スケバン刑事Ⅱ　少女鉄仮面伝説　★第２６話🈞", genreDrama)

	result, err := f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, result.Requests)
	assert.Equal(t, 2, result.Deferred, "the program in the middle of the lookups and the rest are deferred")
	assert.True(t, result.Stopped, "the job continues the rest a little later")
	assert.Len(t, f.fake.takeRequests(), 2)
}

// TestOverride は、管理者の上書きで、作品名の番組を別の作品に結びつけたり、結びつけなかったりできることを確かめる。
func TestOverride(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	const title = "もうパパ！　＃７"
	insertProgram(t, f.db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), title, genreDrama)
	overrides := drama.NewOverrides(f.clk, audit.NewRecorder(f.clk), nil)
	key := drama.TitleKeyOf(title)

	require.NoError(t, overrides.Update(t.Context(), f.db, audit.System("test"), key, &drama.OverrideChange{SeriesID: nil, Exclude: true, Reset: false}))
	_, err := f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	got, found := matchOf(t, f.db, grService, jst(15, 22, 15))
	require.True(t, found)
	assert.Nil(t, got.SeriesID, "an excluded title is not linked")
	assert.Empty(t, f.fake.takeRequests(), "an excluded title is not looked up")

	require.NoError(t, overrides.Update(t.Context(), f.db, audit.System("test"), key, &drama.OverrideChange{SeriesID: new(int32(banshaku)), Exclude: false, Reset: false}))
	_, err = f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	got, _ = matchOf(t, f.db, grService, jst(15, 22, 15))
	assert.Equal(t, new(int32(banshaku)), got.SeriesID)
	assert.Equal(t, new(drama.SeriesSourceOverride), got.SeriesSource)

	require.NoError(t, overrides.Update(t.Context(), f.db, audit.System("test"), key, &drama.OverrideChange{SeriesID: nil, Exclude: false, Reset: true}))
	_, err = f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	got, _ = matchOf(t, f.db, grService, jst(15, 22, 15))
	assert.Equal(t, linkedTo(moPapa, 1, 7, drama.EpisodeSourceAirDate), got, "resetting goes back to the search")

	for _, change := range []*drama.OverrideChange{
		{SeriesID: nil, Exclude: false, Reset: false},
		{SeriesID: new(int32(1)), Exclude: true, Reset: false},
		{SeriesID: new(int32(0)), Exclude: false, Reset: false},
		{SeriesID: nil, Exclude: true, Reset: true},
	} {
		assert.ErrorIs(t, overrides.Update(t.Context(), f.db, audit.System("test"), key, change), drama.ErrInvalidOverride)
	}
}

// TestFinalize は、放送が終わった番組の結びつきを確定させ、それからは照合し直さないことを確かめる。
func TestFinalize(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	insertProgram(t, f.db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)
	f.clk.Set(jst(15, 23, 30))

	result, err := f.matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Finalized)
	got, _ := matchOf(t, f.db, grService, jst(15, 22, 15))
	assert.True(t, got.Finalized)

	result, err = f.matcher.Finalize(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs, "a finalized program is not matched again")
}

// TestMatchDisabled は、連携が無効なら TMDb に問い合わせないことを確かめる。
func TestMatchDisabled(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(jst(15, 0, 0))
	fake := newFakeTMDb()
	tokens := newTokens(t, clk)
	matcher := drama.NewMatcher(db, clk, newClient(t, fake), tokens, slog.New(slog.DiscardHandler), 0)
	insertProgram(t, db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)

	tests := []struct {
		name  string
		token string
		on    bool
	}{
		{name: "linking is disabled", token: testTMDbKey, on: false},
		{name: "token is not configured", token: "", on: true},
	}
	for _, tt := range tests {
		if tt.on {
			enable(t, db, clk)
		}
		setToken(t, db, tokens, tt.token)
		result, err := matcher.MatchHorizon(t.Context())
		require.NoError(t, err, tt.name)
		assert.True(t, result.Disabled, tt.name)
		assert.Empty(t, fake.takeRequests(), tt.name)
	}
}

// TestMatchUnauthorized は、トークンが断られたら、続きとして照合し直さず、同じトークンでは 1 時間は問い合わせないことと、
// 管理者がトークンを入れ直したら、1 時間を待たずに問い合わせることを確かめる。
func TestMatchUnauthorized(t *testing.T) {
	t.Parallel()

	db := newDB(t)
	clk := clocktest.NewFake(jst(15, 0, 0))
	enable(t, db, clk)
	fake := newFakeTMDb()
	tokens := newTokens(t, clk)
	setToken(t, db, tokens, "wrong-token")
	insertProgram(t, db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)
	insertProgram(t, db, grService, 2, jst(16, 16, 0), jst(16, 17, 0), "スケバン刑事Ⅱ　少女鉄仮面伝説　★第２６話🈞", genreDrama)
	matcher := drama.NewMatcher(db, clk, newClient(t, fake), tokens, slog.New(slog.DiscardHandler), 0)

	result, err := matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Requests)
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, 2, result.Deferred)
	assert.False(t, result.Stopped, "a rejected token is not retried as a continuation")
	assert.Len(t, fake.takeRequests(), 1)

	clk.Advance(30 * time.Minute)
	result, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.True(t, result.Paused)
	assert.Empty(t, fake.takeRequests(), "no requests for an hour after the token is rejected")

	clk.Advance(30 * time.Minute)
	_, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.Len(t, fake.takeRequests(), 1, "queries again after an hour")

	// 断られた直後でも、入れ直したトークンでは問い合わせる。
	setToken(t, db, tokens, testTMDbKey)
	result, err = matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.False(t, result.Paused, "a new token is not paused")
	assert.Equal(t, 2, result.Linked)
	assert.NotEmpty(t, fake.takeRequests())
}

// TestMatchChanges は、EPG の変更の記録を読んで、変わったサービスと時間帯の番組だけを照合し、読んだ版数を覚えることを確かめる。
func TestMatchChanges(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	insertProgram(t, f.db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)
	insertProgram(t, f.db, bsService, 1, jst(17, 0, 0), jst(17, 0, 30), banshakuEpisode7, genreDrama)

	// 初めて動いたときは、今の版数を覚えるだけにする。それまでの番組は、照合し直しが受け持つ。
	result, err := f.matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs)

	_, err = f.db.Exec(t.Context(), `INSERT INTO epg_changes (version, service_id, start_at, end_at, created_at) VALUES (1, $1, $2, $3, now())`,
		grService, jst(15, 22, 0), jst(15, 23, 30))
	require.NoError(t, err)
	_, err = f.db.Exec(t.Context(), `INSERT INTO change_versions (channel, version, updated_at) VALUES ('epg', 1, now())
		ON CONFLICT (channel) DO UPDATE SET version = 1`)
	require.NoError(t, err)

	result, err = f.matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Programs, "only the changed service and time are matched")
	got, found := matchOf(t, f.db, grService, jst(15, 22, 15))
	require.True(t, found)
	assert.Equal(t, linkedTo(moPapa, 1, 7, drama.EpisodeSourceAirDate), got)
	_, found = matchOf(t, f.db, bsService, jst(17, 0, 0))
	assert.False(t, found)

	result, err = f.matcher.MatchChanges(t.Context())
	require.NoError(t, err)
	assert.Zero(t, result.Programs, "the read changes are not read again")
}

// TestPruneCache は、TMDb から引いた情報を、引いてから 180 日で消すことを確かめる (TMDb の利用規約)。
func TestPruneCache(t *testing.T) {
	t.Parallel()

	f := newFixture(t, 0)
	insertProgram(t, f.db, grService, 1, jst(15, 22, 15), jst(15, 23, 9), "もうパパ！　＃７", genreDrama)
	_, err := f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	count := func() int {
		var n int
		require.NoError(t, f.db.QueryRow(t.Context(),
			`SELECT (SELECT count(*) FROM tmdb_searches) + (SELECT count(*) FROM tmdb_series) + (SELECT count(*) FROM tmdb_seasons)`).Scan(&n))

		return n
	}
	require.Positive(t, count())

	// 引き直さずに 180 日たつと消す。番組と録画の結びつきの ID は残す。
	_, err = f.db.Exec(t.Context(), `UPDATE system_settings SET drama_linking_enabled = false`)
	require.NoError(t, err)
	f.clk.Advance(drama.CacheRetention + time.Hour)
	result, err := f.matcher.MatchHorizon(t.Context())
	require.NoError(t, err)
	assert.True(t, result.Disabled)
	assert.Zero(t, count(), "TMDb data is not kept longer than 180 days even while disabled")
}
