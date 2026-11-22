package lineup

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// serviceKey は、ネットワークとサービスの組。
type serviceKey struct {
	networkID uint16
	serviceID uint16
}

// activity は、1 つのサービスの、EIT[p/f] の放送中と次の番組のセクションで読んだ番組の有無。
type activity struct {
	transportStreamID uint16
	presentSeen       bool
	followingSeen     bool
	presentEvent      bool
	followingEvent    bool
}

// hasEvents は、放送中か次の番組のどちらかが載っていたかを返す。両方のセクションを読んでいなければ、ok を false にする。
func (a *activity) hasEvents() (bool, bool) {
	if !a.presentSeen || !a.followingSeen {
		return false, false
	}

	return a.presentEvent || a.followingEvent, true
}

// writtenActivity は、書いた番組の有無と、書いた時刻。
type writtenActivity struct {
	hasEvents bool
	at        time.Time
}

// pendingActivity は、まだ書いていない番組の有無。
type pendingActivity struct {
	transportStreamID uint16
	hasEvents         bool
	observedAt        time.Time
}

// ActivityCollector は、録画とタイムシフトの TS の EIT[p/f] (自 TS) から、衛星のサービスの番組の有無を集め、Flush で DB に書く (#2034)。
// NIT のサービスリスト記述子と SDT には載っていても、放送していないサービスがある。チューナーサーバーは、それを PAT に載っていないので
// 登録しない。録画が受けるサービスのストリームの PAT は、そのサービスだけに書き換えられているので、PAT ではほかのサービスを見分けられない。
// EIT[p/f] (自 TS) には、同じトランスポートストリームのほかのサービスも載るので、番組のないサービスを見分けるのに使う。
// 同じ値は、rewriteMinGap に 1 回だけ書き直す (読んだ時刻を新しくする)。値が変わったら、すぐに書く。
type ActivityCollector struct {
	clock  clock.Clock
	logger *slog.Logger

	mu       sync.Mutex
	services map[serviceKey]*activity
	pending  map[serviceKey]pendingActivity
	written  map[serviceKey]writtenActivity
}

// NewActivityCollector は ActivityCollector を返す。
func NewActivityCollector(clk clock.Clock, logger *slog.Logger) *ActivityCollector {
	return &ActivityCollector{
		clock: clk, logger: logger, mu: sync.Mutex{},
		services: map[serviceKey]*activity{}, pending: map[serviceKey]pendingActivity{}, written: map[serviceKey]writtenActivity{},
	}
}

// Observe は、解析器が読んだ EIT[p/f] のセクションを受け取る。ts.AnalyzerOptions.OnPresentFollowing に渡す。
// 解析器の中で呼ばれるので、DB には書かない。衛星のネットワークのサービスだけを使う。
func (c *ActivityCollector) Observe(section *ts.PresentFollowingSection) {
	if section == nil {
		return
	}
	if _, ok := KindOf(section.OriginalNetworkID); !ok {
		return
	}
	key := serviceKey{networkID: section.OriginalNetworkID, serviceID: section.ServiceID}
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.services[key]
	if state == nil || state.transportStreamID != section.TransportStreamID {
		state = &activity{
			transportStreamID: section.TransportStreamID, presentSeen: false, followingSeen: false, presentEvent: false, followingEvent: false,
		}
		c.services[key] = state
	}
	if section.Following {
		state.followingSeen, state.followingEvent = true, section.HasEvent
	} else {
		state.presentSeen, state.presentEvent = true, section.HasEvent
	}
	hasEvents, ok := state.hasEvents()
	if !ok {
		return
	}
	if w, ok := c.written[key]; ok && w.hasEvents == hasEvents && now.Sub(w.at) < rewriteMinGap {
		return
	}
	c.pending[key] = pendingActivity{transportStreamID: state.transportStreamID, hasEvents: hasEvents, observedAt: now}
}

// Flush は、集めた番組の有無を DB に書く。書けなかったものは、次の Flush で書き直す。
// 知らせるためだけのものなので、書けなくても録画は止めない。エラーはログに残すだけにする。
func (c *ActivityCollector) Flush(ctx context.Context, q *sqlc.Queries) {
	c.mu.Lock()
	pending := c.pending
	if len(pending) == 0 {
		c.mu.Unlock()

		return
	}
	c.pending = map[serviceKey]pendingActivity{}
	c.mu.Unlock()

	for key, value := range pending {
		err := q.UpsertBroadcastServiceActivity(ctx, sqlc.UpsertBroadcastServiceActivityParams{
			NetworkID: int32(key.networkID), ServiceID: int32(key.serviceID), TransportStreamID: int32(value.transportStreamID),
			HasEvents: value.hasEvents, ObservedAt: value.observedAt,
		})
		c.mu.Lock()
		if err == nil {
			c.written[key] = writtenActivity{hasEvents: value.hasEvents, at: value.observedAt}
		} else if _, newer := c.pending[key]; !newer {
			c.pending[key] = value
		}
		c.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			c.logger.WarnContext(ctx, "failed to save the service activity",
				slog.Int("network_id", int(key.networkID)), slog.Int("service_id", int(key.serviceID)), slog.Any("error", err))
		}
	}
}

// LoadIdle は、DB に書いた、最後に読んだとき番組のなかった衛星のサービスを返す。読めない行は飛ばす。
func LoadIdle(ctx context.Context, q *sqlc.Queries) (map[Known]bool, error) {
	rows, err := q.ListIdleBroadcastServices(ctx)
	if err != nil {
		return nil, err
	}
	idle := make(map[Known]bool, len(rows))
	for _, row := range rows {
		if row.NetworkID < 0 || row.NetworkID > math.MaxUint16 || row.ServiceID < 0 || row.ServiceID > math.MaxUint16 {
			continue
		}
		idle[Known{NetworkID: uint16(row.NetworkID), ServiceID: uint16(row.ServiceID)}] = true
	}

	return idle, nil
}
