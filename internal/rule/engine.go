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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/epg"
	"github.com/SlashNephy/kotatsu/internal/match"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
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

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
type Guard func(ctx context.Context, tx pgx.Tx) error

// Engine は、ルールの照合を行う (設計書 4.3)。
//
// 全ルールについて「予約すべき番組の集合」を計算し、既存の予約と突き合わせる。何度実行しても同じ結果になる。
//
//   - 集合にあって予約がない番組には、ルール予約を作る。
//   - 予約はあるのに、どのルールにも一致しなくなった番組は、放送が始まっていなければルール予約を消す。
//     無効にしたルール予約も、一致しなくなれば消す。一致している間は、無効のまま残すので作り直されない。
//   - 手動の番組予約がある番組は、手動の設定を優先する。一致したルールの関係だけを残す。
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
	result := &Result{Created: 0, Deleted: 0, Linked: 0, Unlinked: 0, Skipped: map[uuid.UUID]error{}}
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		if err := e.guard(ctx, tx); err != nil {
			return err
		}
		now := e.clock.Now()
		desired, err := e.desired(ctx, tx, now, result, nil)
		if err != nil {
			return err
		}

		return e.apply(ctx, tx, now, desired, result)
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
	rows, err := sqlc.New(tx).ListMatchablePrograms(ctx, sqlc.ListMatchableProgramsParams{Now: now, MediaServiceTypes: epg.MediaServiceTypes()})
	if err != nil {
		return nil, fmt.Errorf("failed to list programs: %w", err)
	}

	matched, err := matchPrograms(rows, rules)
	if err != nil {
		return nil, err
	}

	if err := e.avoidDuplicates(ctx, tx, now, matched, skipped); err != nil {
		return nil, err
	}
	pickSharedEvents(matched)

	return matched, nil
}

// matchPrograms は、番組ごとに、一致したルールを rules の順に集める。
// 必ず含む文字列を決められない正規表現は、前段で絞り込めず、番組ごとに照合するので重い (#878)。
// そこで、番組を GOMAXPROCS の数のまとまりに分け、並べて照合する。GOMAXPROCS は、コンテナの CPU の制限にも従う。
func matchPrograms(rows []sqlc.ListMatchableProgramsRow, rules []*compiledRule) (map[programKey]*desiredProgram, error) {
	workers := max(1, min(runtime.GOMAXPROCS(0), len(rows)))
	parts := make([]map[programKey]*desiredProgram, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			part := map[programKey]*desiredProgram{}
			for i := w; i < len(rows); i += workers {
				row := rows[i]
				program, err := toMatchProgram(row)
				if err != nil {
					errs[w] = err

					return
				}
				for _, r := range rules {
					if !r.covers(row.StartAt) || !r.matcher.Match(program) {
						continue
					}
					key := programKey{serviceID: row.ServiceID, eventID: row.EventID}
					d, ok := part[key]
					if !ok {
						d = &desiredProgram{row: row, rules: []*compiledRule{}}
						part[key] = d
					}
					d.rules = append(d.rules, r)
				}
			}
			parts[w] = part
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	matched := map[programKey]*desiredProgram{}
	for _, part := range parts {
		maps.Copy(matched, part)
	}

	return matched, nil
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
		// 条件を読めないルール (ローリング更新の途中で、新しい版が足した条件など) は飛ばし、ほかのルールの照合を続ける (#529)。
		r, err := fromRow(row)
		if err != nil {
			result.Skipped[row.ID] = err

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
	upstreamGenres := []*mirakurun.Genre{}
	if err := json.Unmarshal(row.Genres, &upstreamGenres); err != nil {
		return nil, fmt.Errorf("failed to decode the genres of program %d/%d: %w", row.ServiceID, row.EventID, err)
	}
	genres := make([]*match.Genre, 0, len(upstreamGenres))
	for _, g := range upstreamGenres {
		genres = append(genres, &match.Genre{Lv1: g.Lv1, Lv2: new(g.Lv2)})
	}
	// 番組ごとに NFKC で正規化し直すと重いので、同期で正規化して持った列を使う (#530)。
	extended := row.ExtendedNormalized
	if extended == nil {
		// 列を足す前に書いた番組は、上流の詳細から正規化する。
		upstream := map[string]string{}
		if err := json.Unmarshal(row.Extended, &upstream); err != nil {
			return nil, fmt.Errorf("failed to decode the extended of program %d/%d: %w", row.ServiceID, row.EventID, err)
		}
		extended = new(epg.NormalizeExtended(upstream))
	}

	return &match.Program{
		ServiceID:   row.ServiceID,
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
		Marks: epg.Marks(row.Name),
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
				skipped[key] = &skippedProgram{row: d.row, rules: removed, lastRecorded: last}
			}
		}
	}

	return nil
}

// pickSharedEvents は、同じイベントを複数のサービスで放送している番組 (イベント共有) が複数一致したとき、
// サービス ID の最も小さい 1 つだけを残す (設計書 4.4)。
func pickSharedEvents(matched map[programKey]*desiredProgram) {
	for _, key := range slices.SortedFunc(maps.Keys(matched), compareKeys) {
		d, ok := matched[key]
		if !ok {
			continue
		}
		var related []*mirakurun.RelatedItem
		if err := json.Unmarshal(d.row.RelatedItems, &related); err != nil {
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

// apply は、予約すべき集合と既存の予約を突き合わせる。
func (e *Engine) apply(ctx context.Context, tx pgx.Tx, now time.Time, desired map[programKey]*desiredProgram, result *Result) error {
	q := sqlc.New(tx)
	existing, err := q.ListFutureProgramReservations(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list reservations: %w", err)
	}
	linkRows, err := q.ListFutureReservationRules(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to list reservation rules: %w", err)
	}
	links := map[uuid.UUID]map[uuid.UUID]bool{}
	for _, row := range linkRows {
		if links[row.ReservationID] == nil {
			links[row.ReservationID] = map[uuid.UUID]bool{}
		}
		links[row.ReservationID][row.RuleID] = true
	}

	reserved := map[programKey]bool{}
	for _, row := range existing {
		key := programKey{serviceID: row.ServiceID, eventID: *row.EventID}
		reserved[key] = true
		if err := e.reconcile(ctx, tx, now, row, desired[key], links[row.ID], result); err != nil {
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

// reconcile は、既存の予約 1 件を、予約すべき集合に合わせる。d は、その番組が集合になければ nil。
func (e *Engine) reconcile(
	ctx context.Context,
	tx pgx.Tx,
	now time.Time,
	row sqlc.ListFutureProgramReservationsRow,
	d *desiredProgram,
	have map[uuid.UUID]bool,
	result *Result,
) error {
	q := sqlc.New(tx)
	isRule := reserve.Kind(row.Kind) == reserve.KindRule
	switch {
	case d == nil && isRule && keepsUnmatched(&row, have, result):
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
func keepsUnmatched(row *sqlc.ListFutureProgramReservationsRow, have map[uuid.UUID]bool, result *Result) bool {
	switch {
	case row.Recording:
		// 録画が始まった予約は、消すと録画を取り消してしまう (#304)。
		return true
	case linksAny(have, result.Skipped):
		// 照合から外したルールに結び付いた予約は、一致しなくなったとはみなさない (#529)。
		return true
	case !row.Decided && len(have) > 0:
		// 番組が EPG から消えたか、終了時刻が未定になった。どちらも一時的なことがあり、消すと録り逃す (#518)。
		// ルールを消して関係もなくなったときは、消したルールの予約で録画しないよう、消す。
		return true
	case !row.Enabled && len(have) > 0:
		// ユーザーが無効にした予約は、ルールがある間は残す。消すと、また一致したときに有効な予約として作り直してしまう (#518)。
		// ルールを消すと関係も消えるので、そのときは消す。
		return true
	default:
		return false
	}
}
