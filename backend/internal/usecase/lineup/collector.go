package lineup

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// 衛星のネットワークの network_id (ARIB TR-B15)。
const (
	networkIDBS   = 4
	networkIDCS1  = 6
	networkIDCS2  = 7
	rewriteMinGap = 5 * time.Minute
)

// KindOf は、network_id の衛星の種別を返す。衛星でなければ false を返す。
func KindOf(networkID uint16) (Kind, bool) {
	switch networkID {
	case networkIDBS:
		return KindBS, true
	case networkIDCS1, networkIDCS2:
		return KindCS, true
	default:
		return "", false
	}
}

// partialNIT は、組み上がる前の、1 つのネットワークの NIT[actual] のセクション。
type partialNIT struct {
	version  uint8
	last     uint8
	sections map[uint8][]*ts.NITTransportStream
}

// Collector は、TS を読む解析器から受け取った、衛星のネットワークの NIT[actual] を組み上げ、Flush で DB に書く (#1519)。
// NIT は複数のセクションに分かれることがあるので、同じ版の全部のセクションが揃ったら、1 つの構成にする。
// 同じ版の構成は、rewriteMinGap に 1 回だけ書き直す (読んだ時刻を新しくする)。
type Collector struct {
	clock  clock.Clock
	logger *slog.Logger

	mu      sync.Mutex
	partial map[uint16]*partialNIT
	// pending は、まだ書いていない構成。
	pending map[uint16]*Observation
	// written は、書いた構成の版と、書いた時刻。
	written map[uint16]written
}

type written struct {
	version uint8
	at      time.Time
}

// NewCollector は Collector を返す。
func NewCollector(clk clock.Clock, logger *slog.Logger) *Collector {
	return &Collector{
		clock: clk, logger: logger, mu: sync.Mutex{},
		partial: map[uint16]*partialNIT{}, pending: map[uint16]*Observation{}, written: map[uint16]written{},
	}
}

// Observe は、解析器が読んだ NIT を受け取る。ts.AnalyzerOptions.OnNIT に渡す。解析器の中で呼ばれるので、DB には書かない。
// 衛星の NIT[actual] だけを使う。
func (c *Collector) Observe(nit *ts.NIT) {
	if nit == nil {
		return
	}
	kind, ok := KindOf(nit.NetworkID)
	if !ok || !nit.Actual || nit.SectionNumber > nit.LastSectionNumber {
		return
	}
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if w, ok := c.written[nit.NetworkID]; ok && w.version == nit.Version && now.Sub(w.at) < rewriteMinGap {
		return
	}
	part := c.partial[nit.NetworkID]
	if part == nil || part.version != nit.Version || part.last != nit.LastSectionNumber {
		part = &partialNIT{version: nit.Version, last: nit.LastSectionNumber, sections: map[uint8][]*ts.NITTransportStream{}}
		c.partial[nit.NetworkID] = part
	}
	part.sections[nit.SectionNumber] = nit.TransportStreams
	if len(part.sections) < int(part.last)+1 {
		return
	}
	delete(c.partial, nit.NetworkID)
	obs := &Observation{Kind: kind, NetworkID: nit.NetworkID, Version: nit.Version, Streams: []Stream{}, ObservedAt: now}
	for number := range int(part.last) + 1 {
		for _, stream := range part.sections[uint8(number)] {
			services := make([]Service, 0, len(stream.Services))
			for _, service := range stream.Services {
				services = append(services, Service{ServiceID: service.ServiceID, ServiceType: service.ServiceType})
			}
			obs.Streams = append(obs.Streams, Stream{TransportStreamID: stream.TransportStreamID, FrequencyKHz: stream.FrequencyKHz, Services: services})
		}
	}
	c.pending[nit.NetworkID] = obs
}

// Flush は、組み上がった構成を DB に書く。書けなかった構成は、次の Flush で書き直す。
// 知らせるためだけのものなので、書けなくても録画は止めない。エラーはログに残すだけにする。
func (c *Collector) Flush(ctx context.Context, q *sqlc.Queries) {
	c.mu.Lock()
	pending := c.pending
	if len(pending) == 0 {
		c.mu.Unlock()

		return
	}
	c.pending = map[uint16]*Observation{}
	c.mu.Unlock()

	for networkID, obs := range pending {
		streams, err := json.Marshal(obs.Streams)
		if err == nil {
			err = q.UpsertBroadcastLineup(ctx, sqlc.UpsertBroadcastLineupParams{
				NetworkID: int32(networkID), Version: int16(obs.Version), Streams: streams, ObservedAt: obs.ObservedAt,
			})
		}
		c.mu.Lock()
		if err == nil {
			c.written[networkID] = written{version: obs.Version, at: obs.ObservedAt}
		} else if _, newer := c.pending[networkID]; !newer {
			c.pending[networkID] = obs
		}
		c.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			c.logger.WarnContext(ctx, "failed to save the broadcast lineup", slog.Int("network_id", int(networkID)), slog.Any("error", err))
		}
	}
}

// Load は、DB に書いた衛星のネットワークの構成を読む。読めない行は飛ばす。
func Load(ctx context.Context, q *sqlc.Queries) ([]*Observation, error) {
	rows, err := q.ListBroadcastLineups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Observation, 0, len(rows))
	for _, row := range rows {
		// 書いたのは uint16 と uint8 の値なので、収まらない行は壊れている。
		if row.NetworkID < 0 || row.NetworkID > math.MaxUint16 || row.Version < 0 || row.Version > math.MaxUint8 {
			continue
		}
		networkID := uint16(row.NetworkID)
		kind, ok := KindOf(networkID)
		if !ok {
			continue
		}
		var streams []Stream
		if err := json.Unmarshal(row.Streams, &streams); err != nil {
			continue
		}
		out = append(out, &Observation{Kind: kind, NetworkID: networkID, Version: uint8(row.Version), Streams: streams, ObservedAt: row.ObservedAt})
	}

	return out, nil
}
