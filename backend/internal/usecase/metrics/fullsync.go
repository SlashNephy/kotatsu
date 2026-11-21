package metrics

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// FullSyncCollector は、neko のリーダーが最後に EPG の全量を突き合わせた時刻と、それから経った秒数を送る (設計書 11.3)。
// 経った秒数も送るのは、今の時刻を返す関数 (PromQL の time()) のない OTLP の送り先でも、同期が止まったことを監視できるようにするためである (#2010)。
// リーダーの値なので、クラスターのレジストリに登録する。元のリーダーが、古い時刻を OTLP で送り続けないようにする (#962)。
type FullSyncCollector struct {
	clock clock.Clock
	// last は、最後に全量を突き合わせた時刻の Unix 秒。まだ突き合わせていなければ 0。
	last      atomic.Int64
	timestamp *prometheus.Desc
	age       *prometheus.Desc
}

var _ prometheus.Collector = new(FullSyncCollector)

// NewFullSyncCollector は FullSyncCollector を返す。
func NewFullSyncCollector(clk clock.Clock) *FullSyncCollector {
	return &FullSyncCollector{
		clock: clk,
		last:  atomic.Int64{},
		timestamp: prometheus.NewDesc(prometheus.BuildFQName(Namespace, "", "epg_last_full_sync_timestamp_seconds"),
			"Unix time when this instance last synced the whole EPG as the neko leader.", nil, nil),
		age: prometheus.NewDesc(prometheus.BuildFQName(Namespace, "", "epg_full_sync_age_seconds"),
			"Seconds since this instance last synced the whole EPG as the neko leader.", nil, nil),
	}
}

// Observe は、全量を突き合わせ終えた時刻を覚える。
func (c *FullSyncCollector) Observe(at time.Time) {
	c.last.Store(at.Unix())
}

// Describe は、集めるメトリクスの説明を送る。
func (c *FullSyncCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.timestamp
	ch <- c.age
}

// Collect は、最後に全量を突き合わせた時刻を送る。まだ突き合わせていなければ、時刻は 0 にし、経った秒数は送らない。
// 経った秒数を 0 にすると、同期していないのに今同期したように見え、今の時刻にすると、リーダーになった直後に大きな遅れに見えるからである。
func (c *FullSyncCollector) Collect(ch chan<- prometheus.Metric) {
	last := c.last.Load()
	ch <- prometheus.MustNewConstMetric(c.timestamp, prometheus.GaugeValue, float64(last))
	if last == 0 {
		return
	}
	age := max(c.clock.Now().Sub(time.Unix(last, 0)).Seconds(), 0)
	ch <- prometheus.MustNewConstMetric(c.age, prometheus.GaugeValue, age)
}
