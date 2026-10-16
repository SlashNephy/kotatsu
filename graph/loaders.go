package graph

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/vikstrous/dataloadgen"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

// loaderWait は、同じリクエストの中で、まとめて引くまで待つ時間。
const loaderWait = time.Millisecond

// Loaders は、1 つのリクエストの中で、同じ種類のデータをまとめて引く dataloader (設計書 9.1)。
// N+1 問題を防ぐため、リゾルバは DB を直接引かずに、ここから引く。リクエストごとに作り直す。
type Loaders struct {
	Users        *dataloadgen.Loader[uuid.UUID, *sqlc.User]
	Services     *dataloadgen.Loader[int64, *sqlc.ListGuideServicesRow]
	ChannelTypes *dataloadgen.Loader[string, *sqlc.ChannelType]
	Rules        *dataloadgen.Loader[uuid.UUID, *rule.Rule]
	// RuleIDs は、予約に一致したルールの ID。ルールに一致していない予約では空にする。
	RuleIDs *dataloadgen.Loader[uuid.UUID, []uuid.UUID]
	// UpcomingReservationCounts は、ルールに一致した予約のうち、終わっていないものの件数。
	UpcomingReservationCounts *dataloadgen.Loader[uuid.UUID, int]
	// ProgramReservations は、番組の予約。予約のない番組では nil にする。
	ProgramReservations *dataloadgen.Loader[reserve.ProgramKey, *reserve.Reservation]
}

type loadersKey struct{}

// newLoaders は、db から引く Loaders を作る。
func newLoaders(db sqlc.DBTX, rules *rule.Rules, reservations *reserve.Reservations) *Loaders {
	return &Loaders{
		Users: dataloadgen.NewLoader(func(ctx context.Context, ids []uuid.UUID) ([]*sqlc.User, []error) {
			rows, err := sqlc.New(db).ListUsersByIDs(ctx, ids)
			if err != nil {
				return failAll[*sqlc.User](len(ids), fmt.Errorf("failed to load users: %w", err))
			}
			byID := make(map[uuid.UUID]*sqlc.User, len(rows))
			for i := range rows {
				byID[rows[i].ID] = &rows[i]
			}

			return orderBy(ids, byID, "user")
		}, dataloadgen.WithWait(loaderWait)),
		Services: dataloadgen.NewLoader(func(ctx context.Context, ids []int64) ([]*sqlc.ListGuideServicesRow, []error) {
			rows, err := sqlc.New(db).ListGuideServices(ctx, sqlc.ListGuideServicesParams{IncludeHidden: true, MediaServiceTypes: []int64{}, Ids: ids})
			if err != nil {
				return failAll[*sqlc.ListGuideServicesRow](len(ids), fmt.Errorf("failed to load services: %w", err))
			}
			byID := make(map[int64]*sqlc.ListGuideServicesRow, len(rows))
			for i := range rows {
				byID[rows[i].ID] = &rows[i]
			}

			return orderBy(ids, byID, "service")
		}, dataloadgen.WithWait(loaderWait)),
		ChannelTypes: dataloadgen.NewLoader(func(ctx context.Context, types []string) ([]*sqlc.ChannelType, []error) {
			rows, err := sqlc.New(db).ListChannelTypesByTypes(ctx, types)
			if err != nil {
				return failAll[*sqlc.ChannelType](len(types), fmt.Errorf("failed to load channel types: %w", err))
			}
			byType := make(map[string]*sqlc.ChannelType, len(rows))
			for i := range rows {
				byType[rows[i].Type] = &rows[i]
			}

			return orderBy(types, byType, "channel type")
		}, dataloadgen.WithWait(loaderWait)),
		Rules: dataloadgen.NewLoader(func(ctx context.Context, ids []uuid.UUID) ([]*rule.Rule, []error) {
			byID, err := rules.ListByIDs(ctx, db, ids)
			if err != nil {
				return failAll[*rule.Rule](len(ids), err)
			}

			return orderBy(ids, byID, "rule")
		}, dataloadgen.WithWait(loaderWait)),
		RuleIDs: dataloadgen.NewLoader(func(ctx context.Context, ids []uuid.UUID) ([][]uuid.UUID, []error) {
			byID, err := reservations.RuleIDs(ctx, db, ids)
			if err != nil {
				return failAll[[]uuid.UUID](len(ids), err)
			}
			ruleIDs := make([][]uuid.UUID, 0, len(ids))
			for _, reservationID := range ids {
				ruleIDs = append(ruleIDs, nonNil(byID[reservationID]))
			}

			return ruleIDs, make([]error, len(ids))
		}, dataloadgen.WithWait(loaderWait)),
		UpcomingReservationCounts: dataloadgen.NewLoader(func(ctx context.Context, ids []uuid.UUID) ([]int, []error) {
			byID, err := reservations.CountUpcomingByRules(ctx, db, ids)
			if err != nil {
				return failAll[int](len(ids), err)
			}
			counts := make([]int, 0, len(ids))
			for _, ruleID := range ids {
				// 予約のないルールは、結果にないので 0 になる。
				counts = append(counts, byID[ruleID])
			}

			return counts, make([]error, len(ids))
		}, dataloadgen.WithWait(loaderWait)),
		ProgramReservations: dataloadgen.NewLoader(func(ctx context.Context, keys []reserve.ProgramKey) ([]*reserve.Reservation, []error) {
			byKey, err := reservations.ListByPrograms(ctx, db, keys)
			if err != nil {
				return failAll[*reserve.Reservation](len(keys), err)
			}
			found := make([]*reserve.Reservation, 0, len(keys))
			for _, key := range keys {
				found = append(found, byKey[key])
			}

			return found, make([]error, len(keys))
		}, dataloadgen.WithWait(loaderWait)),
	}
}

// failAll は、まとめて引いたすべてのキーに、同じエラーを返す。
func failAll[V any](n int, err error) ([]V, []error) {
	errs := make([]error, n)
	for i := range errs {
		errs[i] = err
	}

	return make([]V, n), errs
}

// orderBy は、キーの順に値を並べる。見つからないキーには NOT_FOUND のエラーを返す。
func orderBy[K comparable, V any](keys []K, byKey map[K]V, name string) ([]V, []error) {
	values := make([]V, len(keys))
	errs := make([]error, len(keys))
	for i, key := range keys {
		value, ok := byKey[key]
		if !ok {
			errs[i] = NewError(CodeNotFound, name+" not found")

			continue
		}
		values[i] = value
	}

	return values, errs
}

// withLoaders は、リクエストごとに Loaders を作って context に入れるミドルウェアを返す。
func withLoaders(db sqlc.DBTX, rules *rule.Rules, reservations *reserve.Reservations) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), loadersKey{}, newLoaders(db, rules, reservations))))
		})
	}
}

// loadersFrom は、context に入れた Loaders を返す。
func loadersFrom(ctx context.Context) *Loaders {
	loaders, _ := ctx.Value(loadersKey{}).(*Loaders)

	return loaders
}
