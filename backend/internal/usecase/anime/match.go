package anime

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/epg"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// MatchHorizon は、定期的に照合し直す、これから放送される番組の範囲 (付録 A)。
	MatchHorizon = 7 * 24 * time.Hour
	// CacheMaxAge は、しょぼいカレンダーの放送時間データを引き直すまでの間隔 (付録 A)。
	// しょぼいカレンダーの登録は放送の直前まで変わる (延期、話数の訂正) ので、照合し直すたびに、これより古いものは引き直す。
	CacheMaxAge = 6 * time.Hour
	// SourceSyobocal は、しょぼいカレンダーの放送時間データで結びつけた話の出どころ。
	SourceSyobocal = "syobocal"
	// cacheRetention は、放送が終わったしょぼいカレンダーの放送時間データを残す期間。アーカイブへ移す前に照合し直すのに足りるようにする。
	cacheRetention = epg.DefaultArchiveGrace + 24*time.Hour
	// orphanGrace は、番組がなくなった結びつきを消すまでの猶予。書いたばかりの結びつきを、番組を書く前に消さないようにする。
	orphanGrace = time.Hour
	// day は、しょぼいカレンダーの放送時間データをキャッシュする単位。
	day = 24 * time.Hour
)

// multipleEpisodes は、複数の話をまとめて放送するときの、放送時間データのサブタイトルの書き方 ("#1～#2")。
var multipleEpisodes = regexp.MustCompile(`^#([0-9]+(?:\.[0-9]+)?)[～〜~\-]#([0-9]+(?:\.[0-9]+)?)$`)

// SyobocalClient は、しょぼいカレンダーに問い合わせる。*syobocal.Client が満たす。
type SyobocalClient interface {
	ProgLookup(ctx context.Context, channelIDs []int32, from, to time.Time) ([]*syobocal.Program, error)
}

var _ SyobocalClient = new(syobocal.Client)

// Matcher は、アニメと特撮の番組を、しょぼいカレンダーの放送時間データで、作品 (TID) と話に結びつける (設計書 3.8)。
type Matcher struct {
	db     Database
	clock  clock.Clock
	client SyobocalClient
	logger *slog.Logger
}

// NewMatcher は Matcher を返す。
func NewMatcher(db Database, clk clock.Clock, client SyobocalClient, logger *slog.Logger) *Matcher {
	return &Matcher{db: db, clock: clk, client: client, logger: logger}
}

// MatchResult は、1 回の照合の結果。
type MatchResult struct {
	// Disabled は、連携が無効なので何もしなかったかどうか。
	Disabled bool `json:"disabled"`
	// Programs は、照合した番組の数。Linked は、そのうち 1 つ以上の話に結びついた番組の数。
	Programs int `json:"programs"`
	Linked   int `json:"linked"`
	// Finalized は、放送が終わり、結びつきを確定させた番組の数。
	Finalized int `json:"finalized"`
	// Requests は、しょぼいカレンダーに問い合わせた回数。Failed は、そのうち失敗した回数。
	Requests int `json:"requests"`
	Failed   int `json:"failed"`
	// Pruned は、古くなって消したキャッシュと結びつきの数。
	Pruned int64 `json:"pruned"`
}

// target は、照合する番組。
type target struct {
	serviceID int64
	eventID   int64
	startAt   time.Time
	endAt     time.Time
	// channelID は、サービスに対応する、しょぼいカレンダーの ChID。
	channelID int32
	// fetchedAfter は、この番組を照合する放送時間データを、いつより後に引いたものにするか。
	fetchedAfter time.Time
}

// Episode は、番組に結びついた話 1 つ。
type Episode struct {
	// Source は、結びつけた出どころ (syobocal など)。
	Source string
	// TID は、しょぼいカレンダーの作品の ID。PID は、放送時間データの ID。
	TID *int32
	PID *int32
	// CountStart と CountEnd は、話数の範囲。1 話だけなら同じ値。話数のない放送 (特番など) では nil。
	CountStart *float64
	CountEnd   *float64
	// Subtitle は、その話のサブタイトル。
	Subtitle *string
	// First と Last は、しょぼいカレンダーの [新] と [終] の印。
	First bool
	Last  bool
}

// MatchChanges は、EPG の同期が「EPG が変わった」と通知した (設計書 3.3) サービスと時間帯の番組を照合する。
// 前に読んだ変更の記録 (epg_changes) の版数を覚え、その後の記録を読む。
// 初めて動いたときと、無効の間は、今の版数を覚えるだけにする。それまでの番組は、定期的な照合し直しが受け持つ。
func (m *Matcher) MatchChanges(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	q := sqlc.New(m.db)
	current, err := q.GetEPGChangeVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the EPG version: %w", err)
	}
	enabled, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	cursor, err := q.GetAnimeMatchCursor(ctx)
	if !enabled || errors.Is(err, pgx.ErrNoRows) {
		result.Disabled = !enabled

		return result, m.saveCursor(ctx, current)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the anime match cursor: %w", err)
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
	channels, err := m.channels(ctx)
	if err != nil {
		return nil, err
	}
	targets := []*target{}
	for _, serviceID := range slices.Sorted(maps.Keys(spans)) {
		channelID, ok := channels[serviceID]
		if !ok {
			continue
		}
		span := spans[serviceID]
		from, to := maxTime(span[0], lower), minTime(span[1], upper)
		if !from.Before(to) {
			continue
		}
		found, err := m.programs(ctx, []int64{serviceID}, from, to, map[int64]int32{serviceID: channelID}, now.Add(-CacheMaxAge))
		if err != nil {
			return nil, err
		}
		targets = append(targets, found...)
	}
	if err := m.match(ctx, targets, false, result); err != nil {
		return nil, err
	}

	return result, m.saveCursor(ctx, version)
}

// MatchHorizon は、放送中とこれから 1 週間 (MatchHorizon) に放送される番組を、照合し直す。
// しょぼいカレンダーの放送時間データは、CacheMaxAge より古ければ引き直す。古くなったキャッシュと、番組がなくなった結びつきも消す。
func (m *Matcher) MatchHorizon(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	enabled, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		result.Disabled = true

		return result, nil
	}
	channels, err := m.channels(ctx)
	if err != nil {
		return nil, err
	}
	now := m.clock.Now()
	targets, err := m.programs(ctx, slices.Sorted(maps.Keys(channels)), now, now.Add(MatchHorizon), channels, now.Add(-CacheMaxAge))
	if err != nil {
		return nil, err
	}
	if err := m.match(ctx, targets, false, result); err != nil {
		return nil, err
	}
	pruned, err := m.prune(ctx, now)
	if err != nil {
		return nil, err
	}
	result.Pruned = pruned

	return result, nil
}

// Finalize は、放送が終わり、まだアーカイブへ移していない番組を、放送が終わった後に引いた放送時間データで照合し直し、結びつきを確定させる。
// 確定させた番組は、それからは照合し直さない。しょぼいカレンダーに問い合わせられなかった番組は確定させず、次の機会に回す。
func (m *Matcher) Finalize(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	enabled, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		result.Disabled = true

		return result, nil
	}
	channels, err := m.channels(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(m.db).ListAnimeProgramsToFinalize(ctx, sqlc.ListAnimeProgramsToFinalizeParams{
		ServiceIds: slices.Sorted(maps.Keys(channels)), Now: m.clock.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime programs to finalize: %w", err)
	}
	targets := make([]*target, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, &target{
			serviceID: row.ServiceID, eventID: row.EventID, startAt: row.StartAt, endAt: row.EndAt,
			channelID: channels[row.ServiceID], fetchedAfter: row.EndAt,
		})
	}
	if err := m.match(ctx, targets, true, result); err != nil {
		return nil, err
	}

	return result, nil
}

// enabled は、連携が有効かを返す。
func (m *Matcher) enabled(ctx context.Context) (bool, error) {
	system, err := settings.Load(ctx, m.db)
	if err != nil {
		return false, err
	}

	return system.AnimeLinkingEnabled, nil
}

// channels は、サービスごとの、しょぼいカレンダーの ChID を返す。ChID のないサービスは含めない。
func (m *Matcher) channels(ctx context.Context) (map[int64]int32, error) {
	list, err := ListChannels(ctx, m.db)
	if err != nil {
		return nil, err
	}
	channels := map[int64]int32{}
	for _, channel := range list {
		if channel.SyobocalChannelID != nil {
			channels[channel.ServiceID] = *channel.SyobocalChannelID
		}
	}

	return channels, nil
}

// programs は、サービス serviceIDs の、from から to までと放送時間が重なる、確定させていないアニメと特撮の番組を返す。
func (m *Matcher) programs(
	ctx context.Context, serviceIDs []int64, from, to time.Time, channels map[int64]int32, fetchedAfter time.Time,
) ([]*target, error) {
	if len(serviceIDs) == 0 {
		return []*target{}, nil
	}
	rows, err := sqlc.New(m.db).ListAnimePrograms(ctx, sqlc.ListAnimeProgramsParams{ServiceIds: serviceIDs, RangeStart: from, RangeEnd: to})
	if err != nil {
		return nil, fmt.Errorf("failed to list the anime programs: %w", err)
	}
	targets := make([]*target, 0, len(rows))
	for _, row := range rows {
		if row.FinalizedAt != nil {
			continue
		}
		targets = append(targets, &target{
			serviceID: row.ServiceID, eventID: row.EventID, startAt: row.StartAt, endAt: row.EndAt,
			channelID: channels[row.ServiceID], fetchedAfter: fetchedAfter,
		})
	}

	return targets, nil
}

// channelDay は、しょぼいカレンダーの放送時間データをキャッシュする単位 (ChID と、日本時間の日の始まりを UTC で表したもの)。
type channelDay struct {
	channelID int32
	day       time.Time
}

// match は、targets を照合し、結びつきを書く。finalize なら、照合した番組の結びつきを確定させる。
func (m *Matcher) match(ctx context.Context, targets []*target, finalize bool, result *MatchResult) error {
	if len(targets) == 0 {
		return nil
	}
	// 番組ごとに、その放送時間と重なる日の放送時間データを、いつより後に引いたものにするかを決める。
	required := map[channelDay]time.Time{}
	for _, t := range targets {
		for _, d := range daysOf(t.startAt, t.endAt) {
			key := channelDay{channelID: t.channelID, day: d}
			if current, ok := required[key]; !ok || t.fetchedAfter.After(current) {
				required[key] = t.fetchedAfter
			}
		}
	}
	failed, err := m.refresh(ctx, required, result)
	if err != nil {
		return err
	}

	from, to := targets[0].startAt, targets[0].endAt
	channelIDs := map[int32]bool{}
	for _, t := range targets {
		from, to = minTime(from, t.startAt), maxTime(to, t.endAt)
		channelIDs[t.channelID] = true
	}
	// StOffset で前後にずれた放送時間データも拾えるよう、前後に 1 日広げて読む。
	cached, err := sqlc.New(m.db).ListSyobocalPrograms(ctx, sqlc.ListSyobocalProgramsParams{
		ChannelIds: slices.Sorted(maps.Keys(channelIDs)), RangeStart: from.Add(-day), RangeEnd: to.Add(day),
	})
	if err != nil {
		return fmt.Errorf("failed to read the syobocal programs: %w", err)
	}
	byChannel := map[int32][]*sqlc.SyobocalProgram{}
	for i := range cached {
		byChannel[cached[i].ChannelID] = append(byChannel[cached[i].ChannelID], &cached[i])
	}

	now := m.clock.Now()
	for _, t := range targets {
		if finalize && slices.ContainsFunc(daysOf(t.startAt, t.endAt), func(d time.Time) bool {
			return failed[channelDay{channelID: t.channelID, day: d}]
		}) {
			continue
		}
		episodes := episodesOf(t, byChannel[t.channelID])
		if err := m.write(ctx, t, episodes, finalize, now); err != nil {
			return err
		}
		result.Programs++
		if len(episodes) > 0 {
			result.Linked++
		}
		if finalize {
			result.Finalized++
		}
	}

	return nil
}

// span は、しょぼいカレンダーに 1 回で問い合わせる期間 (日本時間の日の始まりから、終わりの日の次の日の始まりまで)。
type span struct{ from, to time.Time }

// refresh は、required の日のうち、決めた日時より前に引いた (か、まだ引いていない) 放送時間データを引き直す。
// ChID ごとに続いた日をまとめ、同じ期間を引く ChID をまとめて、1 回の問い合わせにする。引けなかった日を返す。
func (m *Matcher) refresh(ctx context.Context, required map[channelDay]time.Time, result *MatchResult) (map[channelDay]bool, error) {
	stale, err := m.staleDays(ctx, required)
	if err != nil {
		return nil, err
	}
	groups := spansOf(stale)
	failed := map[channelDay]bool{}
	for _, s := range slices.SortedFunc(maps.Keys(groups), func(a, b span) int { return a.from.Compare(b.from) }) {
		ids := groups[s]
		slices.Sort(ids)
		for chunk := range slices.Chunk(ids, syobocal.MaxChannelsPerRequest) {
			result.Requests++
			err := m.fetch(ctx, chunk, s.from, s.to)
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			result.Failed++
			m.logger.WarnContext(ctx, "failed to look up syobocal programs",
				slog.Any("channels", chunk), slog.Time("from", s.from), slog.Time("to", s.to), slog.Any("error", err))
			for _, id := range chunk {
				for d := s.from; d.Before(s.to); d = d.AddDate(0, 0, 1) {
					failed[channelDay{channelID: id, day: d}] = true
				}
			}
		}
	}

	return failed, nil
}

// staleDays は、required の日のうち、決めた日時より前に引いた (か、まだ引いていない) 日を、ChID ごとに返す。
func (m *Matcher) staleDays(ctx context.Context, required map[channelDay]time.Time) (map[int32][]time.Time, error) {
	channelIDs := map[int32]bool{}
	first, last := time.Time{}, time.Time{}
	for key := range required {
		channelIDs[key.channelID] = true
		if first.IsZero() || key.day.Before(first) {
			first = key.day
		}
		last = maxTime(last, key.day)
	}
	lookups, err := sqlc.New(m.db).ListSyobocalLookups(ctx, sqlc.ListSyobocalLookupsParams{
		ChannelIds: slices.Sorted(maps.Keys(channelIDs)), RangeStart: first, RangeEnd: last.Add(day),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read the syobocal lookups: %w", err)
	}
	fetched := map[channelDay]time.Time{}
	for _, lookup := range lookups {
		fetched[channelDay{channelID: lookup.ChannelID, day: lookup.DayStart.UTC()}] = lookup.FetchedAt
	}
	stale := map[int32][]time.Time{}
	for key, after := range required {
		if at, ok := fetched[key]; ok && !at.Before(after) {
			continue
		}
		stale[key.channelID] = append(stale[key.channelID], key.day)
	}

	return stale, nil
}

// spansOf は、ChID ごとの日を、続いた日の期間にまとめ、同じ期間を引く ChID をまとめる。
func spansOf(stale map[int32][]time.Time) map[span][]int32 {
	groups := map[span][]int32{}
	for channelID, days := range stale {
		slices.SortFunc(days, func(a, b time.Time) int { return a.Compare(b) })
		start := days[0]
		for i := 1; i <= len(days); i++ {
			if i < len(days) && days[i].Equal(days[i-1].AddDate(0, 0, 1)) {
				continue
			}
			key := span{from: start, to: days[i-1].AddDate(0, 0, 1)}
			groups[key] = append(groups[key], channelID)
			if i < len(days) {
				start = days[i]
			}
		}
	}

	return groups
}

// fetch は、ChID channelIDs の from から to までの放送時間データを引き、キャッシュを入れ替える。
func (m *Matcher) fetch(ctx context.Context, channelIDs []int32, from, to time.Time) error {
	programs, err := m.client.ProgLookup(ctx, channelIDs, from, to)
	if err != nil {
		return err
	}
	now := m.clock.Now()

	return pgx.BeginFunc(ctx, m.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		pids := make([]int32, 0, len(programs))
		for _, program := range programs {
			pids = append(pids, program.PID)
			err := q.UpsertSyobocalProgram(ctx, sqlc.UpsertSyobocalProgramParams{
				Pid: program.PID, Tid: program.TID, ChannelID: program.ChannelID, StartAt: program.StartAt, EndAt: program.EndAt,
				OffsetSeconds: program.OffsetSeconds, Count: program.Count, SubTitle: program.SubTitle,
				StSubTitle: program.STSubTitle, Flag: program.Flag, Deleted: program.Deleted, Now: now,
			})
			if err != nil {
				return fmt.Errorf("failed to save a syobocal program: %w", err)
			}
		}
		// 引いた期間にあったのに、返ってこなかった放送時間データは、しょぼいカレンダーから消えたものとして消す。
		if err := q.DeleteMissingSyobocalPrograms(ctx, sqlc.DeleteMissingSyobocalProgramsParams{
			ChannelIds: channelIDs, RangeStart: from, RangeEnd: to, Pids: pids,
		}); err != nil {
			return fmt.Errorf("failed to delete missing syobocal programs: %w", err)
		}
		for _, id := range channelIDs {
			for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
				if err := q.UpsertSyobocalLookup(ctx, sqlc.UpsertSyobocalLookupParams{ChannelID: id, DayStart: d, Now: now}); err != nil {
					return fmt.Errorf("failed to save a syobocal lookup: %w", err)
				}
			}
		}

		return nil
	})
}

// write は、番組 t の結びつきを episodes で入れ替える。finalize なら、結びつきを確定させる。
// 放送時間が変わった番組 (設計書 4.6) の、前の放送時間の結びつきは消す。アーカイブへ移した、同じイベント ID の番組の結びつきは残す。
func (m *Matcher) write(ctx context.Context, t *target, episodes []*Episode, finalize bool, now time.Time) error {
	return pgx.BeginFunc(ctx, m.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.DeleteMovedProgramAnimeMatches(ctx, sqlc.DeleteMovedProgramAnimeMatchesParams{
			ServiceID: t.serviceID, EventID: t.eventID, StartAt: t.startAt,
		}); err != nil {
			return fmt.Errorf("failed to delete the moved anime match: %w", err)
		}
		var finalizedAt *time.Time
		if finalize {
			finalizedAt = &now
		}
		if err := q.UpsertProgramAnimeMatch(ctx, sqlc.UpsertProgramAnimeMatchParams{
			ServiceID: t.serviceID, StartAt: t.startAt, EventID: t.eventID, Now: now, FinalizedAt: finalizedAt,
		}); err != nil {
			return fmt.Errorf("failed to save the anime match: %w", err)
		}

		return replaceEpisodes(ctx, q, t.serviceID, t.startAt, episodes)
	})
}

// replaceEpisodes は、番組 (serviceID, startAt) に結びついた話を episodes で入れ替える。
func replaceEpisodes(ctx context.Context, q *sqlc.Queries, serviceID int64, startAt time.Time, episodes []*Episode) error {
	if err := q.DeleteProgramAnimeEpisodes(ctx, sqlc.DeleteProgramAnimeEpisodesParams{ServiceID: serviceID, StartAt: startAt}); err != nil {
		return fmt.Errorf("failed to delete the anime episodes: %w", err)
	}
	for i, episode := range episodes {
		err := q.InsertProgramAnimeEpisode(ctx, sqlc.InsertProgramAnimeEpisodeParams{
			ServiceID: serviceID, StartAt: startAt, Position: int16(min(i, maxEpisodes)), Source: episode.Source,
			SyobocalTid: episode.TID, SyobocalPid: episode.PID, CountStart: episode.CountStart, CountEnd: episode.CountEnd,
			Subtitle: episode.Subtitle, IsFirst: episode.First, IsLast: episode.Last,
		})
		if err != nil {
			return fmt.Errorf("failed to save an anime episode: %w", err)
		}
	}

	return nil
}

// maxEpisodes は、1 つの番組に結びつける話の数の上限。
const maxEpisodes = 100

// episodesOf は、番組 t と放送時間が重なるしょぼいカレンダーの放送時間データから、結びつける話を決める。
// 放送時間データの放送時間 (StOffset でずらしたもの) と番組の放送時間が、短いほうの半分以上重なれば結びつける。
// 2 話連続の放送では、番組 1 つに放送時間データが 2 つ重なるので、両方に結びつける。
func episodesOf(t *target, candidates []*sqlc.SyobocalProgram) []*Episode {
	type matched struct {
		start   time.Time
		program *sqlc.SyobocalProgram
	}
	found := []matched{}
	for _, candidate := range candidates {
		if candidate.Deleted {
			continue
		}
		offset := time.Duration(candidate.OffsetSeconds) * time.Second
		start, end := candidate.StartAt.Add(offset), candidate.EndAt.Add(offset)
		overlap := minTime(end, t.endAt).Sub(maxTime(start, t.startAt))
		shorter := min(end.Sub(start), t.endAt.Sub(t.startAt))
		if overlap <= 0 || overlap*2 < shorter {
			continue
		}
		found = append(found, matched{start: start, program: candidate})
	}
	slices.SortFunc(found, func(a, b matched) int {
		if c := a.start.Compare(b.start); c != 0 {
			return c
		}

		return cmp.Compare(a.program.Pid, b.program.Pid)
	})

	episodes := make([]*Episode, 0, min(len(found), maxEpisodes))
	for _, f := range found[:min(len(found), maxEpisodes)] {
		episodes = append(episodes, episodeOf(f.program))
	}

	return episodes
}

// episodeOf は、しょぼいカレンダーの放送時間データ 1 つを、話にする。
func episodeOf(program *sqlc.SyobocalProgram) *Episode {
	episode := &Episode{
		Source: SourceSyobocal, TID: new(program.Tid), PID: new(program.Pid), CountStart: nil, CountEnd: nil, Subtitle: nil,
		First: program.Flag&syobocal.FlagNew != 0, Last: program.Flag&syobocal.FlagFinal != 0,
	}
	if match := multipleEpisodes.FindStringSubmatch(program.SubTitle); match != nil {
		episode.CountStart, episode.CountEnd = parseCount(match[1]), parseCount(match[2])
	} else {
		episode.CountStart = parseCount(program.Count)
		episode.CountEnd = episode.CountStart
		// 放送時間データのサブタイトルは、特番の名前などに使われる。
		if program.SubTitle != "" {
			episode.Subtitle = new(program.SubTitle)
		}
	}
	if program.StSubTitle != "" {
		episode.Subtitle = new(program.StSubTitle)
	}

	return episode
}

// parseCount は、話数を読む。読めなければ nil を返す。
func parseCount(value string) *float64 {
	count, err := strconv.ParseFloat(value, 64)
	if err != nil || count < 0 {
		return nil
	}

	return &count
}

// prune は、放送が終わって時間のたったキャッシュと、番組表からも過去の番組からもなくなった番組の結びつきを消す。
func (m *Matcher) prune(ctx context.Context, now time.Time) (int64, error) {
	q := sqlc.New(m.db)
	before := now.Add(-cacheRetention)
	programs, err := q.PruneSyobocalPrograms(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("failed to prune the syobocal programs: %w", err)
	}
	lookups, err := q.PruneSyobocalLookups(ctx, before.Add(-day))
	if err != nil {
		return 0, fmt.Errorf("failed to prune the syobocal lookups: %w", err)
	}
	matches, err := q.PruneProgramAnimeMatches(ctx, now.Add(-orphanGrace))
	if err != nil {
		return 0, fmt.Errorf("failed to prune the anime matches: %w", err)
	}

	return programs + lookups + matches, nil
}

// saveCursor は、読み終えた EPG の変更の記録の版数を覚える。
func (m *Matcher) saveCursor(ctx context.Context, version int64) error {
	if err := sqlc.New(m.db).SaveAnimeMatchCursor(ctx, sqlc.SaveAnimeMatchCursorParams{EpgVersion: version, Now: m.clock.Now()}); err != nil {
		return fmt.Errorf("failed to save the anime match cursor: %w", err)
	}

	return nil
}

// daysOf は、start から end までと重なる、日本時間の日の始まりを返す。
func daysOf(start, end time.Time) []time.Time {
	first := startOfDay(start)
	days := []time.Time{first}
	for d := first.AddDate(0, 0, 1); d.Before(end); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}

	return days
}

// startOfDay は、t を含む日本時間の日の始まりを、UTC で返す。表の鍵に使うので、タイムゾーンを UTC にそろえる。
func startOfDay(t time.Time) time.Time {
	local := t.In(syobocal.Tokyo())

	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).UTC()
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
