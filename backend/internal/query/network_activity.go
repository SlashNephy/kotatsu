package query

import (
	"context"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// NetworkActivityEntry は、外部との通信の 1 時間ごとの集計の 1 行。
type NetworkActivityEntry = sqlc.NetworkActivity

// NetworkActivityTotals は、外部との通信の合計。
type NetworkActivityTotals = sqlc.SumNetworkActivityRow

// ListNetworkActivity は、since より後の集計を、新しい時間帯から maxCount 件まで返す。
func (q *Queries) ListNetworkActivity(ctx context.Context, since time.Time, maxCount int32) ([]NetworkActivityEntry, error) {
	return sqlc.New(q.pool).ListNetworkActivity(ctx, sqlc.ListNetworkActivityParams{Since: since, MaxCount: maxCount})
}

// SumNetworkActivity は、since より後の集計の合計を返す。
func (q *Queries) SumNetworkActivity(ctx context.Context, since time.Time) (NetworkActivityTotals, error) {
	return sqlc.New(q.pool).SumNetworkActivity(ctx, since)
}
