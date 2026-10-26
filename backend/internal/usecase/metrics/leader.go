package metrics

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// LeaderCollector は、このインスタンスが neko のリースを持っているかを、収集のたびに DB から読む。
// インスタンスごとに違う値なので、DBCollector (クラスター全体の値) とは分けて、インスタンスのレジストリに登録する (#962)。
type LeaderCollector struct {
	pool       *pgxpool.Pool
	clock      clock.Clock
	instanceID uuid.UUID
	logger     *slog.Logger
	leader     *prometheus.Desc
}

var _ prometheus.Collector = new(LeaderCollector)

// NewLeaderCollector は LeaderCollector を返す。instanceID は、このインスタンスの ID。
func NewLeaderCollector(pool *pgxpool.Pool, clk clock.Clock, instanceID uuid.UUID, logger *slog.Logger) *LeaderCollector {
	return &LeaderCollector{
		pool: pool, clock: clk, instanceID: instanceID, logger: logger,
		leader: prometheus.NewDesc(prometheus.BuildFQName(Namespace, "", "neko_leader"),
			"Whether this instance holds the neko lease (1) or not (0).", nil, nil),
	}
}

// Describe は、集めるメトリクスの説明を送る。
func (c *LeaderCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.leader
}

// Collect は、neko のリースを持っているかを DB から読んで送る。読めなければ送らない。
func (c *LeaderCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()

	holds, err := HoldsLeaderLease(ctx, c.pool, c.clock, c.instanceID)
	if err != nil {
		c.logger.WarnContext(ctx, "failed to collect metrics", slog.Any("error", err))

		return
	}
	value := 0.0
	if holds {
		value = 1
	}
	ch <- prometheus.MustNewConstMetric(c.leader, prometheus.GaugeValue, value)
}

// HoldsLeaderLease は、instanceID のインスタンスが、今 neko のリースを持っているかを返す。
// OTLP では、リースを持つインスタンスだけが、クラスター全体の値を送る (#962)。
func HoldsLeaderLease(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, instanceID uuid.UUID) (bool, error) {
	return sqlc.New(pool).HoldsLease(ctx, sqlc.HoldsLeaseParams{Name: leaseName, Holder: instanceID, Now: clk.Now()})
}
