// Package drama は、ドラマの番組を、TMDB の作品 (TV) とシーズン、話に結びつける (#1651、設計書 3.9)。
//
// アニメの結びつき (設計書 3.8) と同じく、nekonote のジョブが EPG の同期の後に照合し、結果を DB に持つ。
// TMDB に問い合わせるのは nekonote のジョブだけで、tenban は DB を読むだけにする。
package drama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/tmdb"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// MatchHorizon は、定期的に照合し直す、これから放送される番組の範囲 (付録 A)。
	MatchHorizon = 7 * 24 * time.Hour
	// SearchMaxAge は、TMDB の検索の結果を引き直すまでの間隔 (付録 A)。始まったばかりの作品が載るのを待つ。
	SearchMaxAge = 3 * 24 * time.Hour
	// DetailMaxAge は、TMDB の作品のシーズンの一覧と、シーズンの話の一覧を引き直すまでの間隔 (付録 A)。
	// 話の放送日は、放送の直前まで足されたり直されたりする。
	DetailMaxAge = 24 * time.Hour
	// CacheRetention は、TMDB から引いた情報を持つ期間 (付録 A)。TMDB の利用規約は 6 か月を超えて持つことを禁じている。
	CacheRetention = 180 * 24 * time.Hour
	// linkedRefreshAge は、番組か録画に結びついた作品とシーズンを、消す前に引き直すまでの間隔。
	linkedRefreshAge = 150 * 24 * time.Hour
	// maxLinkedRefreshes は、1 回の照合し直しで引き直す、結びついた作品とシーズンの数の上限。
	maxLinkedRefreshes = 20
	// DefaultMaxRequests は、1 回の照合で TMDB に問い合わせる回数の上限 (付録 A)。超えた番組は、次の機会に回す。
	DefaultMaxRequests = 300
	// minProgramDuration は、照合する番組の長さの下限。宣伝の短い番組 (数分の見どころなど) を照合しない。
	minProgramDuration = 10 * time.Minute
	// orphanGrace は、番組がなくなった結びつきを消すまでの猶予。
	orphanGrace = time.Hour
	// broadcastDayShift は、深夜の番組の放送日 (前の日の 24 時台など) を求めるのに戻す時間。
	broadcastDayShift = 5 * time.Hour
	// maxCorroborations は、作品名が一致しない候補を、放送日で確認する数の上限。
	maxCorroborations = 3
	// minPrefixRunes は、作品名の前方一致で候補にする、短いほうの作品名の長さの下限 (文字)。
	minPrefixRunes = 3
	// recentSeason は、番組名の話数で話を選ぶとき、今のシーズンとみなす、シーズンの始まりからの長さ。
	recentSeason = 365 * 24 * time.Hour
	// upcomingSeries は、作品の放送開始が番組の放送より後でも、候補に残す長さ (TMDB の放送開始日のずれを見込む)。
	upcomingSeries = 30 * 24 * time.Hour
	// unauthorizedPause は、トークンが断られた (401) とき、同じトークンで問い合わせを控える長さ。管理者が入れ直すまで断られ続けるので、
	// 長く空ける。入れ直したトークンでは、すぐに問い合わせる。
	unauthorizedPause = time.Hour
	// continueAfter は、問い合わせの上限に達して照合しきれなかったとき、続きを照合するまで待つ長さ。
	// 連携を有効にした直後は、これから 1 週間のドラマの番組を一度に照合するので、上限ごとに区切って続ける。
	continueAfter = time.Minute
	// genreAnimation は、TMDB のジャンルのアニメーション。アニメはしょぼいカレンダーで結びつけるので、候補にしない。
	genreAnimation = 16
)

// 作品と話を決めた根拠 (program_drama_matches.series_source と episode_source)。
const (
	SeriesSourceSearch   = "search"
	SeriesSourceOverride = "override"
	EpisodeSourceAirDate = "air_date"
	EpisodeSourceTitle   = "title"
)

// Database は、照合に使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Client は、TMDB に token で問い合わせる。*tmdb.Client が満たす。
type Client interface {
	SearchTV(ctx context.Context, token, query string) ([]*tmdb.SearchResult, error)
	TV(ctx context.Context, token string, id int32) (*tmdb.Series, error)
	Season(ctx context.Context, token string, id, number int32) ([]*tmdb.Episode, error)
}

// TokenSource は、管理者が連携の画面で入れた TMDB のトークンを返す。設定していなければ空を返す。*Tokens が満たす。
type TokenSource interface {
	Token(ctx context.Context, db sqlc.DBTX) (string, error)
}

var _ Client = new(tmdb.Client)

var (
	// errStop は、問い合わせの上限に達したか、TMDB に 429 で断られたので、残りの番組を、少し空けて続きとして照合することを表す。
	errStop = errors.New("stop matching for now")
	// errUnauthorized は、トークンが断られたので、残りの番組を、次の定期の照合に回すことを表す。
	errUnauthorized = errors.New("stop matching until the token is fixed")
)

// Matcher は、ドラマの番組を、TMDB の作品と話に結びつける。
type Matcher struct {
	db          Database
	clock       clock.Clock
	client      Client
	tokens      TokenSource
	logger      *slog.Logger
	maxRequests int

	// mu は、limitedUntil、rejectedToken、rejectedUntil を守る。
	mu sync.Mutex
	// limitedUntil は、TMDB に 429 で断られたので、問い合わせを控える期限 (Retry-After の後)。
	// このプロセスの照合のジョブすべてで共有し、どの照合も、期限までは問い合わせない。
	limitedUntil time.Time
	// rejectedToken は、TMDB に 401 で断られたトークン。rejectedUntil (unauthorizedPause の後) までは、このトークンで問い合わせない。
	// 管理者がトークンを入れ直したら、新しいトークンで、すぐに問い合わせる。
	rejectedToken string
	rejectedUntil time.Time
}

// NewMatcher は Matcher を返す。tokens は、照合のたびに TMDB のトークンを読む先。
// maxRequests は、1 回の照合で TMDB に問い合わせる回数の上限 (0 以下なら DefaultMaxRequests)。
func NewMatcher(db Database, clk clock.Clock, client Client, tokens TokenSource, logger *slog.Logger, maxRequests int) *Matcher {
	if maxRequests <= 0 {
		maxRequests = DefaultMaxRequests
	}

	return &Matcher{
		db: db, clock: clk, client: client, tokens: tokens, logger: logger, maxRequests: maxRequests,
		mu: sync.Mutex{}, limitedUntil: time.Time{}, rejectedToken: "", rejectedUntil: time.Time{},
	}
}

// MatchResult は、1 回の照合の結果。
type MatchResult struct {
	// Disabled は、連携が無効か、トークンを設定していないので、何もしなかったかどうか。
	Disabled bool `json:"disabled"`
	// Programs は、照合した番組の数。Linked は、そのうち作品に結びついた番組の数。Episodes は、話まで決まった番組の数。
	Programs int `json:"programs"`
	Linked   int `json:"linked"`
	Episodes int `json:"episodes"`
	// Finalized は、放送が終わり、結びつきを確定させた番組の数。
	Finalized int `json:"finalized"`
	// Deferred は、問い合わせの上限か失敗で、次の機会に回した番組の数。
	Deferred int `json:"deferred"`
	// Requests は、TMDB に問い合わせた回数。Failed は、そのうち失敗した回数。
	Requests int `json:"requests"`
	Failed   int `json:"failed"`
	// Pruned は、古くなって消したキャッシュと結びつきの数。
	Pruned int64 `json:"pruned"`
	// Stopped は、問い合わせの上限に達したか、TMDB に 429 で断られて、照合しきれなかった番組があるかどうか。
	// ジョブは、ContinueAfter の後に、続きを照合し直す。トークンが断られた (401) ときは続けず、次の定期の照合に任せる。
	Stopped bool `json:"stopped"`
	// ContinueAfter は、続きを照合するまで待つ長さ。TMDB が 429 で示した長さか、上限に達したときは continueAfter。
	ContinueAfter time.Duration `json:"continueAfter"`
	// Paused は、前に TMDB に断られて問い合わせを控えている間なので、何もしなかったかどうか。
	Paused bool `json:"paused"`
}

// target は、照合する番組。
type target struct {
	serviceID     int64
	eventID       int64
	startAt       time.Time
	endAt         time.Time
	endUndecided  bool
	name          string
	seriesEpisode *int32
	seriesRepeat  bool
}

// Link は、番組に結びついた TMDB の作品と話。作品に結びつかなければ SeriesID が nil。
type Link struct {
	// TitleKey は、番組名から読んだ作品名のキー。管理者の上書きを取得するのに使う。
	TitleKey string
	SeriesID *int32
	// SeriesSource は、作品を決めた根拠 (SeriesSourceSearch か SeriesSourceOverride)。
	SeriesSource *string
	// SeasonNumber と EpisodeNumber は、話を決められたときだけ持つ。
	SeasonNumber  *int32
	EpisodeNumber *int32
	// EpisodeSource は、話を決めた根拠 (EpisodeSourceAirDate か EpisodeSourceTitle)。
	EpisodeSource *string
}

// run は、1 回の照合の間に持つ状態。TMDB から取得したものをキャッシュし、同じものを問い合わせ直さない。
type run struct {
	// token は、この照合で TMDB に問い合わせるトークン。
	token     string
	result    *MatchResult
	now       time.Time
	overrides map[string]*int32
	searches  map[string][]*tmdb.SearchResult
	series    map[int32]*tmdb.Series
	seasons   map[seasonKey][]*tmdb.Episode
}

type seasonKey struct {
	seriesID int32
	number   int32
}

// MatchChanges は、EPG の同期が「EPG が変わった」と通知した (設計書 3.3) サービスと時間帯の番組を照合する。
// 前に読んだ変更の記録 (epg_changes) のバージョンを保存し、その後の記録を読む。初めて動いたときと、無効の間は、今のバージョンを保存するだけにする。
// 問い合わせの上限に達したら、バージョンを進めない。照合しきれなかった番組は、次の機会に読み直す。
func (m *Matcher) MatchChanges(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	q := sqlc.New(m.db)
	current, err := q.GetEPGChangeVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the EPG version: %w", err)
	}
	token, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	cursor, err := q.GetDramaMatchCursor(ctx)
	if token == "" || errors.Is(err, pgx.ErrNoRows) {
		result.Disabled = token == ""

		return result, m.saveCursor(ctx, current)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the drama match cursor: %w", err)
	}
	// 断られて控えている間は、バージョンを進めずに、控え終えてから読む。
	if m.paused(token) {
		result.Paused = true

		return result, nil
	}
	changes, err := q.ListEPGChangesSince(ctx, cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to read the EPG changes: %w", err)
	}
	if len(changes) == 0 {
		return result, nil
	}

	now := m.clock.Now()
	lower, upper := now.Add(-epg.DefaultArchiveGrace), now.Add(MatchHorizon)
	spans := map[int64][2]time.Time{}
	version := cursor
	for _, change := range changes {
		version = max(version, change.Version)
		start, end := change.StartAt, change.EndAt
		if span, ok := spans[change.ServiceID]; ok {
			start, end = minTime(start, span[0]), maxTime(end, span[1])
		}
		spans[change.ServiceID] = [2]time.Time{start, end}
	}
	targets := []*target{}
	for _, serviceID := range slices.Sorted(maps.Keys(spans)) {
		span := spans[serviceID]
		from, to := maxTime(span[0], lower), minTime(span[1], upper)
		if !from.Before(to) {
			continue
		}
		found, err := m.programs(ctx, []int64{serviceID}, from, to)
		if err != nil {
			return nil, err
		}
		targets = append(targets, found...)
	}
	stopped, err := m.match(ctx, token, targets, false, result)
	if err != nil {
		return nil, err
	}
	if stopped {
		return result, nil
	}

	return result, m.saveCursor(ctx, version)
}

// MatchHorizon は、放送中とこれから 1 週間 (MatchHorizon) に放送される番組を、照合し直す。
// 古くなったキャッシュと、番組がなくなった結びつきも消し、結びついた作品の名前を消す前に引き直す。
func (m *Matcher) MatchHorizon(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	token, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	now := m.clock.Now()
	// 無効の間も、TMDB から引いた情報は 6 か月で消す (TMDB の利用規約)。
	pruned, err := m.prune(ctx, now)
	if err != nil {
		return nil, err
	}
	result.Pruned = pruned
	if token == "" {
		result.Disabled = true

		return result, nil
	}
	if m.paused(token) {
		result.Paused = true

		return result, nil
	}
	targets, err := m.programs(ctx, nil, now, now.Add(MatchHorizon))
	if err != nil {
		return nil, err
	}
	stopped, err := m.match(ctx, token, targets, false, result)
	if err != nil {
		return nil, err
	}
	if stopped {
		return result, nil
	}
	if err := m.refreshLinked(ctx, token, result, now); err != nil {
		return nil, err
	}

	return result, nil
}

// Finalize は、放送が終わり、まだアーカイブへ移していない番組を照合し直し、結びつきを確定させる。
// 確定させた番組は、それからは照合し直さない。TMDB に問い合わせられなかった番組は確定させず、次の機会に回す。
func (m *Matcher) Finalize(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	token, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		result.Disabled = true

		return result, nil
	}
	if m.paused(token) {
		result.Paused = true

		return result, nil
	}
	rows, err := sqlc.New(m.db).ListDramaProgramsToFinalize(ctx, m.clock.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama programs to finalize: %w", err)
	}
	targets := make([]*target, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, &target{
			serviceID: row.ServiceID, eventID: row.EventID, startAt: row.StartAt, endAt: row.EndAt, endUndecided: row.EndUndecided,
			name: row.Name, seriesEpisode: row.SeriesEpisode, seriesRepeat: row.SeriesRepeat.Valid && row.SeriesRepeat.Bool,
		})
	}
	if _, err := m.match(ctx, token, targets, true, result); err != nil {
		return nil, err
	}

	return result, nil
}

// enabled は、連携が有効なら、管理者が入れたトークンを返す。無効か、トークンを設定していなければ空を返す。
// トークンは tenban の画面で変わるので、照合のたびに読む。
func (m *Matcher) enabled(ctx context.Context) (string, error) {
	system, err := settings.Load(ctx, m.db)
	if err != nil {
		return "", err
	}
	if !system.DramaLinkingEnabled {
		return "", nil
	}

	return m.tokens.Token(ctx, m.db)
}

// programs は、サービス serviceIDs (nil ならすべて) の、from から to までと放送時間が重なる、確定させていないドラマの番組を返す。
func (m *Matcher) programs(ctx context.Context, serviceIDs []int64, from, to time.Time) ([]*target, error) {
	rows, err := sqlc.New(m.db).ListDramaPrograms(ctx, sqlc.ListDramaProgramsParams{ServiceIds: serviceIDs, RangeStart: from, RangeEnd: to})
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama programs: %w", err)
	}
	targets := make([]*target, 0, len(rows))
	for _, row := range rows {
		if row.FinalizedAt != nil {
			continue
		}
		targets = append(targets, &target{
			serviceID: row.ServiceID, eventID: row.EventID, startAt: row.StartAt, endAt: row.EndAt, endUndecided: row.EndUndecided,
			name: row.Name, seriesEpisode: row.SeriesEpisode, seriesRepeat: row.SeriesRepeat.Valid && row.SeriesRepeat.Bool,
		})
	}

	return targets, nil
}

// match は、targets を照合し、結びつきを書く。finalize なら、照合した番組の結びつきを確定させる。
// 問い合わせの上限に達したか、TMDB に断られたら、残りの番組を書かずに true を返す。
func (m *Matcher) match(ctx context.Context, token string, targets []*target, finalize bool, result *MatchResult) (bool, error) {
	if len(targets) == 0 {
		return false, nil
	}
	overrides, err := m.overrides(ctx)
	if err != nil {
		return false, err
	}
	r := &run{
		token: token, result: result, now: m.clock.Now(), overrides: overrides,
		searches: map[string][]*tmdb.SearchResult{}, series: map[int32]*tmdb.Series{}, seasons: map[seasonKey][]*tmdb.Episode{},
	}
	for i, t := range targets {
		link, err := m.resolve(ctx, r, t)
		if errors.Is(err, errStop) || errors.Is(err, errUnauthorized) {
			result.Deferred += len(targets) - i
			if errors.Is(err, errStop) {
				result.Stopped = true
				result.ContinueAfter = max(result.ContinueAfter, continueAfter)
			}

			return true, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			// 1 つの番組の問い合わせに失敗しても、ほかの番組は照合する。前の結びつきは残し、確定もさせない。
			result.Deferred++
			m.logger.WarnContext(ctx, "failed to match a drama program with TMDB",
				slog.Int64("service_id", t.serviceID), slog.Time("start_at", t.startAt), slog.Any("error", err))

			continue
		}
		if err := m.write(ctx, t, link, finalize, r.now); err != nil {
			return false, err
		}
		result.Programs++
		if link.SeriesID != nil {
			result.Linked++
		}
		if link.EpisodeNumber != nil {
			result.Episodes++
		}
		if finalize {
			result.Finalized++
		}
	}

	return false, nil
}

// overrides は、管理者の上書きを、作品名のキーごとに返す。値が nil なら、その作品名の番組をどの作品にも結びつけない。
func (m *Matcher) overrides(ctx context.Context) (map[string]*int32, error) {
	rows, err := sqlc.New(m.db).ListDramaTitleOverrides(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the drama title overrides: %w", err)
	}
	overrides := make(map[string]*int32, len(rows))
	for _, row := range rows {
		overrides[row.TitleKey] = row.TmdbSeriesID
	}

	return overrides, nil
}

// resolve は、番組 t に結びつける作品と話を決める。
func (m *Matcher) resolve(ctx context.Context, r *run, t *target) (*Link, error) {
	title := WorkTitle(t.name)
	link := &Link{TitleKey: TitleKey(title), SeriesID: nil, SeriesSource: nil, SeasonNumber: nil, EpisodeNumber: nil, EpisodeSource: nil}
	if link.TitleKey == "" || Excluded(t.name) || (!t.endUndecided && t.endAt.Sub(t.startAt) < minProgramDuration) {
		return link, nil
	}
	dates := broadcastDates(t.startAt)
	if override, ok := r.overrides[link.TitleKey]; ok {
		if override == nil {
			return link, nil
		}
		link.SeriesID, link.SeriesSource = new(*override), new(SeriesSourceOverride)
	} else {
		seriesID, err := m.findSeries(ctx, r, title, dates)
		if err != nil {
			return nil, err
		}
		if seriesID == 0 {
			return link, nil
		}
		link.SeriesID, link.SeriesSource = new(seriesID), new(SeriesSourceSearch)
	}
	season, episode, source, err := m.findEpisode(ctx, r, *link.SeriesID, t, dates)
	if err != nil {
		return nil, err
	}
	if episode > 0 {
		link.SeasonNumber, link.EpisodeNumber, link.EpisodeSource = new(season), new(episode), new(source)
	}

	return link, nil
}

// broadcastDates は、番組の放送日の候補を返す。日本時間の暦の日と、深夜の番組なら前の日 (放送日の数え方) の順。
// TMDB の話の放送日は、日本の作品では暦の日で書かれていることが多いが、放送日の数え方で書かれた作品もある。
func broadcastDates(startAt time.Time) []string {
	dates := []string{startAt.In(tokyo()).Format(time.DateOnly)}
	if shifted := startAt.Add(-broadcastDayShift).In(tokyo()).Format(time.DateOnly); shifted != dates[0] {
		dates = append(dates, shifted)
	}

	return dates
}

// tokyo は、日本時間を返す。TMDB の日本の作品の放送日は、日本時間の日で書かれる。日本は 1951 年から夏時間を使っていない。
func tokyo() *time.Location {
	return time.FixedZone("JST", 9*60*60)
}

// findSeries は、作品名 title で TMDB を探し、番組の作品を決める。決められなければ 0 を返す。
// 誤った作品に結びつけるより、結びつけないほうを選ぶ。作品名 (NFKC で正規化し、空白と記号を除いたもの) が一致する候補だけを、
// そのまま結びつける。一致する候補が複数あれば、放送の時期で選ぶ。一致する候補がなく、作品名の前方が一致する候補だけがあれば、
// その作品に番組の放送日の話があるかで確認し、1 つだけに絞れたら結びつける。
func (m *Matcher) findSeries(ctx context.Context, r *run, title string, dates []string) (int32, error) {
	queries := SearchQueries(title)
	partial := []*tmdb.SearchResult{}
	for _, query := range queries {
		results, err := m.search(ctx, r, query)
		if err != nil {
			return 0, err
		}
		key := TitleKey(query)
		exact := []*tmdb.SearchResult{}
		for _, result := range results {
			if slices.Contains(result.GenreIDs, genreAnimation) {
				continue
			}
			nameKey, originalKey := TitleKey(result.Name), TitleKey(result.OriginalName)
			switch {
			case nameKey == key || originalKey == key:
				exact = append(exact, result)
			case prefixOf(key, nameKey) || prefixOf(key, originalKey):
				if !slices.ContainsFunc(partial, func(p *tmdb.SearchResult) bool { return p.ID == result.ID }) {
					partial = append(partial, result)
				}
			}
		}
		if len(exact) > 0 {
			return pickByDate(exact, dates[0]), nil
		}
	}
	partial, err := m.headCandidates(ctx, r, title, queries, partial)
	if err != nil {
		return 0, err
	}
	corroborated := int32(0)
	for _, candidate := range partial[:min(len(partial), maxCorroborations)] {
		if candidate.FirstAirDate != "" && candidate.FirstAirDate > shiftDate(dates[0], upcomingSeries) {
			continue
		}
		found, err := m.airedOn(ctx, r, candidate.ID, dates)
		if err != nil {
			return 0, err
		}
		if !found {
			continue
		}
		if corroborated != 0 {
			// 2 つ以上の候補に放送日の話があれば、どれか決められない。
			return 0, nil
		}
		corroborated = candidate.ID
	}

	return corroborated, nil
}

// headCandidates は、空白の後ろにサブタイトルが付いた番組名を、最初の空白より前 (HeadQuery) でも探し、作品名の先頭が一致する候補を partial に足す。
// 頭と同じ名前の作品も、別の作品かもしれないので、作品名が一致する候補としては扱わず、放送日の話で確認させる。
func (m *Matcher) headCandidates(
	ctx context.Context, r *run, title string, queries []string, partial []*tmdb.SearchResult,
) ([]*tmdb.SearchResult, error) {
	head := HeadQuery(title)
	if head == "" || slices.Contains(queries, head) {
		return partial, nil
	}
	results, err := m.search(ctx, r, head)
	if err != nil {
		return nil, err
	}
	key := TitleKey(title)
	for _, result := range results {
		if slices.Contains(result.GenreIDs, genreAnimation) {
			continue
		}
		if (prefixOf(key, TitleKey(result.Name)) || prefixOf(key, TitleKey(result.OriginalName))) &&
			!slices.ContainsFunc(partial, func(p *tmdb.SearchResult) bool { return p.ID == result.ID }) {
			partial = append(partial, result)
		}
	}

	return partial, nil
}

// prefixOf は、短いほうの作品名が長いほうの先頭と一致するかを返す。短いほうが短すぎれば false。
func prefixOf(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	short, long := a, b
	if utf8.RuneCountInString(short) > utf8.RuneCountInString(long) {
		short, long = long, short
	}

	return utf8.RuneCountInString(short) >= minPrefixRunes && strings.HasPrefix(long, short)
}

// pickByDate は、作品名が一致する候補から、番組の放送日 date より前に始まった (始まったばかりのものを含む) 作品のうち、
// 最も新しく始まった作品を選ぶ。同じ名前のリメイクがあっても、放送中の作品を選ぶため。どれも当たらなければ、TMDB の並びの先頭を選ぶ。
func pickByDate(candidates []*tmdb.SearchResult, date string) int32 {
	limit := shiftDate(date, upcomingSeries)
	best := (*tmdb.SearchResult)(nil)
	for _, candidate := range candidates {
		if candidate.FirstAirDate == "" || candidate.FirstAirDate > limit {
			continue
		}
		if best == nil || candidate.FirstAirDate > best.FirstAirDate {
			best = candidate
		}
	}
	if best == nil {
		best = candidates[0]
	}

	return best.ID
}

// shiftDate は、日付 date (YYYY-MM-DD) を d だけずらした日付を返す。
func shiftDate(date string, d time.Duration) string {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}

	return parsed.Add(d).Format(time.DateOnly)
}

// findEpisode は、作品 seriesID の、番組 t の話を決める。決められなければ episode が 0。
//   - 再放送でなければ、TMDB の話の放送日と番組の放送日が一致する話を選ぶ。シーズンの番号は番組名の数字と合わないことがある
//     (晩酌の流儀5 は TMDB ではシーズン 7) ので、番組名からは読まない。
//   - 放送日で選べなければ (遅れて放送する局や、TMDB にまだ放送日のない話)、番組名の話数で、今放送しているシーズンの話を選ぶ。
//   - 再放送は、放送日では選べないので、番組名の話数で選ぶ。シーズンがいくつもある作品は、どのシーズンか決められないので、作品だけに結びつける。
//     再放送のマークがなくても、今放送しているシーズンがなければ (昔の作品や海外ドラマ)、再放送と同じく選ぶ。
func (m *Matcher) findEpisode(ctx context.Context, r *run, seriesID int32, t *target, dates []string) (int32, int32, string, error) {
	series, err := m.seriesDetail(ctx, r, seriesID)
	if err != nil {
		return 0, 0, "", err
	}
	number := EpisodeNumber(t.name)
	if number == 0 && t.seriesEpisode != nil {
		number = *t.seriesEpisode
	}
	regular := []*tmdb.Season{}
	for _, season := range series.Seasons {
		if season.SeasonNumber > 0 {
			regular = append(regular, season)
		}
	}
	slices.SortFunc(regular, func(a, b *tmdb.Season) int { return int(a.SeasonNumber - b.SeasonNumber) })

	if Rerun(t.name, t.seriesRepeat) {
		if number > 0 && len(regular) == 1 {
			return m.titleEpisode(ctx, r, seriesID, regular[0].SeasonNumber, number, "")
		}

		return 0, 0, "", nil
	}
	season, episode, err := m.airDateEpisode(ctx, r, series, regular, number, dates)
	if err != nil || episode > 0 {
		return season, episode, EpisodeSourceAirDate, err
	}
	if number == 0 {
		return 0, 0, "", nil
	}
	// 今放送しているシーズン (番組の放送日より前に始まった、最も新しいシーズン) で、番組名の話数を探す。
	current := (*tmdb.Season)(nil)
	for _, s := range regular {
		if s.AirDate != "" && s.AirDate <= dates[0] {
			current = s
		}
	}
	if current == nil || current.AirDate < shiftDate(dates[0], -recentSeason) {
		// 今放送しているシーズンがなければ、再放送のマークのない再放送 (昔の作品や海外ドラマ) とみなし、再放送と同じく選ぶ。
		if len(regular) == 1 {
			return m.titleEpisode(ctx, r, seriesID, regular[0].SeasonNumber, number, "")
		}

		return 0, 0, "", nil
	}

	return m.titleEpisode(ctx, r, seriesID, current.SeasonNumber, number, dates[0])
}

// airDateEpisode は、放送日 dates の話を探す。番組の放送日より前に始まった、新しいほうの 2 つのシーズンと、特別編 (シーズン 0) から探す。
// 同じ日に複数の話があれば (2 話連続の放送など)、番組名の話数 number の話を、なければ最初の話を選ぶ。
func (m *Matcher) airDateEpisode(
	ctx context.Context, r *run, series *tmdb.Series, regular []*tmdb.Season, number int32, dates []string,
) (int32, int32, error) {
	for _, date := range dates {
		for _, seasonNumber := range seasonsBefore(series, regular, date) {
			episodes, err := m.seasonEpisodes(ctx, r, series.ID, seasonNumber)
			if err != nil {
				return 0, 0, err
			}
			if episode := episodeOn(episodes, date, number); episode > 0 {
				return seasonNumber, episode, nil
			}
		}
	}

	return 0, 0, nil
}

// seasonsBefore は、放送日 date の話を探すシーズンを返す。date より前に始まった、新しいほうの 2 つのシーズンと、特別編 (シーズン 0)。
func seasonsBefore(series *tmdb.Series, regular []*tmdb.Season, date string) []int32 {
	candidates := []int32{}
	for i := len(regular) - 1; i >= 0 && len(candidates) < 2; i-- {
		if regular[i].AirDate != "" && regular[i].AirDate <= date {
			candidates = append(candidates, regular[i].SeasonNumber)
		}
	}
	for _, s := range series.Seasons {
		if s.SeasonNumber == 0 && s.AirDate != "" && s.AirDate <= date {
			candidates = append(candidates, 0)
		}
	}

	return candidates
}

// episodeOn は、episodes のうち、放送日が date の話の番号を返す。同じ日に複数の話があれば、番組名の話数 number の話を、
// なければ最初の話を選ぶ。なければ 0 を返す。
// 番組名に話数があり (number > 0)、その日の話のどれとも食い違えば、0 を返す。1 週遅れて放送する局では、その日に次の話が当たるので、
// 誤った話に結びつけるより、番組名の話数で選び直す。
func episodeOn(episodes []*tmdb.Episode, date string, number int32) int32 {
	pick := int32(0)
	for _, e := range episodes {
		if e.AirDate != date {
			continue
		}
		if e.EpisodeNumber == number {
			return number
		}
		if pick == 0 {
			pick = e.EpisodeNumber
		}
	}
	if number > 0 {
		return 0
	}

	return pick
}

// titleEpisode は、作品 seriesID のシーズン seasonNumber に、番組名の話数 number の話があれば返す。
// notAfter が空でなければ、その日より後に放送する話は選ばない (話数の書き誤りや、別のシーズンの話を選ばないため)。
func (m *Matcher) titleEpisode(
	ctx context.Context, r *run, seriesID, seasonNumber, number int32, notAfter string,
) (int32, int32, string, error) {
	episodes, err := m.seasonEpisodes(ctx, r, seriesID, seasonNumber)
	if err != nil {
		return 0, 0, "", err
	}
	for _, e := range episodes {
		if e.EpisodeNumber != number {
			continue
		}
		if notAfter != "" && e.AirDate != "" && e.AirDate > notAfter {
			return 0, 0, "", nil
		}

		return seasonNumber, number, EpisodeSourceTitle, nil
	}

	return 0, 0, "", nil
}

// airedOn は、作品 seriesID に、放送日 dates の話があるかを返す。
func (m *Matcher) airedOn(ctx context.Context, r *run, seriesID int32, dates []string) (bool, error) {
	series, err := m.seriesDetail(ctx, r, seriesID)
	if err != nil {
		return false, err
	}
	regular := []*tmdb.Season{}
	for _, season := range series.Seasons {
		if season.SeasonNumber > 0 {
			regular = append(regular, season)
		}
	}
	slices.SortFunc(regular, func(a, b *tmdb.Season) int { return int(a.SeasonNumber - b.SeasonNumber) })
	_, episode, err := m.airDateEpisode(ctx, r, series, regular, 0, dates)

	return episode > 0, err
}

// limit は、TMDB に 429 で断られたので、どのトークンでも、問い合わせを d だけ控える。
func (m *Matcher) limit(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.limitedUntil = maxTime(m.limitedUntil, m.clock.Now().Add(d))
}

// reject は、TMDB に 401 で断られたので、トークン token での問い合わせを unauthorizedPause だけ控える。
func (m *Matcher) reject(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rejectedToken, m.rejectedUntil = token, m.clock.Now().Add(unauthorizedPause)
}

// paused は、トークン token での TMDB への問い合わせを控えている間かどうかを返す。
func (m *Matcher) paused(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()

	return now.Before(m.limitedUntil) || (token == m.rejectedToken && now.Before(m.rejectedUntil))
}

// request は、TMDB への問い合わせを 1 回数え、上限に達していれば errStop を返す。
func (m *Matcher) request(r *run) error {
	if r.result.Requests >= m.maxRequests {
		return errStop
	}
	r.result.Requests++

	return nil
}

// failed は、問い合わせの失敗を数える。断られた (429) なら errStop を、トークンが使えなければ errUnauthorized を返し、
// 残りの番組も問い合わせないようにする。
func (m *Matcher) failed(ctx context.Context, r *run, err error) error {
	r.result.Failed++
	if limited, ok := errors.AsType[*tmdb.RateLimitedError](err); ok {
		m.logger.WarnContext(ctx, "stopped querying TMDB", slog.Any("error", err))
		r.result.ContinueAfter = max(r.result.ContinueAfter, limited.RetryAfter)
		m.limit(limited.RetryAfter)

		return errStop
	}
	if errors.Is(err, tmdb.ErrUnauthorized) {
		m.logger.WarnContext(ctx, "TMDB rejected the token; enter a valid token in the integrations settings", slog.Any("error", err))
		m.reject(r.token)

		return errUnauthorized
	}

	return err
}

// search は、作品名 query の検索の結果を返す。SearchMaxAge より新しいキャッシュがあれば、問い合わせない。
func (m *Matcher) search(ctx context.Context, r *run, query string) ([]*tmdb.SearchResult, error) {
	if cached, ok := r.searches[query]; ok {
		return cached, nil
	}
	q := sqlc.New(m.db)
	row, err := q.GetTMDBSearch(ctx, query)
	if err == nil && row.FetchedAt.After(r.now.Add(-SearchMaxAge)) {
		results := []*tmdb.SearchResult{}
		if err := json.Unmarshal(row.Results, &results); err != nil {
			return nil, fmt.Errorf("failed to decode the cached TMDB search: %w", err)
		}
		r.searches[query] = results

		return results, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to read the cached TMDB search: %w", err)
	}
	if err := m.request(r); err != nil {
		return nil, err
	}
	results, err := m.client.SearchTV(ctx, r.token, query)
	if err != nil {
		return nil, m.failed(ctx, r, err)
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the TMDB search: %w", err)
	}
	if err := q.UpsertTMDBSearch(ctx, sqlc.UpsertTMDBSearchParams{Query: query, Results: encoded, Now: m.clock.Now()}); err != nil {
		return nil, fmt.Errorf("failed to save the TMDB search: %w", err)
	}
	r.searches[query] = results

	return results, nil
}

// seriesDetail は、作品 id のシーズンの一覧を返す。DetailMaxAge より新しいキャッシュがあれば、問い合わせない。
// TMDB に作品がなければ (管理者の上書きが、消えた作品を指しているときなど)、シーズンのない作品を返す。
func (m *Matcher) seriesDetail(ctx context.Context, r *run, id int32) (*tmdb.Series, error) {
	if cached, ok := r.series[id]; ok {
		return cached, nil
	}
	row, err := sqlc.New(m.db).GetTMDBSeries(ctx, id)
	if err == nil && row.FetchedAt.After(r.now.Add(-DetailMaxAge)) {
		series, err := seriesOfRow(&row)
		if err != nil {
			return nil, err
		}
		r.series[id] = series

		return series, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to read the cached TMDB series: %w", err)
	}
	series, err := m.fetchSeries(ctx, r, id)
	if err != nil {
		return nil, err
	}
	r.series[id] = series

	return series, nil
}

// fetchSeries は、作品 id を TMDB から引き、キャッシュを入れ替える。TMDB に作品がなければ、シーズンのない作品を返す。
func (m *Matcher) fetchSeries(ctx context.Context, r *run, id int32) (*tmdb.Series, error) {
	if err := m.request(r); err != nil {
		return nil, err
	}
	series, err := m.client.TV(ctx, r.token, id)
	if errors.Is(err, tmdb.ErrNotFound) {
		// 作品がないことは、誤りではなく結果として、シーズンのない作品として返す。キャッシュはしない。
		return &tmdb.Series{ID: id, Name: "", OriginalName: "", FirstAirDate: "", Seasons: []*tmdb.Season{}}, nil
	}
	if err != nil {
		return nil, m.failed(ctx, r, err)
	}
	seasons, err := json.Marshal(series.Seasons)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the TMDB seasons: %w", err)
	}
	if err := sqlc.New(m.db).UpsertTMDBSeries(ctx, sqlc.UpsertTMDBSeriesParams{
		ID: series.ID, Name: series.Name, OriginalName: series.OriginalName, FirstAirDate: dateOf(series.FirstAirDate), Seasons: seasons, Now: m.clock.Now(),
	}); err != nil {
		return nil, fmt.Errorf("failed to save the TMDB series: %w", err)
	}

	return series, nil
}

// seasonEpisodes は、作品 seriesID のシーズン number の話の一覧を返す。DetailMaxAge より新しいキャッシュがあれば、問い合わせない。
// TMDB にシーズンがなければ、空の一覧を返す。
func (m *Matcher) seasonEpisodes(ctx context.Context, r *run, seriesID, number int32) ([]*tmdb.Episode, error) {
	key := seasonKey{seriesID: seriesID, number: number}
	if cached, ok := r.seasons[key]; ok {
		return cached, nil
	}
	row, err := sqlc.New(m.db).GetTMDBSeason(ctx, sqlc.GetTMDBSeasonParams{SeriesID: seriesID, SeasonNumber: number})
	if err == nil && row.FetchedAt.After(r.now.Add(-DetailMaxAge)) {
		episodes := []*tmdb.Episode{}
		if err := json.Unmarshal(row.Episodes, &episodes); err != nil {
			return nil, fmt.Errorf("failed to decode the cached TMDB season: %w", err)
		}
		r.seasons[key] = episodes

		return episodes, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to read the cached TMDB season: %w", err)
	}
	episodes, err := m.fetchSeason(ctx, r, seriesID, number)
	if err != nil {
		return nil, err
	}
	r.seasons[key] = episodes

	return episodes, nil
}

// fetchSeason は、作品 seriesID のシーズン number を TMDB から引き、キャッシュを入れ替える。
func (m *Matcher) fetchSeason(ctx context.Context, r *run, seriesID, number int32) ([]*tmdb.Episode, error) {
	if err := m.request(r); err != nil {
		return nil, err
	}
	episodes, err := m.client.Season(ctx, r.token, seriesID, number)
	if errors.Is(err, tmdb.ErrNotFound) {
		episodes, err = []*tmdb.Episode{}, nil
	}
	if err != nil {
		return nil, m.failed(ctx, r, err)
	}
	encoded, err := json.Marshal(episodes)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the TMDB season: %w", err)
	}
	if err := sqlc.New(m.db).UpsertTMDBSeason(ctx, sqlc.UpsertTMDBSeasonParams{
		SeriesID: seriesID, SeasonNumber: number, Episodes: encoded, Now: m.clock.Now(),
	}); err != nil {
		return nil, fmt.Errorf("failed to save the TMDB season: %w", err)
	}

	return episodes, nil
}

// write は、番組 t の結びつきを link で入れ替える。finalize なら、結びつきを確定させる。
// 放送時間が変わった番組 (設計書 4.6) の、前の放送時間の結びつきは消す。照合し直した結びつきは、この番組の録画のコピーにも反映する。
func (m *Matcher) write(ctx context.Context, t *target, link *Link, finalize bool, now time.Time) error {
	return pgx.BeginFunc(ctx, m.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.DeleteMovedProgramDramaMatches(ctx, sqlc.DeleteMovedProgramDramaMatchesParams{
			ServiceID: t.serviceID, EventID: t.eventID, StartAt: t.startAt,
		}); err != nil {
			return fmt.Errorf("failed to delete the moved drama match: %w", err)
		}
		var finalizedAt *time.Time
		if finalize {
			finalizedAt = &now
		}
		if err := q.UpsertProgramDramaMatch(ctx, sqlc.UpsertProgramDramaMatchParams{
			ServiceID: t.serviceID, StartAt: t.startAt, EventID: t.eventID, TitleKey: link.TitleKey,
			TmdbSeriesID: link.SeriesID, SeasonNumber: link.SeasonNumber, EpisodeNumber: link.EpisodeNumber,
			SeriesSource: link.SeriesSource, EpisodeSource: link.EpisodeSource, Now: now, FinalizedAt: finalizedAt,
		}); err != nil {
			return fmt.Errorf("failed to save the drama match: %w", err)
		}
		recordings, err := q.ListProgramRecordingIDs(ctx, sqlc.ListProgramRecordingIDsParams{ServiceID: t.serviceID, StartAt: t.startAt})
		if err != nil {
			return fmt.Errorf("failed to list the recordings of the program: %w", err)
		}
		for _, recordingID := range recordings {
			if err := CopyToRecording(ctx, tx, recordingID, t.serviceID, t.startAt, now); err != nil {
				return err
			}
		}

		return nil
	})
}

// prune は、TMDB から引いて 6 か月がたつ前の情報と、番組表からも過去の番組からもなくなった番組の結びつきを消す。
func (m *Matcher) prune(ctx context.Context, now time.Time) (int64, error) {
	q := sqlc.New(m.db)
	before := now.Add(-CacheRetention)
	searches, err := q.PruneTMDBSearches(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("failed to prune the TMDB searches: %w", err)
	}
	series, err := q.PruneTMDBSeries(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("failed to prune the TMDB series: %w", err)
	}
	seasons, err := q.PruneTMDBSeasons(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("failed to prune the TMDB seasons: %w", err)
	}
	matches, err := q.PruneProgramDramaMatches(ctx, now.Add(-orphanGrace))
	if err != nil {
		return 0, fmt.Errorf("failed to prune the drama matches: %w", err)
	}

	return searches + series + seasons + matches, nil
}

// refreshLinked は、番組か録画に結びついた作品とシーズンのうち、引いてから linkedRefreshAge がたったものを引き直す。
// 6 か月で消す前に引き直し、何か月も前の録画にも、作品と話の名前を出し続ける。
func (m *Matcher) refreshLinked(ctx context.Context, token string, result *MatchResult, now time.Time) error {
	q := sqlc.New(m.db)
	r := &run{
		token: token, result: result, now: now, overrides: map[string]*int32{},
		searches: map[string][]*tmdb.SearchResult{}, series: map[int32]*tmdb.Series{}, seasons: map[seasonKey][]*tmdb.Episode{},
	}
	before := now.Add(-linkedRefreshAge)
	ids, err := q.ListStaleLinkedTMDBSeries(ctx, sqlc.ListStaleLinkedTMDBSeriesParams{Before: before, MaxCount: maxLinkedRefreshes})
	if err != nil {
		return fmt.Errorf("failed to list the stale TMDB series: %w", err)
	}
	for _, id := range ids {
		if _, err := m.fetchSeries(ctx, r, id); err != nil {
			return m.stopRefresh(ctx, err)
		}
	}
	seasons, err := q.ListStaleLinkedTMDBSeasons(ctx, sqlc.ListStaleLinkedTMDBSeasonsParams{Before: before, MaxCount: maxLinkedRefreshes})
	if err != nil {
		return fmt.Errorf("failed to list the stale TMDB seasons: %w", err)
	}
	for _, season := range seasons {
		if _, err := m.fetchSeason(ctx, r, season.SeriesID, season.SeasonNumber); err != nil {
			return m.stopRefresh(ctx, err)
		}
	}

	return nil
}

// stopRefresh は、再取得の失敗を、ジョブの失敗にせずにログに残す。次の照合し直しで続きを取得する。
func (m *Matcher) stopRefresh(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !errors.Is(err, errStop) && !errors.Is(err, errUnauthorized) {
		m.logger.WarnContext(ctx, "failed to refresh a linked TMDB series", slog.Any("error", err))
	}

	return nil
}

// saveCursor は、読み終えた EPG の変更の記録のバージョンを保存する。
func (m *Matcher) saveCursor(ctx context.Context, version int64) error {
	if err := sqlc.New(m.db).SaveDramaMatchCursor(ctx, sqlc.SaveDramaMatchCursorParams{EpgVersion: version, Now: m.clock.Now()}); err != nil {
		return fmt.Errorf("failed to save the drama match cursor: %w", err)
	}

	return nil
}

// CopyToRecording は、tx の中で、番組 (serviceID, startAt) の結びつきを、録画 recordingID にコピーする (設計書 3.9)。
// アニメの結びつきと同じく、録画を始めたときにコピーする。番組をまだ照合していなければ、何もしない。
func CopyToRecording(ctx context.Context, tx sqlc.DBTX, recordingID uuid.UUID, serviceID int64, startAt, now time.Time) error {
	if _, err := sqlc.New(tx).CopyProgramDramaMatchToRecording(ctx, sqlc.CopyProgramDramaMatchToRecordingParams{
		RecordingID: recordingID, Now: now, ServiceID: serviceID, StartAt: startAt,
	}); err != nil {
		return fmt.Errorf("failed to copy the drama match to the recording: %w", err)
	}

	return nil
}

// seriesOfRow は、キャッシュした作品の行を tmdb.Series にする。
func seriesOfRow(row *sqlc.TmdbSeries) (*tmdb.Series, error) {
	seasons := []*tmdb.Season{}
	if err := json.Unmarshal(row.Seasons, &seasons); err != nil {
		return nil, fmt.Errorf("failed to decode the cached TMDB seasons: %w", err)
	}
	firstAirDate := ""
	if row.FirstAirDate.Valid {
		firstAirDate = row.FirstAirDate.Time.Format(time.DateOnly)
	}

	return &tmdb.Series{ID: row.ID, Name: row.Name, OriginalName: row.OriginalName, FirstAirDate: firstAirDate, Seasons: seasons}, nil
}

// dateOf は、TMDB の日付 (YYYY-MM-DD) を DB の日付にする。空か読めなければ NULL にする。
func dateOf(value string) pgtype.Date {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return pgtype.Date{Time: time.Time{}, InfinityModifier: pgtype.Finite, Valid: false}
	}

	return pgtype.Date{Time: parsed, InfinityModifier: pgtype.Finite, Valid: true}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}
