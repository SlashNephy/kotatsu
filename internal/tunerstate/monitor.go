// Package tunerstate は、チューナーの構成とチューナーサーバーの状態を把握し続ける (設計書 3.6、3.7)。
//
// チューナーの構成は、定期的な取得とイベントで更新し、DB の tuners に持つ。
// チューナーの割り当て (neko) と、システムの状態の画面 (tenban) は、DB から読む。
// チューナーサーバーに接続できない間は縮退中として記録する。縮退は Kotatsu の起動や liveness の判定には影響させない。
package tunerstate

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/notify"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
)

const (
	// NotifyChannel は、チューナーの構成が変わったことを知らせる変更通知のチャンネル。
	NotifyChannel = "tuners"
	// DefaultPollInterval は、チューナーの構成を取得し直す間隔。
	DefaultPollInterval = time.Minute
	// DefaultSkewThreshold は、チューナーサーバーの時計がこれより大きくずれていたら警告する差。
	// 設計書に定めはない。録画のマージンの既定値 (15 秒) を食い潰す前に気付けるよう、10 秒にする。
	DefaultSkewThreshold = 10 * time.Second
	// maxErrorLength は、DB に残す失敗の理由の長さの上限。
	maxErrorLength = 1000
)

// Upstream は、状態の把握に使うチューナーサーバーの API。*mirakurun.Client が満たす。
type Upstream interface {
	Tuners(ctx context.Context) ([]*mirakurun.Tuner, error)
	ServerDate(ctx context.Context) (time.Time, error)
}

var _ Upstream = new(mirakurun.Client)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確かめる。
type Guard func(ctx context.Context, tx pgx.Tx) error

// Options は Monitor の設定。0 の項目は既定値にする。
type Options struct {
	PollInterval  time.Duration
	SkewThreshold time.Duration
}

// Monitor は、チューナーの構成とチューナーサーバーの状態を把握し続ける。
type Monitor struct {
	pool     *pgxpool.Pool
	upstream Upstream
	clock    clock.Clock
	guard    Guard
	logger   *slog.Logger
	opts     Options
	trigger  chan struct{}

	// skewed は、前回の確認で時計が閾値を超えてずれていたかどうか。警告を状態が変わったときだけ出すのに使う。
	skewed bool
}

// NewMonitor は Monitor を作る。opts は nil でもよい。
func NewMonitor(pool *pgxpool.Pool, upstream Upstream, clk clock.Clock, guard Guard, logger *slog.Logger, opts *Options) *Monitor {
	o := Options{PollInterval: DefaultPollInterval, SkewThreshold: DefaultSkewThreshold}
	if opts != nil {
		if opts.PollInterval > 0 {
			o.PollInterval = opts.PollInterval
		}
		if opts.SkewThreshold > 0 {
			o.SkewThreshold = opts.SkewThreshold
		}
	}

	return &Monitor{
		pool:     pool,
		upstream: upstream,
		clock:    clk,
		guard:    guard,
		logger:   logger,
		opts:     o,
		trigger:  make(chan struct{}, 1),
		skewed:   false,
	}
}

// Trigger は、次の取得を待たずに状態を取得し直させる。チューナーのイベントが届いたときに呼ぶ。
func (m *Monitor) Trigger() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// Run は、ctx が終わるまで、PollInterval ごとと Trigger されたときに状態を取得し直す。
func (m *Monitor) Run(ctx context.Context) error {
	ticker := time.NewTicker(m.opts.PollInterval)
	defer ticker.Stop()
	for {
		if err := m.Check(ctx); err != nil && ctx.Err() == nil {
			m.logger.WarnContext(ctx, "failed to record the tuner state", slog.Any("error", err))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-m.trigger:
		}
	}
}

// Check は、チューナーの構成とチューナーサーバーの時計を取得し、DB に記録する。
// チューナーサーバーに接続できなければ縮退中として記録する。エラーを返すのは、DB に記録できなかったときだけである。
func (m *Monitor) Check(ctx context.Context) error {
	tuners, tunersErr := m.upstream.Tuners(ctx)
	date, dateErr := m.upstream.ServerDate(ctx)
	now := m.clock.Now()

	if err := errors.Join(tunersErr, dateErr); err != nil {
		return m.recordDegraded(ctx, now, err)
	}

	skew := date.Sub(now)
	m.warnSkew(ctx, skew)

	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if err := m.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		if err := q.UpsertTunerServerStatus(ctx, sqlc.UpsertTunerServerStatusParams{
			DegradedSince: nil,
			LastError:     "",
			ClockSkewMs:   new(skew.Milliseconds()),
			CheckedAt:     now,
		}); err != nil {
			return fmt.Errorf("failed to record the tuner server status: %w", err)
		}

		return m.replaceTuners(ctx, tx, q, tuners, now)
	})
}

// recordDegraded は、縮退中であることを記録する。縮退が続いている間は、縮退し始めた日時を変えない。
func (m *Monitor) recordDegraded(ctx context.Context, now time.Time, cause error) error {
	m.logger.WarnContext(ctx, "tuner server is unreachable; running degraded", slog.Any("error", cause))

	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if err := m.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		since := now
		current, err := q.GetTunerServerStatus(ctx)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("failed to read the tuner server status: %w", err)
		case current.DegradedSince != nil:
			since = *current.DegradedSince
		}

		message := cause.Error()
		if len(message) > maxErrorLength {
			message = message[:maxErrorLength]
		}
		if err := q.UpsertTunerServerStatus(ctx, sqlc.UpsertTunerServerStatusParams{
			DegradedSince: &since,
			LastError:     message,
			ClockSkewMs:   nil,
			CheckedAt:     now,
		}); err != nil {
			return fmt.Errorf("failed to record the tuner server status: %w", err)
		}

		return nil
	})
}

// warnSkew は、時計のずれが閾値を超えたときと、閾値の内に戻ったときにログを出す。
func (m *Monitor) warnSkew(ctx context.Context, skew time.Duration) {
	skewed := skew.Abs() > m.opts.SkewThreshold
	switch {
	case skewed && !m.skewed:
		m.logger.WarnContext(ctx, "the clock of the tuner server differs from the OS clock; check NTP on both hosts",
			slog.Duration("skew", skew), slog.Duration("threshold", m.opts.SkewThreshold))
	case !skewed && m.skewed:
		m.logger.InfoContext(ctx, "the clock of the tuner server is back in sync", slog.Duration("skew", skew))
	}
	m.skewed = skewed
}

// replaceTuners は、チューナーの構成が変わっていれば DB の tuners を置き換え、変更を通知する。
// チューナーは数本しかないので、変わったときは丸ごと入れ替える。
func (m *Monitor) replaceTuners(ctx context.Context, tx pgx.Tx, q *sqlc.Queries, tuners []*mirakurun.Tuner, now time.Time) error {
	current, err := q.ListTuners(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tuners: %w", err)
	}
	next := make([]sqlc.ListTunersRow, 0, len(tuners))
	for _, tuner := range tuners {
		next = append(next, sqlc.ListTunersRow{
			TunerIndex:  int64(tuner.Index),
			Name:        tuner.Name,
			Types:       tuner.Types,
			IsAvailable: tuner.IsAvailable,
			IsRemote:    tuner.IsRemote,
			IsFault:     tuner.IsFault,
		})
	}
	slices.SortFunc(next, func(a, b sqlc.ListTunersRow) int { return cmp.Compare(a.TunerIndex, b.TunerIndex) })
	if slices.EqualFunc(current, next, equalTuner) {
		return nil
	}

	if err := q.DeleteTuners(ctx); err != nil {
		return fmt.Errorf("failed to delete tuners: %w", err)
	}
	for _, tuner := range next {
		if err := q.InsertTuner(ctx, sqlc.InsertTunerParams{
			TunerIndex:  tuner.TunerIndex,
			Name:        tuner.Name,
			Types:       tuner.Types,
			IsAvailable: tuner.IsAvailable,
			IsRemote:    tuner.IsRemote,
			IsFault:     tuner.IsFault,
			Now:         now,
		}); err != nil {
			return fmt.Errorf("failed to insert tuner %d: %w", tuner.TunerIndex, err)
		}
	}
	if _, err := notify.Publish(ctx, tx, m.clock, NotifyChannel); err != nil {
		return err
	}
	m.logger.InfoContext(ctx, "tuner configuration changed", slog.Int("tuners", len(next)))

	return nil
}

func equalTuner(a, b sqlc.ListTunersRow) bool {
	return a.TunerIndex == b.TunerIndex && a.Name == b.Name && slices.Equal(a.Types, b.Types) &&
		a.IsAvailable == b.IsAvailable && a.IsRemote == b.IsRemote && a.IsFault == b.IsFault
}
