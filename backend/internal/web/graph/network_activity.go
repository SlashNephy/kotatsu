package graph

import (
	"context"
	"fmt"
	"time"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// maxNetworkActivityEntries は、画面に返す行の上限。保持期間の中でも、新しい時間帯から返す。
const maxNetworkActivityEntries = 2000

// networkActivity は、外部との通信の記録を返す (#1369)。ログイン中のユーザーなら誰でも見られる。
func (r *Resolver) networkActivity(ctx context.Context) (*model.NetworkActivity, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	system, err := settings.Load(ctx, r.db)
	if err != nil {
		return nil, err
	}
	since := r.clock.Now().Add(-time.Duration(system.NetworkActivityRetentionDays) * 24 * time.Hour).Truncate(time.Hour)
	q := query.New(r.db)
	rows, err := q.ListNetworkActivity(ctx, since, maxNetworkActivityEntries)
	if err != nil {
		return nil, fmt.Errorf("failed to list the network activity: %w", err)
	}
	totals, err := q.SumNetworkActivity(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to sum the network activity: %w", err)
	}

	entries := make([]*model.NetworkActivityEntry, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		var host *string
		if row.Host != "" {
			host = &row.Host
		}
		entries = append(entries, &model.NetworkActivityEntry{
			Hour: row.Hour, Purpose: model.NetworkActivityPurpose(row.Purpose), Host: host, Initiator: model.NetworkActivityInitiator(row.Initiator),
			Success: row.Success, Count: row.Count, SentBytes: row.SentBytes, ReceivedBytes: row.ReceivedBytes,
		})
	}

	return &model.NetworkActivity{
		Enabled: system.NetworkActivityEnabled, EnabledAt: system.NetworkActivityEnabledAt, RetentionDays: system.NetworkActivityRetentionDays,
		TotalCount: totals.Count, TotalSentBytes: totals.SentBytes, TotalReceivedBytes: totals.ReceivedBytes, Entries: entries,
	}, nil
}
