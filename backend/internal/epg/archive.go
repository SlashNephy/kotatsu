package epg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

const (
	// DefaultArchiveGrace は、放送が終わってから program_archives へ移すまでの猶予 (設計書 3.4)。
	// Mirakurun は放送の終わった番組をしばらく持ち続け、その間に更新が届くことがある。
	DefaultArchiveGrace = 24 * time.Hour
	// DefaultArchiveRetention は、アーカイブの保持期間 (設計書 3.4)。
	DefaultArchiveRetention = 90 * 24 * time.Hour
	// DefaultArchiveInterval は、アーカイブと削除を行う間隔。
	DefaultArchiveInterval = 10 * time.Minute
	// DefaultArchiveBatchSize は、1 つのトランザクションで移したり消したりする行の数の上限。
	// 一度に大量の行をロックしないよう、少しずつ処理する。
	DefaultArchiveBatchSize = 1000
)

// ArchiverOptions は Archiver の設定。0 の項目は既定値にする。
type ArchiverOptions struct {
	Grace time.Duration
	// Retention はアーカイブの保持期間。負の値なら無期限に残す (設計書 3.4 の「0 で無期限」を、ここでは負の値で表す)。
	Retention time.Duration
	// RetentionFunc は、保持期間を返す。画面から変えられるので (設計書 11.2)、消すたびに引く。nil なら Retention を使う。
	RetentionFunc func(ctx context.Context) (time.Duration, error)
	Interval      time.Duration
	BatchSize     int32
}

// Archiver は、放送が終わった番組を program_archives へ移し、保持期間を過ぎたアーカイブを消す (設計書 3.4)。
// 放送前に予定から消えた番組は、同期 (Reconciler) が programs から消すので、アーカイブには入らない。
type Archiver struct {
	pool   *pgxpool.Pool
	clock  clock.Clock
	guard  Guard
	logger *slog.Logger
	opts   ArchiverOptions
}

// NewArchiver は Archiver を作る。opts は nil でもよい。
func NewArchiver(pool *pgxpool.Pool, clk clock.Clock, guard Guard, logger *slog.Logger, opts *ArchiverOptions) *Archiver {
	o := ArchiverOptions{
		Grace:         DefaultArchiveGrace,
		Retention:     DefaultArchiveRetention,
		RetentionFunc: nil,
		Interval:      DefaultArchiveInterval,
		BatchSize:     DefaultArchiveBatchSize,
	}
	if opts != nil {
		o.Grace = cmpOr(opts.Grace, o.Grace)
		o.Retention = cmpOr(opts.Retention, o.Retention)
		o.RetentionFunc = opts.RetentionFunc
		o.Interval = cmpOr(opts.Interval, o.Interval)
		if opts.BatchSize > 0 {
			o.BatchSize = opts.BatchSize
		}
	}

	return &Archiver{pool: pool, clock: clk, guard: guard, logger: logger, opts: o}
}

// ArchiveResult は、1 回の処理の結果。
type ArchiveResult struct {
	Archived int64
	Pruned   int64
}

// Run は、ctx が終わるまで、Interval ごとにアーカイブと削除を行う。失敗はログに出して次の回を待つ。
func (a *Archiver) Run(ctx context.Context) error {
	ticker := time.NewTicker(a.opts.Interval)
	defer ticker.Stop()
	for {
		result, err := a.RunOnce(ctx)
		if err != nil {
			a.logger.WarnContext(ctx, "failed to archive programs", slog.Any("error", err))
		} else if result.Archived > 0 || result.Pruned > 0 {
			a.logger.InfoContext(ctx, "archived programs", slog.Int64("archived", result.Archived), slog.Int64("pruned", result.Pruned))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// RunOnce は、猶予が過ぎた番組をすべて移し、保持期間を過ぎたアーカイブをすべて消す。
// どちらも BatchSize 件ずつ、別々のトランザクションで行う。
func (a *Archiver) RunOnce(ctx context.Context) (*ArchiveResult, error) {
	now := a.clock.Now()
	result := &ArchiveResult{Archived: 0, Pruned: 0}

	archived, err := a.repeat(ctx, func(q *sqlc.Queries) (int64, error) {
		return q.ArchivePrograms(ctx, sqlc.ArchiveProgramsParams{Now: now, Before: now.Add(-a.opts.Grace), BatchSize: a.opts.BatchSize})
	})
	result.Archived = archived
	if err != nil {
		return result, fmt.Errorf("failed to archive programs: %w", err)
	}

	retention := a.opts.Retention
	if a.opts.RetentionFunc != nil {
		if retention, err = a.opts.RetentionFunc(ctx); err != nil {
			return result, fmt.Errorf("failed to get the archive retention: %w", err)
		}
	}
	if retention < 0 {
		return result, nil
	}
	pruned, err := a.repeat(ctx, func(q *sqlc.Queries) (int64, error) {
		return q.PruneProgramArchives(ctx, sqlc.PruneProgramArchivesParams{Before: now.Add(-retention), BatchSize: a.opts.BatchSize})
	})
	result.Pruned = pruned
	if err != nil {
		return result, fmt.Errorf("failed to prune program archives: %w", err)
	}

	return result, nil
}

// repeat は、step が BatchSize 件より少なく処理するまで、step を別々のトランザクションで繰り返す。
func (a *Archiver) repeat(ctx context.Context, step func(q *sqlc.Queries) (int64, error)) (int64, error) {
	var total int64
	for {
		var n int64
		err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
			if err := a.guard(ctx, tx); err != nil {
				return err
			}
			var err error
			n, err = step(sqlc.New(tx))

			return err
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < int64(a.opts.BatchSize) {
			return total, nil
		}
	}
}
