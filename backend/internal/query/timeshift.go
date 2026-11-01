package query

import (
	"context"
	"time"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// ListTimeshiftChannels は、タイムシフトで録るチャンネルを、バッファにある範囲と、録っているかと一緒に返す。
// チャンネル種別の並び順、サービス ID の順に並べる。now は、書いている途中のチャンクの終わりと、リースが切れたかの判定に使う。
func (q *Queries) ListTimeshiftChannels(ctx context.Context, now time.Time) ([]domain.TimeshiftChannel, error) {
	db := sqlc.New(q.pool)
	rows, err := db.ListTimeshiftChannelStatuses(ctx, now)
	if err != nil {
		return nil, err
	}
	ranges, err := db.ListTimeshiftBufferRanges(ctx, now)
	if err != nil {
		return nil, err
	}
	byService := make(map[int64]*sqlc.ListTimeshiftBufferRangesRow, len(ranges))
	for i := range ranges {
		byService[ranges[i].ServiceID] = &ranges[i]
	}
	out := make([]domain.TimeshiftChannel, 0, len(rows))
	for _, row := range rows {
		channel := domain.TimeshiftChannel{
			ServiceID: row.ServiceID, RetentionSeconds: row.RetentionSeconds, BufferedFrom: nil, BufferedUntil: nil, Recording: row.Recording,
		}
		if buffered, ok := byService[row.ServiceID]; ok {
			channel.BufferedFrom, channel.BufferedUntil = &buffered.BufferedFrom, &buffered.BufferedUntil
		}
		out = append(out, channel)
	}

	return out, nil
}
