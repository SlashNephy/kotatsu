package kyusu

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// LastKnownInstances は、Instances を包み、DB に届かない間は、最後に引けたインスタンスの一覧を返す (#1084)。
// HLS はセグメントを 2 秒ほどごとに取るので、登録簿を引けないと、DB が止まってすぐに視聴が止まる。
type LastKnownInstances struct {
	instances Instances
	clock     clock.Clock
	logger    *slog.Logger

	mu    sync.Mutex
	known map[string][]*registry.Instance
	// unreachableAt は、最後に DB に届かずに覚えた一覧を返した時刻。DB に届くようになった直後の扱いに使う。
	unreachableAt time.Time
	degraded      bool

	used atomic.Int64
}

var _ Instances = new(LastKnownInstances)

// NewLastKnownInstances は、instances を包む LastKnownInstances を返す。
func NewLastKnownInstances(instances Instances, clk clock.Clock, logger *slog.Logger) *LastKnownInstances {
	return &LastKnownInstances{
		instances: instances, clock: clk, logger: logger,
		mu: sync.Mutex{}, known: map[string][]*registry.Instance{}, unreachableAt: time.Time{}, degraded: false,
		used: atomic.Int64{},
	}
}

// ListAlive は、role のインスタンスを引く。DB に届かなければ、最後に引けた一覧を返す。
// DB に届くようになった直後は、止まっていた間にハートビートを書けなかったインスタンスが、まだ止まったように見える。
// そこで、DB に届かなかったときから registry.AliveTTL の間は、引けた一覧が空なら、最後に引けた一覧を返す。
func (l *LastKnownInstances) ListAlive(ctx context.Context, role string) ([]*registry.Instance, error) {
	// 詰まった DB を、リクエストのたびに待たないよう、届かないと分かってからしばらくは引き直さない。
	l.mu.Lock()
	known, ok := l.known[role]
	retrying := l.degraded && l.clock.Now().Sub(l.unreachableAt) < database.FallbackRetryAfter
	l.mu.Unlock()
	if retrying && ok {
		l.used.Add(1)

		return known, nil
	}

	queryCtx, cancel := context.WithTimeout(ctx, database.FallbackQueryTimeout)
	instances, err := l.instances.ListAlive(queryCtx, role)
	cancel()

	l.mu.Lock()
	now := l.clock.Now()
	known, ok = l.known[role]
	switch {
	case err == nil && len(instances) == 0 && ok && now.Sub(l.unreachableAt) < registry.AliveTTL:
		l.mu.Unlock()
		l.used.Add(1)

		return known, nil
	case err == nil:
		l.known[role] = instances
		recovered := l.degraded
		l.degraded = false
		l.mu.Unlock()
		if recovered {
			l.logger.InfoContext(ctx, "the database is reachable again; stopped relaying with the last known kyusu instances")
		}

		return instances, nil
	case ctx.Err() == nil && database.IsUnavailable(err) && ok:
		l.unreachableAt = now
		first := !l.degraded
		l.degraded = true
		l.mu.Unlock()
		l.used.Add(1)
		if first {
			l.logger.WarnContext(ctx, "the database is unreachable; relaying with the last known kyusu instances", slog.Any("error", err))
		}

		return known, nil
	default:
		l.mu.Unlock()

		return nil, err
	}
}

// FallbackUsed は、DB に届かずに、最後に引けた一覧を返した回数を返す。
func (l *LastKnownInstances) FallbackUsed() int64 {
	return l.used.Load()
}
