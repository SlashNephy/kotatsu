package rule

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"runtime"
	"slices"
	"sync"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/reserve"
)

const (
	// serviceIDBase は、Mirakurun のサービス ID を組み立てる基数 (networkId × 100000 + serviceId)。
	serviceIDBase = 100_000
	// relatedShared は、同じイベントを複数のサービスで放送する関係 (イベント共有)。
	relatedShared = "shared"
	day           = 24 * time.Hour
)

// engineActor は、照合が作ったり消したりした予約の、監査ログの操作者。
func engineActor() *audit.Actor {
	return audit.System("neko")
}

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確認する。
type Guard func(ctx context.Context, tx pgx.Tx) error

// Engine は、ルールの照合を行う (設計書 4.3)。
//
// 全ルールについて「予約すべき番組の集合」を計算し、既存の予約と照合する。何度実行しても同じ結果になる。
//
//   - 集合にあって予約がない番組には、ルール予約を作る。
//   - 予約はあるのに、どのルールにも一致しなくなった番組は、放送が始まっていなければルール予約を消す。
//     無効にしたルール予約も、一致しなくなれば消す。一致している間は、無効のまま残すので作り直されない。
//     番組が EPG から消えた予約は、予約に残した番組の情報で、結び付いたルールに照合し直し、一致すれば残す。
//     ただし、同じ枠にルールに一致する別のイベントの番組が出ていれば、差し替えられたとみなして消す (#518、#1875)。
//     終了時刻が未定になった番組の予約は、延長などの一時的なことなので残す (#518)。
//   - 手動の番組予約がある番組は、手動の設定を優先する。一致したルールの関係だけを残す。
//     番組が EPG から消えた有効な手動の予約の時刻 (マージンを含めない) に収まる番組には、ルール予約を作らない。
//     手動の予約がその番組を録るからである。はみ出す番組は、録り逃さないよう予約する (#1903)。
//
// あわせて、ユーザーごとの視聴ルールも同じ番組に照合し、一致した番組に視聴予約を付ける (設計書 4.8)。
type Engine struct {
	pool         *pgxpool.Pool
	clock        clock.Clock
	location     *time.Location
	guard        Guard
	reservations *reserve.Reservations
}

// NewEngine は Engine を作る。location は、ルールの曜日と時間帯を判定するタイムゾーン。
func NewEngine(pool *pgxpool.Pool, clk clock.Clock, location *time.Location, guard Guard, reservations *reserve.Reservations) *Engine {
	return &Engine{pool: pool, clock: clk, location: location, guard: guard, reservations: reservations}
}

// Result は、1 回の照合の結果。
type Result struct {
	Created int
	Deleted int
	// Linked と Unlinked は、予約と一致したルールの関係を足した数と除いた数。
	Linked   int
	Unlinked int
	// Skipped は、条件を読めずに照合から外したルールと、読めなかった理由。そのルールの予約は、消さずに残す (#529)。
	Skipped map[uuid.UUID]error
	// WatchCreated と WatchDeleted は、視聴ルールで付けた視聴予約と、一致しなくなって外した視聴予約の数 (設計書 4.8)。
	WatchCreated int
	WatchDeleted int
	// WatchSkipped は、条件を読めずに照合から外した視聴ルールと、読めなかった理由。そのユーザーの視聴予約は、消さずに残す。
	WatchSkipped map[uuid.UUID]error
	// DismissalsPruned は、番組が programs からなくなったので消した、手で外した視聴予約の記録の数。
	DismissalsPruned int
	// WishlistCreated と WishlistDeleted は、見たいリストの作品の放送の通知 (#1234) で付けた視聴予約と、外した視聴予約の数。
	WishlistCreated int
	WishlistDeleted int
}

// newResult は、空の照合の結果を返す。
func newResult() *Result {
	return &Result{
		Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{},
		WatchCreated: 0, WatchDeleted: 0, WatchSkipped: map[uuid.UUID]error{}, DismissalsPruned: 0, WishlistCreated: 0, WishlistDeleted: 0,
	}
}

// programKey は番組の識別子 (サービスと eventId の組)。
type programKey struct {
	serviceID int64
	eventID   int64
}

func compareKeys(a, b programKey) int {
	return cmp.Or(cmp.Compare(a.serviceID, b.serviceID), cmp.Compare(a.eventID, b.eventID))
}

// Run は、ルールの照合を 1 回行う。すべて 1 つのトランザクションで読み書きする。
func (e *Engine) Run(ctx context.Context) (*Result, error) {
	result := newResult()
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		if err := e.guard(ctx, tx); err != nil {
			return err
		}
		now := e.clock.Now()
		rows, err := listMatchablePrograms(ctx, tx, now)
		if err != nil {
			return err
		}
		rules, err := e.enabledRules(ctx, tx, result)
		if err != nil {
			return err
		}
		watchRules, skippedUsers, err := e.enabledWatchRules(ctx, tx, result)
		if err != nil {
			return err
		}
		// 番組を照合する形にするのは重いので、録画のルールと視聴ルールを、1 回の走査でまとめて照合する。
		matched, watchMatched, err := matchPrograms(rows, rules, watchRules)
		if err != nil {
			return err
		}
		// 前の回を比較する相手を選ぶのにも、apply と同じ残し方を使うので、絞り込む前に作る。
		keep := newKeepPolicy(rules, matched)
		desired, err := e.narrow(ctx, tx, now, matched, nil, keep, result)
		if err != nil {
			return err
		}
		if err := e.apply(ctx, tx, now, desired, keep, result); err != nil {
			return err
		}

		if err := e.applyWishlistWatch(ctx, tx, now, result); err != nil {
			return err
		}

		return e.applyWatch(ctx, tx, now, rows, watchMatched, skippedUsers, result)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to match rules: %w", err)
	}

	return result, nil
}

// desired は、予約すべき番組と、それぞれに一致したルールを返す。
// skipped を渡せば、重複の回避で予約すべき集合から外した番組を、そこに入れる。
func (e *Engine) desired(
	ctx context.Context, tx pgx.Tx, now time.Time, result *Result, skipped map[programKey]*skippedProgram,
) (map[programKey]*desiredProgram, error) {
	rules, err := e.enabledRules(ctx, tx, result)
	if err != nil {
		return nil, err
	}
	rows, err := listMatchablePrograms(ctx, tx, now)
	if err != nil {
		return nil, err
	}

	matched, _, err := matchPrograms(rows, rules, []*compiledWatchRule{})
	if err != nil {
		return nil, err
	}
	keep := newKeepPolicy(rules, matched)

	return e.narrow(ctx, tx, now, matched, skipped, keep, result)
}

// narrow は、ルールに一致した番組を、人が取り消した放送、重複の回避、イベント共有、同じ回のまとめ、前の回を録らない設定で絞り込み、
// 予約すべき番組にする。skipped を渡せば、重複の回避、同じ回のまとめ、前の回を録らない設定で予約すべき集合から外した番組を、そこに入れる。
// keep と result は、apply と同じく、どのルールにも一致しなくなったルール予約を残すかを決める。前の回を比較する相手から、
// この照合で消える予約を除くのに使う。
func (e *Engine) narrow(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram, skipped map[programKey]*skippedProgram,
	keep *keepPolicy, result *Result,
) (map[programKey]*desiredProgram, error) {
	if err := excludeCancelledBroadcasts(ctx, tx, now, matched); err != nil {
		return nil, err
	}
	if err := e.avoidDuplicates(ctx, tx, now, matched, skipped); err != nil {
		return nil, err
	}
	pickSharedEvents(matched, func(d *desiredProgram) []byte { return d.row.RelatedItems })
	// イベント共有で残した側の放送を見る。先に外すと、落とした側の放送が残って、同じ放送を別のサービスで録る。
	// まとめる前に外すのは、手動で予約した同じ回を、まとめる相手に入れないのと揃えるためである (設計書 4.4)。
	if err := excludeManuallyReservedSlots(ctx, tx, now, matched, keep); err != nil {
		return nil, err
	}
	// イベント共有で落とした側の放送を除いてからまとめる。
	// まとめた後に前の回を外す。両方に当たる放送は、まとめた理由 (別の放送で録る、録画済み) を優先して出す (#1499)。
	bundled := map[programKey]*skippedProgram{}
	if err := bundleSameEpisodes(ctx, tx, now, matched, skipped, bundled); err != nil {
		return nil, err
	}
	// この照合で消えるルール予約は、前の回を比較する相手にしない。消える予約を理由に若い回を外すと、両方の予約が消える (#1724)。
	removed, err := removedRuleReservations(ctx, tx, now, matched, keep, result)
	if err != nil {
		return nil, err
	}
	if err := skipEarlierEpisodes(ctx, tx, now, matched, skipped, bundled, removed); err != nil {
		return nil, err
	}

	return matched, nil
}

// listMatchablePrograms は、ルールの対象になる番組を返す。
func listMatchablePrograms(ctx context.Context, tx pgx.Tx, now time.Time) ([]sqlc.ListMatchableProgramsRow, error) {
	rows, err := sqlc.New(tx).ListMatchablePrograms(ctx, sqlc.ListMatchableProgramsParams{Now: now, MediaServiceTypes: domain.MediaServiceTypes()})
	if err != nil {
		return nil, fmt.Errorf("failed to list programs: %w", err)
	}

	return rows, nil
}

// matchPrograms は、番組ごとに、一致したルールを rules の順に、一致した視聴ルールを watchRules の順に集める。
// 必ず含む文字列を決められない正規表現は、前段で絞り込めず、番組ごとに照合するので重い (#878)。
// そこで、番組を GOMAXPROCS の数のまとまりに分け、並べて照合する。GOMAXPROCS は、コンテナの CPU の制限にも従う。
func matchPrograms(
	rows []sqlc.ListMatchableProgramsRow, rules []*compiledRule, watchRules []*compiledWatchRule,
) (map[programKey]*desiredProgram, map[programKey]*watchProgram, error) {
	workers := max(1, min(runtime.GOMAXPROCS(0), len(rows)))
	parts := make([]map[programKey]*desiredProgram, workers)
	watchParts := make([]map[programKey]*watchProgram, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			part := map[programKey]*desiredProgram{}
			watchPart := map[programKey]*watchProgram{}
			for i := w; i < len(rows); i += workers {
				if err := matchProgram(rows[i], rules, watchRules, part, watchPart); err != nil {
					errs[w] = err

					return
				}
			}
			parts[w] = part
			watchParts[w] = watchPart
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}

	matched := map[programKey]*desiredProgram{}
	for _, part := range parts {
		maps.Copy(matched, part)
	}
	watchMatched := map[programKey]*watchProgram{}
	for _, part := range watchParts {
		maps.Copy(watchMatched, part)
	}

	return matched, watchMatched, nil
}

// matchProgram は、番組 row に一致したルールを part に、一致した視聴ルールを watchPart に足す。
func matchProgram(
	row sqlc.ListMatchableProgramsRow,
	rules []*compiledRule,
	watchRules []*compiledWatchRule,
	part map[programKey]*desiredProgram,
	watchPart map[programKey]*watchProgram,
) error {
	program, err := toMatchProgram(row)
	if err != nil {
		return err
	}
	key := programKey{serviceID: row.ServiceID, eventID: row.EventID}
	for _, r := range rules {
		if !r.covers(row.StartAt) || !r.matcher.Match(program) {
			continue
		}
		d, ok := part[key]
		if !ok {
			d = &desiredProgram{row: row, rules: []*compiledRule{}}
			part[key] = d
		}
		d.rules = append(d.rules, r)
	}
	for _, r := range watchRules {
		if !r.matcher.Match(program) {
			continue
		}
		d, ok := watchPart[key]
		if !ok {
			d = &watchProgram{row: row, rules: []*compiledWatchRule{}}
			watchPart[key] = d
		}
		d.rules = append(d.rules, r)
	}

	return nil
}

// desiredProgram は、予約すべき番組と、一致したルール。
type desiredProgram struct {
	row   sqlc.ListMatchableProgramsRow
	rules []*compiledRule
}

func (d *desiredProgram) ruleIDs() map[uuid.UUID]bool {
	ids := map[uuid.UUID]bool{}
	for _, r := range d.rules {
		ids[r.ID] = true
	}

	return ids
}

// followEventRelay は、一致したルールのどれかがイベントリレーを追うなら true を返す。
func (d *desiredProgram) followEventRelay() bool {
	return slices.ContainsFunc(d.rules, func(r *compiledRule) bool { return r.FollowEventRelay })
}

type compiledRule struct {
	*Rule

	matcher *match.Matcher
}

// covers は、開始時刻がルールの有効期間に入るかどうかを返す。
func (r *compiledRule) covers(start time.Time) bool {
	return (r.ValidFrom == nil || !start.Before(*r.ValidFrom)) && (r.ValidUntil == nil || start.Before(*r.ValidUntil))
}

func (e *Engine) enabledRules(ctx context.Context, tx pgx.Tx, result *Result) ([]*compiledRule, error) {
	rows, err := sqlc.New(tx).ListRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	rules := []*compiledRule{}
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		// 条件を読めないルール (ローリング更新の途中で、新しいバージョンが足した条件など) は飛ばし、ほかのルールの照合を続ける (#529)。
		r, err := fromRow(row)
		if err != nil {
			result.Skipped[row.ID] = err

			continue
		}
		// 番組を選ぶ条件のないルール (時間帯と曜日だけなど) は、その時間の全局に一致するので当てない (#1501)。
		// 読めないルールと同じく、そのルールで作り済みの予約は消さずに残す。
		if !r.Conditions.HasTarget() {
			result.Skipped[row.ID] = ErrUnconditioned

			continue
		}
		matcher, err := match.Compile(r.Conditions, e.location)
		if err != nil {
			result.Skipped[row.ID] = err

			continue
		}
		rules = append(rules, &compiledRule{Rule: r, matcher: matcher})
	}

	return rules, nil
}

// toMatchProgram は、DB の番組を照合する形にする。
func toMatchProgram(row sqlc.ListMatchableProgramsRow) (*match.Program, error) {
	upstreamGenres := []*domain.Genre{}
	if err := json.Unmarshal(row.Genres, &upstreamGenres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	genres := make([]*match.Genre, 0, len(upstreamGenres))
	for _, g := range upstreamGenres {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: new(g.Lv2)})
	}
	// 番組ごとに NFKC で正規化し直すと重いので、同期で正規化して持ったカラムを使う (#530)。
	extended := row.ExtendedNormalized
	if extended == nil {
		// カラムを足す前に書いた番組は、上流の詳細から正規化する。
		upstream := map[string]string{}
		if err := json.Unmarshal(row.Extended, &upstream); err != nil {
			return nil, fmt.Errorf("failed to decode the extended of program %d/%d: %w", row.ServiceID, row.EventID, err)
		}
		extended = new(domain.NormalizeExtended(upstream))
	}

	sharedFrom, err := domain.SharedFrom(row.ServiceID, row.RelatedItems)
	if err != nil {
		return nil, fmt.Errorf("failed to decode the related items of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}

	return &match.Program{
		ServiceID:   row.ServiceID,
		SharedFrom:  sharedFrom,
		ChannelType: row.ChannelType,
		Name:        row.NameNormalized,
		Description: row.DescriptionNormalized,
		Extended:    *extended,
		Genres:      genres,
		StartAt:     row.StartAt,
		Duration:    row.EndAt.Sub(row.StartAt),
		IsFree:      row.IsFree,
		// 囲み文字を括弧の書き方にする前に正規化した番組名では、囲み文字と番組名の文字を見分けられないので、
		// 囲み文字は正規化する前の番組名から取り出す。
		Marks: domain.Marks(row.Name),
	}, nil
}

// avoidDuplicates は、重複の回避を指定したルールについて、同じ番組名を期間内に録画済みの番組からそのルールを外す (設計書 4.4)。
// どのルールにも一致しなくなった番組は、予約すべき集合から除き、skipped が nil でなければ、外したルールと一緒に入れる。
func (e *Engine) avoidDuplicates(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram, skipped map[programKey]*skippedProgram,
) error {
	titleKeys := map[programKey]string{}
	for key, d := range matched {
		if slices.ContainsFunc(d.rules, func(r *compiledRule) bool { return r.AvoidDuplicates }) {
			titleKeys[key] = TitleKey(d.row.Name)
		}
	}
	if len(titleKeys) == 0 {
		return nil
	}

	rows, err := sqlc.New(tx).ListLatestRecordingHistory(ctx, slices.Sorted(maps.Values(titleKeys)))
	if err != nil {
		return fmt.Errorf("failed to read the recording history: %w", err)
	}
	type historyKey struct {
		titleKey  string
		serviceID int64
	}
	lastRecorded := map[historyKey]time.Time{}
	for _, row := range rows {
		lastRecorded[historyKey{titleKey: row.TitleKey, serviceID: row.ServiceID}] = row.LastEndAt
	}

	for key, titleKey := range titleKeys {
		last, ok := lastRecorded[historyKey{titleKey: titleKey, serviceID: key.serviceID}]
		if !ok {
			continue
		}
		d := matched[key]
		removed := []*compiledRule{}
		d.rules = slices.DeleteFunc(d.rules, func(r *compiledRule) bool {
			if !r.AvoidDuplicates {
				return false
			}
			if r.DuplicateWindowDays == nil || !last.Before(now.Add(-time.Duration(*r.DuplicateWindowDays)*day)) {
				removed = append(removed, r)

				return true
			}

			return false
		})
		if len(d.rules) == 0 {
			delete(matched, key)
			if skipped != nil {
				skipped[key] = &skippedProgram{
					row: d.row, rules: removed, reason: SkipReasonDuplicateTitle, lastRecorded: new(last), recorded: nil, instead: nil,
					laterEpisode: 0,
				}
			}
		}
	}

	return nil
}

// CancelledBroadcastTolerance は、人が取り消した予約と同じ放送とみなす、開始の差の最大 (付録 A、#1504)。
// イベント ID が変わる経路 (局の送り直しと、枝番の局のまとめが一時的に外れたとき) では、開始はほとんど動かない。
// 延長や繰り下げで開始が動いても、イベント ID が同じなら、取り消した予約そのものが番組を追う。
// 一方、本番の番組表では、同じ局で同じ番組名の放送が 15 分未満の間隔で並ぶことがある (多くは穴埋めの番組)。
// そうした別の回まで録らなくならないよう、短くする。
const CancelledBroadcastTolerance = 10 * time.Minute

// cancelledBroadcast は、人が取り消した予約の放送。
type cancelledBroadcast struct {
	program programKey
	name    string
	startAt time.Time
}

// broadcastChannel は、物理チャンネル。
type broadcastChannel struct {
	channelType string
	channel     string
}

// excludeCancelledBroadcasts は、人が取り消した (無効にした) 予約と同じ放送の番組を、予約すべき集合から外す (#1504、設計書 4.3)。
// 同じ放送とは、物理チャンネルが同じで、番組名が完全に一致し、開始の差が CancelledBroadcastTolerance の内の番組である。
// 局が同じ番組を別のイベント ID で送り直しても、取り消した回を予約し直さない。取り消した予約の番組そのものは、
// 無効の予約が残っているので、ここでは外さない。
func excludeCancelledBroadcasts(ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram) error {
	rows, err := sqlc.New(tx).ListCancelledBroadcasts(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list cancelled broadcasts: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	cancelled := map[broadcastChannel][]cancelledBroadcast{}
	for _, row := range rows {
		channel := broadcastChannel{channelType: row.ChannelType, channel: row.Channel}
		cancelled[channel] = append(cancelled[channel], cancelledBroadcast{
			program: programKey{serviceID: row.ServiceID, eventID: *row.EventID}, name: row.Name, startAt: row.StartAt,
		})
	}

	for key, d := range matched {
		for _, c := range cancelled[broadcastChannel{channelType: d.row.ChannelType, channel: d.row.Channel}] {
			if c.program != key && c.name == d.row.Name && d.row.StartAt.Sub(c.startAt).Abs() <= CancelledBroadcastTolerance {
				delete(matched, key)

				break
			}
		}
	}

	return nil
}

// pickSharedEvents は、同じイベントを複数のサービスで放送している番組 (イベント共有) が複数一致したとき、
// サービス ID の最も小さい 1 つだけを残す (設計書 4.4)。relatedItems は、番組の relatedItems を返す。
func pickSharedEvents[V any](matched map[programKey]V, relatedItems func(V) []byte) {
	for _, key := range slices.SortedFunc(maps.Keys(matched), compareKeys) {
		d, ok := matched[key]
		if !ok {
			continue
		}
		var related []*domain.RelatedItem
		if err := json.Unmarshal(relatedItems(d), &related); err != nil {
			continue
		}
		networkID := key.serviceID / serviceIDBase
		for _, item := range related {
			if item.Type != relatedShared {
				continue
			}
			itemNetwork := int64(item.NetworkID)
			if itemNetwork == 0 {
				itemNetwork = networkID
			}
			other := programKey{serviceID: itemNetwork*serviceIDBase + int64(item.ServiceID), eventID: int64(item.EventID)}
			// key は小さい順に見ているので、残っている自分より大きい番組を消せば、最も小さい 1 つが残る。
			if compareKeys(other, key) > 0 {
				delete(matched, other)
			}
		}
	}
}

// keepPolicy は、どのルールにも一致しなくなったルール予約を、消さずに残す条件のうち、照合ごとに決まるもの。
type keepPolicy struct {
	// enabled は、照合した (有効で、条件を読めた) ルール。無効にしたか消したルールは含まない。
	enabled map[uuid.UUID]*compiledRule
	// slots は、サービスごとの、有効なルールのどれかに一致した、EPG にある番組の放送の枠 (#1875)。
	// 絞り込む前の一致で作るので、apply と、前の回を比較する相手を選ぶ removedRuleReservations で同じものを見る。
	slots map[int64][]broadcastSlot
	// manuallyReserved は、EPG から消えた番組の有効な手動の予約の時刻に収まるので、予約すべき集合から外した番組 (#1903)。
	// 絞り込みの中で excludeManuallyReservedSlots が入れる。
	manuallyReserved map[programKey]bool
}

// broadcastSlot は、番組の放送の枠 [startAt, endAt)。
type broadcastSlot struct {
	startAt time.Time
	endAt   time.Time
}

// overlaps は、2 つの放送が同じ枠かどうかを返す。重なりが短い方の長さの半分を超えるときだけ、同じ枠とみなす。
// 前後の回が少し重なっただけで、同じ枠とみなさないためである (#1875)。
func (s broadcastSlot) overlaps(other broadcastSlot) bool {
	start := latest(s.startAt, other.startAt)
	end := earliest(s.endAt, other.endAt)
	shorter := min(s.endAt.Sub(s.startAt), other.endAt.Sub(other.startAt))

	return end.Sub(start)*2 > shorter
}

// replaced は、EPG から消えた番組の予約 row の枠に、ルールに一致した別の番組が出ているかを返す (#1875)。
// 局が番組を別のイベント ID で出し直して差し替えると、古いイベントは EPG から消え、同じ枠に新しいイベントが入る。
// 一時的に消えた回の予約を、前後の回が少し重なっただけで消さないよう、同じ枠かは overlaps で見る。
func (k *keepPolicy) replaced(row *sqlc.ListFutureProgramReservationsRow) bool {
	return slices.ContainsFunc(k.slots[row.ServiceID], broadcastSlot{startAt: row.StartAt, endAt: row.EndAt}.overlaps)
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}

	return b
}

// matchesStored は、EPG から消えた番組の予約 row が、予約に残した番組の情報で、結び付いたルール have のうち
// 照合したもののどれかに一致するかを返す。番組が消えても予約は残すが (#518)、人がルールを書き換えて外した予約まで
// 残さないよう、照合し直す。
func (k *keepPolicy) matchesStored(row *sqlc.ListFutureProgramReservationsRow, have map[uuid.UUID]bool) (bool, error) {
	if !linksAny(have, k.enabled) {
		return false, nil
	}
	program, err := storedProgram(row)
	if err != nil {
		return false, err
	}
	for id := range have {
		if r := k.enabled[id]; r != nil && r.covers(row.StartAt) && r.matcher.Match(program) {
			return true, nil
		}
	}

	return false, nil
}

// storedProgram は、EPG から消えた番組の予約 row に残した番組の情報を、照合する形にする。
// 予約は、無料放送かどうかとイベント共有を持たないので、無料放送で、共有していないとみなす。
// 分からない条件で外して消すと、録り逃すからである。
func storedProgram(row *sqlc.ListFutureProgramReservationsRow) (*match.Program, error) {
	stored := []*domain.Genre{}
	if err := json.Unmarshal(row.Genres, &stored); err != nil {
		return nil, fmt.Errorf("failed to decode the genres of reservation %s: %w", row.ID, err)
	}
	genres := make([]*match.Genre, 0, len(stored))
	for _, g := range stored {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: new(g.Lv2)})
	}
	extended := map[string]string{}
	if err := json.Unmarshal(row.Extended, &extended); err != nil {
		return nil, fmt.Errorf("failed to decode the extended of reservation %s: %w", row.ID, err)
	}

	return &match.Program{
		ServiceID:   row.ServiceID,
		SharedFrom:  0,
		ChannelType: row.ChannelType,
		Name:        domain.Normalize(row.Name),
		Description: domain.Normalize(row.Description),
		Extended:    domain.NormalizeExtended(extended),
		Genres:      genres,
		StartAt:     row.StartAt,
		Duration:    row.EndAt.Sub(row.StartAt),
		IsFree:      true,
		Marks:       domain.Marks(row.Name),
	}, nil
}

// newKeepPolicy は keepPolicy を作る。matched は、ルールに一致した番組で、絞り込む前に渡す。
func newKeepPolicy(rules []*compiledRule, matched map[programKey]*desiredProgram) *keepPolicy {
	enabled := make(map[uuid.UUID]*compiledRule, len(rules))
	for _, r := range rules {
		enabled[r.ID] = r
	}
	slots := map[int64][]broadcastSlot{}
	for key, d := range matched {
		slots[key.serviceID] = append(slots[key.serviceID], broadcastSlot{startAt: d.row.StartAt, endAt: d.row.EndAt})
	}

	return &keepPolicy{
		enabled: enabled, slots: slots, manuallyReserved: map[programKey]bool{},
	}
}

// contains は、放送 other が、s の枠に収まるかどうかを返す。
func (s broadcastSlot) contains(other broadcastSlot) bool {
	return !other.startAt.Before(s.startAt) && !other.endAt.After(s.endAt)
}

// excludeManuallyReservedSlots は、番組が EPG から消えた有効な手動の予約の時刻 (開始から終了) に収まる、
// 同じサービスの番組を、予約すべき集合 matched から外し、keep に保存する (#1903)。
// 局が番組を別のイベント ID で出し直して差し替えると、古いイベントは EPG から消え、同じ枠に新しいイベントが入る。手動の予約は、
// 手動を優先して消さず (#1875)、番組を見失っても予定の終わりまで録る (設計書 5.1) ので、新しいイベントをルールで予約すると 2 本録る。
// 新しいイベントが予約の時刻からはみ出すとき (遅れる、延びるなど) は、手動の予約では録り逃すので外さない。2 本録るほうを選ぶ。
// マージンは、割り当てで隣の録画と重なると削られることがあるので、含めない。無効の手動の予約は録らないので見ない。
func excludeManuallyReservedSlots(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram, keep *keepPolicy,
) error {
	rows, err := sqlc.New(tx).ListFutureProgramReservations(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list reservations: %w", err)
	}
	manual := map[int64][]broadcastSlot{}
	for _, row := range rows {
		if row.Kind == domain.ReservationKindProgram && row.Enabled && !row.Listed {
			manual[row.ServiceID] = append(manual[row.ServiceID], broadcastSlot{startAt: row.StartAt, endAt: row.EndAt})
		}
	}
	if len(manual) == 0 {
		return nil
	}

	for key, d := range matched {
		if slices.ContainsFunc(manual[key.serviceID], func(s broadcastSlot) bool {
			return s.contains(broadcastSlot{startAt: d.row.StartAt, endAt: d.row.EndAt})
		}) {
			delete(matched, key)
			keep.manuallyReserved[key] = true
		}
	}

	return nil
}

// apply は、予約すべき集合と既存の予約を照合する。
func (e *Engine) apply(
	ctx context.Context, tx pgx.Tx, now time.Time, desired map[programKey]*desiredProgram, keep *keepPolicy, result *Result,
) error {
	q := sqlc.New(tx)
	existing, links, err := futureReservations(ctx, q, now)
	if err != nil {
		return err
	}

	reserved := map[programKey]bool{}
	for _, row := range existing {
		key := programKey{serviceID: row.ServiceID, eventID: *row.EventID}
		reserved[key] = true
		if err := e.reconcile(ctx, tx, now, row, desired[key], links[row.ID], keep, result); err != nil {
			return err
		}
	}

	for _, key := range slices.SortedFunc(maps.Keys(desired), compareKeys) {
		if reserved[key] {
			continue
		}
		d := desired[key]
		reservation, created, err := e.reservations.CreateRule(ctx, tx, engineActor(), key.serviceID, key.eventID, d.followEventRelay())
		if err != nil {
			return err
		}
		if created {
			result.Created++
		}
		if err := e.link(ctx, q, reservation.ID, map[uuid.UUID]bool{}, d.ruleIDs(), result); err != nil {
			return err
		}
	}

	return nil
}

// futureReservations は、放送が始まっていない、番組を指定した予約と、予約ごとに結び付いたルールの ID を返す。
func futureReservations(
	ctx context.Context, q *sqlc.Queries, now time.Time,
) ([]sqlc.ListFutureProgramReservationsRow, map[uuid.UUID]map[uuid.UUID]bool, error) {
	existing, err := q.ListFutureProgramReservations(ctx, now)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list reservations: %w", err)
	}
	linkRows, err := q.ListFutureReservationRules(ctx, now)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list reservation rules: %w", err)
	}
	links := map[uuid.UUID]map[uuid.UUID]bool{}
	for _, row := range linkRows {
		if links[row.ReservationID] == nil {
			links[row.ReservationID] = map[uuid.UUID]bool{}
		}
		links[row.ReservationID][row.RuleID] = true
	}

	return existing, links, nil
}

// removedRuleReservations は、この照合の apply が消すルール予約の番組を返す。予約すべき集合 matched にない番組のルール予約を、
// apply と同じ keepsUnmatched で確認する (#1724)。
func removedRuleReservations(
	ctx context.Context, tx pgx.Tx, now time.Time, matched map[programKey]*desiredProgram, keep *keepPolicy, result *Result,
) (map[programKey]bool, error) {
	existing, links, err := futureReservations(ctx, sqlc.New(tx), now)
	if err != nil {
		return nil, err
	}
	removed := map[programKey]bool{}
	for i := range existing {
		row := &existing[i]
		key := programKey{serviceID: row.ServiceID, eventID: *row.EventID}
		if row.Kind != domain.ReservationKindRule || matched[key] != nil {
			continue
		}
		kept, err := keepsUnmatched(row, links[row.ID], keep, result)
		if err != nil {
			return nil, err
		}
		if !kept {
			removed[key] = true
		}
	}

	return removed, nil
}

// reconcile は、既存の予約 1 件を、予約すべき集合に合わせる。d は、その番組が集合になければ nil。
func (e *Engine) reconcile(
	ctx context.Context,
	tx pgx.Tx,
	now time.Time,
	row sqlc.ListFutureProgramReservationsRow,
	d *desiredProgram,
	have map[uuid.UUID]bool,
	keep *keepPolicy,
	result *Result,
) error {
	q := sqlc.New(tx)
	isRule := row.Kind == domain.ReservationKindRule
	kept := false
	if d == nil && isRule {
		var err error
		if kept, err = keepsUnmatched(&row, have, keep, result); err != nil {
			return err
		}
	}
	switch {
	case kept:
		return nil
	case d == nil && isRule:
		// どのルールにも一致しなくなった。
		if err := e.reservations.Remove(ctx, tx, engineActor(), row.ID); err != nil {
			return err
		}
		result.Deleted++

		return nil
	case d == nil:
		// 手動の予約は残し、ルールとの関係だけを除く。
		return e.link(ctx, q, row.ID, have, map[uuid.UUID]bool{}, result)
	}

	if err := e.link(ctx, q, row.ID, have, d.ruleIDs(), result); err != nil {
		return err
	}
	// ルール予約のイベントリレーへの追従は、一致したルールに合わせる。手動の予約は手動の設定を優先する。
	if !isRule || row.FollowEventRelay == d.followEventRelay() {
		return nil
	}
	err := q.SetReservationFollowEventRelay(ctx, sqlc.SetReservationFollowEventRelayParams{
		ID: row.ID, FollowEventRelay: d.followEventRelay(), Now: now,
	})
	if err != nil {
		return fmt.Errorf("failed to update reservation %s: %w", row.ID, err)
	}

	return nil
}

// link は、予約と一致したルールの関係を want にそろえる。
func (e *Engine) link(ctx context.Context, q *sqlc.Queries, reservationID uuid.UUID, have, want map[uuid.UUID]bool, result *Result) error {
	for _, ruleID := range slices.SortedFunc(maps.Keys(want), compareUUIDs) {
		if have[ruleID] {
			continue
		}
		if err := q.InsertReservationRule(ctx, sqlc.InsertReservationRuleParams{ReservationID: reservationID, RuleID: ruleID}); err != nil {
			return fmt.Errorf("failed to link reservation %s to rule %s: %w", reservationID, ruleID, err)
		}
		result.Linked++
	}
	for _, ruleID := range slices.SortedFunc(maps.Keys(have), compareUUIDs) {
		// 照合から外したルールとの関係は、外さない (#529)。
		if _, skipped := result.Skipped[ruleID]; want[ruleID] || skipped {
			continue
		}
		if err := q.DeleteReservationRule(ctx, sqlc.DeleteReservationRuleParams{ReservationID: reservationID, RuleID: ruleID}); err != nil {
			return fmt.Errorf("failed to unlink reservation %s from rule %s: %w", reservationID, ruleID, err)
		}
		result.Unlinked++
	}

	return nil
}

func compareUUIDs(a, b uuid.UUID) int {
	return slices.Compare(a[:], b[:])
}

// linksAny は、予約が、ruleIDs のどれかと結び付いているかを返す。
func linksAny[V any](have map[uuid.UUID]bool, ruleIDs map[uuid.UUID]V) bool {
	for id := range ruleIDs {
		if have[id] {
			return true
		}
	}

	return false
}

// keepsUnmatched は、どのルールにも一致しなくなったルール予約を、消さずに残すかどうかを返す。
func keepsUnmatched(
	row *sqlc.ListFutureProgramReservationsRow, have map[uuid.UUID]bool, keep *keepPolicy, result *Result,
) (bool, error) {
	switch {
	case row.Recording:
		// 録画が始まった予約は、消すと録画を取り消してしまう (#304)。
		return true, nil
	case linksAny(have, result.Skipped):
		// 照合から外したルールに結び付いた予約は、一致しなくなったとはみなさない (#529)。
		return true, nil
	case row.Enabled && !row.Listed && keep.replaced(row):
		// 番組が EPG から消え、同じ枠に、ルールに一致する別のイベントの番組が出た。局が番組を出し直して差し替えたので、
		// 残すと同じ放送を 2 本録る (#1875)。無効の予約は録らず、取り消しの記録 (#1504) にもなるので残す。
		return false, nil
	case row.Enabled && keep.manuallyReserved[programKey{serviceID: row.ServiceID, eventID: *row.EventID}]:
		// 番組の全体を、EPG から消えた番組の手動の予約が録る。残すと同じ放送を 2 本録るので消す (#1903)。
		return false, nil
	case !row.Enabled && len(have) > 0:
		// ユーザーが無効にした予約は、ルールがある間は残す。消すと、また一致したときに有効な予約として作り直してしまう (#518)。
		// ルールを消すと関係も消えるので、そのときは消す。
		return true, nil
	case !row.Listed:
		// 番組が EPG から消えた。一時的なことがあるので、予約に残した番組の情報で、結び付いたルールに照合し直し、
		// 一致すれば残す (#518)。人がルールを書き換えたか、消したか無効にして外れた予約は消す。
		return keep.matchesStored(row, have)
	case !row.Decided && len(have) > 0:
		// 終了時刻が未定になった。延長などの一時的なことで、消すと録り逃す (#518)。
		// ルールを消して関係もなくなったときは、消したルールの予約で録画しないよう、消す。
		return true, nil
	default:
		return false, nil
	}
}
