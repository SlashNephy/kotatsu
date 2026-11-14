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

// 地上波のネットワーク ID の範囲 (ARIB TR-B14)。地上波の SDT[actual] だけを集める。
const (
	terrestrialNetworkMin = 0x7880
	terrestrialNetworkMax = 0x7FE8
)

// isTerrestrial は、networkID が地上波のネットワークかを返す。
func isTerrestrial(networkID uint16) bool {
	return networkID >= terrestrialNetworkMin && networkID <= terrestrialNetworkMax
}

// TerrestrialObservation は、ある時点で読んだ、地上波の 1 つのネットワークの SDT[actual] のサービス。
type TerrestrialObservation struct {
	NetworkID         uint16
	TransportStreamID uint16
	Version           uint8
	// Services は、サービス記述子のあるサービス。
	Services   []Service
	ObservedAt time.Time
}

// partialSDT は、組み上げている途中の SDT[actual] のセクション。
type partialSDT struct {
	version  uint8
	last     uint8
	tsid     uint16
	sections map[uint8][]*ts.SDTService
}

// TerrestrialCollector は、録画とタイムシフトの TS の SDT[actual] から、地上波のネットワークのサービスを集め、DB に書く (#1519)。
// 地上波の NIT には自分の TS のぶんしか載らず、チャンネルのサービスはチューナーサーバーが SDT から作るので、SDT で比べる。
type TerrestrialCollector struct {
	clock  clock.Clock
	logger *slog.Logger

	mu      sync.Mutex
	partial map[uint16]*partialSDT
	// pending は、まだ書いていないサービス。
	pending map[uint16]*TerrestrialObservation
	// written は、書いたサービスの版と、書いた時刻。
	written map[uint16]written
}

// NewTerrestrialCollector は TerrestrialCollector を返す。
func NewTerrestrialCollector(clk clock.Clock, logger *slog.Logger) *TerrestrialCollector {
	return &TerrestrialCollector{
		clock: clk, logger: logger, mu: sync.Mutex{},
		partial: map[uint16]*partialSDT{}, pending: map[uint16]*TerrestrialObservation{}, written: map[uint16]written{},
	}
}

// Observe は、解析器が読んだ SDT を受け取る。ts.AnalyzerOptions.OnSDT に渡す。解析器の中で呼ばれるので、DB には書かない。
// 地上波の SDT[actual] だけを使う。サービス記述子が 1 つもない SDT は使わない。ロゴの対応だけを載せた SDT が先にそろうと、
// 局が全部消えたように見えるからである。
func (c *TerrestrialCollector) Observe(sdt *ts.SDT) {
	if sdt == nil || !sdt.Actual || !isTerrestrial(sdt.OriginalNetworkID) || sdt.SectionNumber > sdt.LastSectionNumber {
		return
	}
	networkID := sdt.OriginalNetworkID
	now := c.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if w, ok := c.written[networkID]; ok && w.version == sdt.Version && now.Sub(w.at) < rewriteMinGap {
		return
	}
	part := c.partial[networkID]
	if part == nil || part.version != sdt.Version || part.last != sdt.LastSectionNumber || part.tsid != sdt.TransportStreamID {
		part = &partialSDT{version: sdt.Version, last: sdt.LastSectionNumber, tsid: sdt.TransportStreamID, sections: map[uint8][]*ts.SDTService{}}
		c.partial[networkID] = part
	}
	part.sections[sdt.SectionNumber] = sdt.Services
	if len(part.sections) < int(part.last)+1 {
		return
	}
	delete(c.partial, networkID)
	services := []Service{}
	for number := range int(part.last) + 1 {
		for _, service := range part.sections[uint8(number)] {
			if service.HasServiceDescriptor {
				services = append(services, Service{ServiceID: service.ServiceID, ServiceType: service.ServiceType})
			}
		}
	}
	if len(services) == 0 {
		return
	}
	c.pending[networkID] = &TerrestrialObservation{
		NetworkID: networkID, TransportStreamID: part.tsid, Version: part.version, Services: services, ObservedAt: now,
	}
}

// Flush は、組み上がったサービスを DB に書く。書けなかったものは、次の Flush で書き直す。
// 知らせるためだけのものなので、書けなくても録画は止めない。エラーはログに残すだけにする。
func (c *TerrestrialCollector) Flush(ctx context.Context, q *sqlc.Queries) {
	c.mu.Lock()
	pending := c.pending
	if len(pending) == 0 {
		c.mu.Unlock()

		return
	}
	c.pending = map[uint16]*TerrestrialObservation{}
	c.mu.Unlock()

	for networkID, obs := range pending {
		services, err := json.Marshal(obs.Services)
		if err == nil {
			err = q.UpsertTerrestrialLineup(ctx, sqlc.UpsertTerrestrialLineupParams{
				NetworkID: int32(networkID), TransportStreamID: int32(obs.TransportStreamID), Version: int16(obs.Version),
				Services: services, ObservedAt: obs.ObservedAt,
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
			c.logger.WarnContext(ctx, "failed to save the terrestrial lineup", slog.Int("network_id", int(networkID)), slog.Any("error", err))
		}
	}
}

// LoadTerrestrial は、DB に書いた地上波のネットワークのサービスを読む。読めない行は飛ばす。
func LoadTerrestrial(ctx context.Context, q *sqlc.Queries) ([]*TerrestrialObservation, error) {
	rows, err := q.ListTerrestrialLineups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*TerrestrialObservation, 0, len(rows))
	for _, row := range rows {
		// 書いたのは uint16 と uint8 の値なので、収まらない行は壊れている。
		if row.NetworkID < 0 || row.NetworkID > math.MaxUint16 || row.TransportStreamID < 0 || row.TransportStreamID > math.MaxUint16 ||
			row.Version < 0 || row.Version > math.MaxUint8 {
			continue
		}
		var services []Service
		if err := json.Unmarshal(row.Services, &services); err != nil {
			continue
		}
		out = append(out, &TerrestrialObservation{
			NetworkID: uint16(row.NetworkID), TransportStreamID: uint16(row.TransportStreamID), Version: uint8(row.Version),
			Services: services, ObservedAt: row.ObservedAt,
		})
	}

	return out, nil
}
