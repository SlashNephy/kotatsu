package anime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/syobocal"
)

const (
	// maxBroadcastTitles は、1 回のジョブで引く TID の数の上限。しょぼいカレンダーへの問い合わせを散らす。
	maxBroadcastTitles = 20
	// broadcastRefreshInterval は、同じ TID を引き直す間隔。broadcastRefreshWindow は、録画を終えてから引き直し続ける期間。
	// 同じ話は、録画の後の 1 週間ほどで別の局でも放送されることが多い。
	broadcastRefreshInterval = 24 * time.Hour
	broadcastRefreshWindow   = 14 * 24 * time.Hour
)

// CollectBroadcasts は、録画に結びついた作品の、ほかの放送を引いて DB に持つ (#1361、設計書 3.8)。
// 別の放送の実況の過去ログを、録画の再生で流すのに使う。1 つの TID で失敗しても、ほかの TID は引く。
func (m *Matcher) CollectBroadcasts(ctx context.Context) (*MatchResult, error) {
	result := new(MatchResult)
	enabled, err := m.enabled(ctx)
	if err != nil {
		return nil, err
	}
	if !enabled {
		result.Disabled = true

		return result, nil
	}
	now := m.clock.Now()
	titles, err := sqlc.New(m.db).ListTitlesForBroadcastLookup(ctx, sqlc.ListTitlesForBroadcastLookupParams{
		FetchedBefore: now.Add(-broadcastRefreshInterval), RecordedSince: now.Add(-broadcastRefreshWindow), MaxTitles: maxBroadcastTitles,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the titles to look up: %w", err)
	}
	var failures error
	for _, tid := range titles {
		result.Requests++
		if err := m.collectBroadcast(ctx, tid, now); err != nil {
			result.Failed++
			failures = errors.Join(failures, fmt.Errorf("failed to look up the broadcasts of TID %d: %w", tid, err))
		}
	}
	// すべて失敗したときだけ、ジョブを失敗にする。次の機会に、引けなかった TID から引き直す。
	if result.Failed > 0 && result.Failed == result.Requests {
		return nil, failures
	}

	return result, nil
}

// collectBroadcast は、作品 tid の放送を引き、持っている放送を入れ替える。
func (m *Matcher) collectBroadcast(ctx context.Context, tid int32, now time.Time) error {
	programs, err := m.client.ProgLookupByTID(ctx, tid)
	if err != nil {
		return err
	}
	if len(programs) >= maxProgLookupItems {
		return fmt.Errorf("%w: ProgLookup returned %d items, which may be truncated", syobocal.ErrUnexpectedResponse, len(programs))
	}

	return pgx.BeginFunc(ctx, m.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if err := q.DeleteAnimeEpisodeBroadcasts(ctx, tid); err != nil {
			return fmt.Errorf("failed to delete the broadcasts: %w", err)
		}
		for _, program := range programs {
			count, err := strconv.ParseFloat(program.Count, 64)
			if program.Deleted || err != nil {
				continue
			}
			if err := q.InsertAnimeEpisodeBroadcast(ctx, sqlc.InsertAnimeEpisodeBroadcastParams{
				Pid: program.PID, Tid: tid, ChannelID: program.ChannelID, StartAt: program.StartAt, EndAt: program.EndAt,
				OffsetSeconds: program.OffsetSeconds, EpisodeCount: count,
			}); err != nil {
				return fmt.Errorf("failed to save a broadcast: %w", err)
			}
		}
		if err := q.UpsertAnimeTitleBroadcastLookup(ctx, sqlc.UpsertAnimeTitleBroadcastLookupParams{Tid: tid, Now: now}); err != nil {
			return fmt.Errorf("failed to save the lookup: %w", err)
		}

		return nil
	})
}
