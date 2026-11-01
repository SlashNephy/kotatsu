package rule

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
)

// compiledWatchRule は、照合に使う視聴ルール。
type compiledWatchRule struct {
	*watch.Rule

	matcher *match.Matcher
}

// watchProgram は、視聴ルールに一致した番組と、一致した視聴ルール。
type watchProgram struct {
	row   sqlc.ListMatchableProgramsRow
	rules []*compiledWatchRule
}

// enabledWatchRules は、すべてのユーザーの有効な視聴ルールを、ユーザーごとに作った順に返す。
// 条件を読めない視聴ルールは照合から外し、そのルールの持ち主を、2 つ目の戻り値に入れる。
func (e *Engine) enabledWatchRules(ctx context.Context, tx pgx.Tx, result *Result) ([]*compiledWatchRule, map[uuid.UUID]bool, error) {
	rows, err := sqlc.New(tx).ListEnabledWatchRules(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list watch rules: %w", err)
	}

	rules := []*compiledWatchRule{}
	skippedUsers := map[uuid.UUID]bool{}
	for _, row := range rows {
		// 録画のルールと同じく、条件を読めない視聴ルールは飛ばし、ほかの視聴ルールの照合を続ける (#529)。
		r, err := watch.DecodeRule(row)
		if err != nil {
			result.WatchSkipped[row.ID] = err
			skippedUsers[row.UserID] = true

			continue
		}
		matcher, err := match.Compile(r.Conditions, e.location)
		if err != nil {
			result.WatchSkipped[row.ID] = err
			skippedUsers[row.UserID] = true

			continue
		}
		rules = append(rules, &compiledWatchRule{Rule: r, matcher: matcher})
	}

	return rules, skippedUsers, nil
}

// applyWatch は、視聴ルールに一致した番組と、視聴ルールが作った視聴予約を、ユーザーごとに突き合わせる (設計書 4.8)。
//
//   - 一致した番組に視聴予約がなければ、作る。手で外した番組と、通知を送る時刻を過ぎた番組には作らない。
//   - 視聴ルールが作った視聴予約のうち、そのユーザーのどの有効な視聴ルールにも一致しなくなったものは、放送の前なら消す。
//     手で付けた視聴予約と、視聴ルールを消して手で付けたものと同じになった視聴予約は消さない。
//   - 視聴予約を付け外ししたユーザーには、ユーザーの設定の変更として、ほかの端末に知らせる。
//
// rows は、照合の対象にした番組。
func (e *Engine) applyWatch(
	ctx context.Context,
	tx pgx.Tx,
	now time.Time,
	rows []sqlc.ListMatchableProgramsRow,
	matched map[programKey]*watchProgram,
	skippedUsers map[uuid.UUID]bool,
	result *Result,
) error {
	q := sqlc.New(tx)
	byUser := watchProgramsByUser(matched)
	changed := map[uuid.UUID]bool{}

	for _, userID := range slices.SortedFunc(maps.Keys(byUser), compareUUIDs) {
		programs := byUser[userID]
		keys := slices.SortedFunc(maps.Keys(programs), compareKeys)
		params := sqlc.InsertRuleWatchReservationsParams{
			// 照合に時間がかかる間に、通知のジョブが通知の時刻を過ぎた視聴予約を通知していることがある。照合を始めた時刻ではなく、
			// 付ける直前の時刻で、通知の時刻を過ぎたかを確かめる。
			UserID: userID, Now: e.clock.Now(),
			ServiceIds: make([]int64, 0, len(keys)), EventIds: make([]int64, 0, len(keys)), RuleIds: make([]uuid.UUID, 0, len(keys)),
		}
		for _, key := range keys {
			params.ServiceIds = append(params.ServiceIds, key.serviceID)
			params.EventIds = append(params.EventIds, key.eventID)
			// 複数の視聴ルールに一致したときは、最も古く作った視聴ルールが作ったものとする。
			params.RuleIds = append(params.RuleIds, programs[key].rules[0].ID)
		}
		created, err := q.InsertRuleWatchReservations(ctx, params)
		if err != nil {
			return fmt.Errorf("failed to create the watch reservations of user %s: %w", userID, err)
		}
		if created > 0 {
			result.WatchCreated += int(created)
			changed[userID] = true
		}
	}

	deleted, err := e.removeUnmatchedWatch(ctx, q, now, rows, byUser, skippedUsers, result)
	if err != nil {
		return err
	}
	for userID := range deleted {
		changed[userID] = true
	}
	for _, userID := range slices.SortedFunc(maps.Keys(changed), compareUUIDs) {
		if err := usersettings.Touch(ctx, tx, e.clock, userID); err != nil {
			return err
		}
	}

	// 番組が programs からなくなれば、視聴ルールが付け直すことはないので、手で外した記録も要らない。
	pruned, err := q.PruneWatchRuleDismissals(ctx)
	if err != nil {
		return fmt.Errorf("failed to prune the watch rule dismissals: %w", err)
	}
	result.DismissalsPruned = int(pruned)

	return nil
}

// removeUnmatchedWatch は、視聴ルールが作った視聴予約のうち、どの有効な視聴ルールにも一致しなくなったものを消す。
// 視聴予約を消したユーザーを返す。
func (e *Engine) removeUnmatchedWatch(
	ctx context.Context,
	q *sqlc.Queries,
	now time.Time,
	rows []sqlc.ListMatchableProgramsRow,
	byUser map[uuid.UUID]map[programKey]*watchProgram,
	skippedUsers map[uuid.UUID]bool,
	result *Result,
) (map[uuid.UUID]bool, error) {
	matchable := make(map[programKey]bool, len(rows))
	for _, row := range rows {
		matchable[programKey{serviceID: row.ServiceID, eventID: row.EventID}] = true
	}
	existing, err := q.ListRuleWatchReservations(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watch reservations made by watch rules: %w", err)
	}

	unmatched := map[uuid.UUID][]programKey{}
	for _, row := range existing {
		key := programKey{serviceID: row.ServiceID, eventID: row.EventID}
		switch {
		case skippedUsers[row.UserID]:
			// 条件を読めない視聴ルールがあるユーザーの視聴予約は、そのルールに一致しているかもしれないので消さない (#529)。
			continue
		case !matchable[key]:
			// 終了時刻が未定になった番組は、一時的なことがあるので、一致しなくなったとはみなさない (#518)。
			continue
		case byUser[row.UserID][key] != nil:
			continue
		}
		unmatched[row.UserID] = append(unmatched[row.UserID], key)
	}

	deleted := map[uuid.UUID]bool{}
	for _, userID := range slices.SortedFunc(maps.Keys(unmatched), compareUUIDs) {
		keys := unmatched[userID]
		params := sqlc.DeleteRuleWatchReservationsParams{
			UserID: userID, ServiceIds: make([]int64, 0, len(keys)), EventIds: make([]int64, 0, len(keys)),
		}
		for _, key := range keys {
			params.ServiceIds = append(params.ServiceIds, key.serviceID)
			params.EventIds = append(params.EventIds, key.eventID)
		}
		n, err := q.DeleteRuleWatchReservations(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("failed to delete the watch reservations of user %s: %w", userID, err)
		}
		if n > 0 {
			result.WatchDeleted += int(n)
			deleted[userID] = true
		}
	}

	return deleted, nil
}

// watchProgramsByUser は、視聴ルールに一致した番組を、視聴ルールの持ち主ごとに分ける。
// 同じイベントを複数のサービスで放送している番組は、録画のルールと同じく、1 つだけを残す。二度通知しないためである。
func watchProgramsByUser(matched map[programKey]*watchProgram) map[uuid.UUID]map[programKey]*watchProgram {
	byUser := map[uuid.UUID]map[programKey]*watchProgram{}
	for _, key := range slices.SortedFunc(maps.Keys(matched), compareKeys) {
		p := matched[key]
		for _, r := range p.rules {
			programs, ok := byUser[r.UserID]
			if !ok {
				programs = map[programKey]*watchProgram{}
				byUser[r.UserID] = programs
			}
			own, ok := programs[key]
			if !ok {
				own = &watchProgram{row: p.row, rules: []*compiledWatchRule{}}
				programs[key] = own
			}
			own.rules = append(own.rules, r)
		}
	}
	for _, programs := range byUser {
		pickSharedEvents(programs, func(p *watchProgram) []byte { return p.row.RelatedItems })
	}

	return byUser
}
