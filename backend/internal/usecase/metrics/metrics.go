// Package metrics は、/metrics で公開する Kotatsu のメトリクスを集める (設計書 11.3)。
//
// 録画、予約、チューナー、ジョブのキューは、どのインスタンスからも同じ DB を読むので、どのインスタンスの /metrics も、クラスター全体の同じ値を返す。
// 集計するときは、インスタンスをまたいで足さず、max などで 1 つにまとめる。
// ラベルには、番組名のように視聴の傾向が分かる値を入れない。
package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// Namespace は、Kotatsu のメトリクスの名前の接頭辞。
	Namespace = "kotatsu"
	// collectTimeout は、1 回の収集で DB を読む時間の上限。
	collectTimeout = 5 * time.Second
	// leaseName は、neko のリースの名前。neko を読み込むと循環するので、同じ値を持つ。
	leaseName = "neko"
)

// DBCollector は、DB から、録画、予約、割り振り、チューナー、neko のリース、River のキューの状態を、収集のたびに読む。
type DBCollector struct {
	pool       *pgxpool.Pool
	clock      clock.Clock
	instanceID uuid.UUID
	logger     *slog.Logger

	recordings       *prometheus.Desc
	failures         *prometheus.Desc
	failuresTotal    *prometheus.Desc
	lastFailure      *prometheus.Desc
	activePackets    *prometheus.Desc
	reservations     *prometheus.Desc
	assignments      *prometheus.Desc
	tuners           *prometheus.Desc
	leader           *prometheus.Desc
	jobs             *prometheus.Desc
	collectionErrors *prometheus.Desc
}

var _ prometheus.Collector = new(DBCollector)

// NewDBCollector は DBCollector を返す。instanceID は、このインスタンスの ID で、neko のリースを持っているかの判定に使う。
func NewDBCollector(pool *pgxpool.Pool, clk clock.Clock, instanceID uuid.UUID, logger *slog.Logger) *DBCollector {
	desc := func(name, help string, labels ...string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(Namespace, "", name), help, labels, nil)
	}

	return &DBCollector{
		pool: pool, clock: clk, instanceID: instanceID, logger: logger,
		recordings:       desc("recordings", "Number of recordings by state.", "state"),
		failures:         desc("recording_failures", "Number of failed recordings by failure reason.", "reason"),
		failuresTotal:    desc("recording_failures_total", "Number of recordings that have failed, by failure reason. Deleting recordings does not decrease it.", "reason"),
		lastFailure:      desc("recording_last_failure_timestamp_seconds", "Time when a recording last failed, by failure reason.", "reason"),
		activePackets:    desc("recording_active_packets", "Dropped, errored, and scrambled packets of the recordings in progress.", "kind"),
		reservations:     desc("reservations", "Number of reservations that have not ended, by assignment state, or disabled.", "state"),
		assignments:      desc("recording_assignments", "Number of recording assignments by state.", "state"),
		tuners:           desc("tuners", "Number of tuners of the tuner server by state.", "state"),
		leader:           desc("neko_leader", "Whether this instance holds the neko lease (1) or not (0)."),
		jobs:             desc("river_jobs", "Number of River jobs waiting or running, by queue and state.", "queue", "state"),
		collectionErrors: desc("metrics_collection_errors", "Number of metric groups that could not be read from the database in this scrape."),
	}
}

// Describe は、集めるメトリクスの説明を送る。
func (c *DBCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.recordings, c.failures, c.failuresTotal, c.lastFailure, c.activePackets, c.reservations, c.assignments, c.tuners, c.leader, c.jobs, c.collectionErrors,
	} {
		ch <- d
	}
}

// Collect は、DB を読んでメトリクスを送る。読めなかったグループは送らず、その数を metrics_collection_errors で示す。
func (c *DBCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collectTimeout)
	defer cancel()

	failed := 0
	for _, collect := range []func(context.Context, chan<- prometheus.Metric) error{
		c.collectRecordings, c.collectReservations, c.collectAssignments, c.collectTuners, c.collectLeader, c.collectJobs,
	} {
		if err := collect(ctx, ch); err != nil {
			c.logger.WarnContext(ctx, "failed to collect metrics", slog.Any("error", err))
			failed++
		}
	}
	ch <- prometheus.MustNewConstMetric(c.collectionErrors, prometheus.GaugeValue, float64(failed))
}

func (c *DBCollector) collectRecordings(ctx context.Context, ch chan<- prometheus.Metric) error {
	q := sqlc.New(c.pool)
	states, err := q.CountRecordingsByState(ctx)
	if err != nil {
		return err
	}
	for _, row := range states {
		ch <- prometheus.MustNewConstMetric(c.recordings, prometheus.GaugeValue, float64(row.Count), row.State)
	}
	reasons, err := q.CountRecordingFailures(ctx)
	if err != nil {
		return err
	}
	for _, row := range reasons {
		ch <- prometheus.MustNewConstMetric(c.failures, prometheus.GaugeValue, float64(row.Count), row.Reason)
	}
	// 失敗の累計は、録画を消しても減らない (#960)。一度も失敗していない理由は、累計を 0 で送り、時刻は送らない。
	counts, err := q.ListRecordingFailureCounts(ctx)
	if err != nil {
		return err
	}
	for _, row := range counts {
		ch <- prometheus.MustNewConstMetric(c.failuresTotal, prometheus.CounterValue, float64(row.Count), row.Reason)
		if row.LastFailedAt != nil {
			ch <- prometheus.MustNewConstMetric(c.lastFailure, prometheus.GaugeValue, float64(row.LastFailedAt.Unix()), row.Reason)
		}
	}
	packets, err := q.SumActiveRecordingPackets(ctx)
	if err != nil {
		return err
	}
	for kind, value := range map[string]int64{"drop": packets.Drops, "error": packets.Errors, "scrambled": packets.Scrambled} {
		ch <- prometheus.MustNewConstMetric(c.activePackets, prometheus.GaugeValue, float64(value), kind)
	}

	return nil
}

// collectReservations は、予約を状態ごとに数える。件数が 0 の状態も送り、系列が途切れないようにする。
func (c *DBCollector) collectReservations(ctx context.Context, ch chan<- prometheus.Metric) error {
	row, err := sqlc.New(c.pool).CountReservationsByState(ctx, c.clock.Now())
	if err != nil {
		return err
	}
	for state, value := range map[string]int64{
		"ok": row.Ok, "partial": row.Partial, "conflict": row.Conflict, "pending": row.Pending, "disabled": row.Disabled,
	} {
		ch <- prometheus.MustNewConstMetric(c.reservations, prometheus.GaugeValue, float64(value), state)
	}

	return nil
}

func (c *DBCollector) collectAssignments(ctx context.Context, ch chan<- prometheus.Metric) error {
	rows, err := sqlc.New(c.pool).CountRecordingAssignmentsByState(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		ch <- prometheus.MustNewConstMetric(c.assignments, prometheus.GaugeValue, float64(row.Count), row.State)
	}

	return nil
}

func (c *DBCollector) collectTuners(ctx context.Context, ch chan<- prometheus.Metric) error {
	row, err := sqlc.New(c.pool).CountTunersByState(ctx)
	if err != nil {
		return err
	}
	for state, value := range map[string]int64{"available": row.Available, "unavailable": row.Unavailable, "fault": row.Fault} {
		ch <- prometheus.MustNewConstMetric(c.tuners, prometheus.GaugeValue, float64(value), state)
	}

	return nil
}

func (c *DBCollector) collectLeader(ctx context.Context, ch chan<- prometheus.Metric) error {
	holds, err := sqlc.New(c.pool).HoldsLease(ctx, sqlc.HoldsLeaseParams{Name: leaseName, Holder: c.instanceID, Now: c.clock.Now()})
	if err != nil {
		return err
	}
	value := 0.0
	if holds {
		value = 1
	}
	ch <- prometheus.MustNewConstMetric(c.leader, prometheus.GaugeValue, value)

	return nil
}

// collectJobs は、River のジョブのうち、待っているものと動いているものを数える。River のテーブルは sqlc のスキーマにないので、直接読む。
func (c *DBCollector) collectJobs(ctx context.Context, ch chan<- prometheus.Metric) error {
	rows, err := c.pool.Query(ctx, `SELECT queue, state::text, count(*) FROM river_job
		WHERE state IN ('available', 'scheduled', 'retryable', 'running') GROUP BY queue, state`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var queue, state string
		var count int64
		if err := rows.Scan(&queue, &state, &count); err != nil {
			return err
		}
		ch <- prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue, float64(count), queue, state)
	}

	return rows.Err()
}
