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
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/notification"
)

const (
	// NotifyChannel は、チューナーの構成が変わったことを通知する変更通知のチャンネル。
	NotifyChannel = "tuners"
	// DefaultPollInterval は、チューナーの構成を取得し直す間隔。
	DefaultPollInterval = time.Minute
	// DefaultSkewThreshold は、チューナーサーバーの時計がこれより大きくずれていたら警告する差。
	// 設計書に定めはない。録画のマージンの既定値 (15 秒) を食い潰す前に気付けるよう、10 秒にする。
	DefaultSkewThreshold = 10 * time.Second
	// maxErrorLength は、DB に残す失敗の理由の長さの上限。
	maxErrorLength = 1000
)

// Upstream は、状態の把握に使うチューナーサーバーの API。tunerserver.Client が満たす。
type Upstream interface {
	Tuners(ctx context.Context) ([]*tunerserver.Tuner, error)
	ServerDate(ctx context.Context) (time.Time, error)
	Identify(ctx context.Context) (*tunerserver.Identity, error)
}

var _ Upstream = tunerserver.Client(nil)

// Guard は、書き込みのトランザクションの中で、まだ neko のリーダーであることを確認する。
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
	tuners, err := m.upstream.Tuners(ctx)
	now := m.clock.Now()
	if err != nil {
		return m.recordDegraded(ctx, now, err)
	}

	// 時計は、Date ヘッダーが取れたときだけ比較する。取れなくても、チューナーの構成は記録する。
	// EDCB は時計を返さないので (#1532)、比較せずに、警告も出さない。
	var skewMs *int64
	date, err := m.upstream.ServerDate(ctx)
	switch {
	case errors.Is(err, tunerserver.ErrClockUnavailable):
	case err != nil:
		m.logger.WarnContext(ctx, "failed to read the clock of the tuner server", slog.Any("error", err))
	default:
		skew := date.Sub(now)
		m.warnSkew(ctx, skew)
		skewMs = new(skew.Milliseconds())
	}

	// チューナーサーバー自身の予約がチューナーを使う時間帯 (#1536)。読めなければ、前に読んだものを残す。
	// 空にすると、EDCB の予約に使われるチューナーへ、Kotatsu の録画を割り当ててしまう。
	var occupancies []*tunerserver.TunerOccupancy
	if lister, ok := m.upstream.(tunerserver.TunerOccupancyLister); ok {
		occupancies, err = lister.TunerOccupancies(ctx)
		if err != nil {
			m.logger.WarnContext(ctx, "failed to read the reservations of the tuner server", slog.Any("error", err))
			occupancies = nil
		} else if occupancies == nil {
			occupancies = []*tunerserver.TunerOccupancy{}
		}
	} else {
		occupancies = []*tunerserver.TunerOccupancy{}
	}

	// 種類とバージョンは、判定できたときだけ書く。判定できなければ、最後に分かったものを残す (#1538)。
	var kind, version *string
	identity, err := m.upstream.Identify(ctx)
	if err != nil {
		m.logger.WarnContext(ctx, "failed to identify the tuner server", slog.Any("error", err))
	} else {
		kind, version = new(identity.Kind.String()), new(identity.Version)
	}

	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if err := m.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		if err := q.UpsertTunerServerStatus(ctx, sqlc.UpsertTunerServerStatusParams{
			DegradedSince: nil,
			LastError:     "",
			ClockSkewMs:   skewMs,
			CheckedAt:     now,
			ServerKind:    kind,
			ServerVersion: version,
		}); err != nil {
			return fmt.Errorf("failed to record the tuner server status: %w", err)
		}

		changed, err := m.replaceTuners(ctx, q, tuners, now)
		if err != nil {
			return err
		}
		if occupancies != nil {
			occupied, err := m.replaceOccupancies(ctx, q, occupancies, now)
			if err != nil {
				return err
			}
			changed = changed || occupied
		}
		if !changed {
			return nil
		}
		if _, err := notify.Publish(ctx, tx, m.clock, NotifyChannel); err != nil {
			return err
		}

		return nil
	})
}

// replaceOccupancies は、チューナーサーバー自身の予約がチューナーを使う時間帯が変わっていれば、DB の tuner_occupancies を置き換える (#1536)。
// 終わった時間帯は割り当てに効かないので残さない。置き換えたかどうかを返す。
func (m *Monitor) replaceOccupancies(ctx context.Context, q *sqlc.Queries, occupancies []*tunerserver.TunerOccupancy, now time.Time) (bool, error) {
	current, err := q.ListTunerOccupancies(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list tuner occupancies: %w", err)
	}
	next := make([]sqlc.TunerOccupancy, 0, len(occupancies))
	for _, o := range occupancies {
		if o == nil || !o.Start.Before(o.End) || !o.End.After(now) {
			continue
		}
		next = append(next, sqlc.TunerOccupancy{TunerIndex: int64(o.TunerIndex), StartAt: o.Start.UTC(), EndAt: o.End.UTC()})
	}
	slices.SortFunc(next, func(a, b sqlc.TunerOccupancy) int {
		return cmp.Or(cmp.Compare(a.TunerIndex, b.TunerIndex), a.StartAt.Compare(b.StartAt), a.EndAt.Compare(b.EndAt))
	})
	next = slices.CompactFunc(next, equalOccupancy)
	if slices.EqualFunc(current, next, equalOccupancy) {
		return false, nil
	}
	if err := q.DeleteTunerOccupancies(ctx); err != nil {
		return false, fmt.Errorf("failed to delete tuner occupancies: %w", err)
	}
	for _, o := range next {
		if err := q.InsertTunerOccupancy(ctx, sqlc.InsertTunerOccupancyParams(o)); err != nil {
			return false, fmt.Errorf("failed to insert a tuner occupancy: %w", err)
		}
	}
	m.logger.InfoContext(ctx, "tuner occupancies by the tuner server changed", slog.Int("occupancies", len(next)))

	return true, nil
}

func equalOccupancy(a, b sqlc.TunerOccupancy) bool {
	return a.TunerIndex == b.TunerIndex && a.StartAt.Equal(b.StartAt) && a.EndAt.Equal(b.EndAt)
}

// recordDegraded は、縮退中であることを記録する。縮退が続いている間は、縮退し始めた日時を変えない。
func (m *Monitor) recordDegraded(ctx context.Context, now time.Time, cause error) error {
	m.logger.WarnContext(ctx, "tuner server is unreachable; running degraded", slog.Any("error", cause))

	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if err := m.guard(ctx, tx); err != nil {
			return err
		}
		q := sqlc.New(tx)
		since, starting := now, true
		current, err := q.GetTunerServerStatus(ctx)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("failed to read the tuner server status: %w", err)
		case current.DegradedSince != nil:
			since, starting = *current.DegradedSince, false
		}

		message := truncate(cause.Error(), maxErrorLength)
		// 縮退し始めたときだけ通知する (設計書 6.2)。縮退が続く間は、確認するたびに通知しない。
		// 通知は視聴者にも届くので、チューナーサーバーの URL を含みうる理由は載せない。理由は、管理者がシステムの状態で読む。
		if starting {
			if err := notification.Emit(ctx, tx, m.clock, domain.NotificationKindTunerServerUnreachable, nil); err != nil {
				return err
			}
		}
		if err := q.UpsertTunerServerStatus(ctx, sqlc.UpsertTunerServerStatusParams{
			DegradedSince: &since,
			LastError:     message,
			ClockSkewMs:   nil,
			CheckedAt:     now,
			ServerKind:    nil,
			ServerVersion: nil,
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

// replaceTuners は、チューナーの構成が変わっていれば DB の tuners を置き換える。置き換えたかどうかを返す。
// チューナーは数本しかないので、変わったときは丸ごと入れ替える。
func (m *Monitor) replaceTuners(ctx context.Context, q *sqlc.Queries, tuners []*tunerserver.Tuner, now time.Time) (bool, error) {
	current, err := q.ListTuners(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list tuners: %w", err)
	}
	next := make([]sqlc.ListTunersRow, 0, len(tuners))
	for _, tuner := range tuners {
		next = append(next, sqlc.ListTunersRow{
			TunerIndex: int64(tuner.Index),
			Name:       tuner.Name,
			// 種別のカラムは NOT NULL なので、返されなかったときは空にする。
			Types:       append([]string{}, tuner.Types...),
			IsAvailable: tuner.IsAvailable,
			IsRemote:    tuner.IsRemote,
			IsFault:     tuner.IsFault,
		})
	}
	slices.SortFunc(next, func(a, b sqlc.ListTunersRow) int { return cmp.Compare(a.TunerIndex, b.TunerIndex) })
	if slices.EqualFunc(current, next, equalTuner) {
		return false, nil
	}

	if err := q.DeleteTuners(ctx); err != nil {
		return false, fmt.Errorf("failed to delete tuners: %w", err)
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
			return false, fmt.Errorf("failed to insert tuner %d: %w", tuner.TunerIndex, err)
		}
	}
	m.logger.InfoContext(ctx, "tuner configuration changed", slog.Int("tuners", len(next)))

	return true, nil
}

// truncate は、s を maxBytes バイト以内に、文字の途中で切らずに縮める。
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}

	return s
}

func equalTuner(a, b sqlc.ListTunersRow) bool {
	return a.TunerIndex == b.TunerIndex && a.Name == b.Name && slices.Equal(a.Types, b.Types) &&
		a.IsAvailable == b.IsAvailable && a.IsRemote == b.IsRemote && a.IsFault == b.IsFault
}
