// Package jobs は、River によるジョブの投入と実行を扱う (設計書 1.3、2.1)。
//
// ジョブは、ドメインの書き込みと同じトランザクションで Client.InsertTx を使って投入する。
// トランザクションをロールバックすれば、ジョブも投入されない。
// ジョブを実行するのは nekonote だけで、他のロールは投入だけを行う。
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/SlashNephy/kotatsu/internal/database"
)

// Client は River のクライアント。トランザクションには pgx.Tx を使う。
type Client = river.Client[pgx.Tx]

const (
	// maxWorkers は、nekonote が既定のキューで同時に実行するジョブの数の上限。
	maxWorkers = 10
	// QueueMedia は、FFmpeg で動画を読むような重いジョブのキュー。
	// EPGStation から多くの録画を取り込んだときも、CPU とディスクを使い切らないよう、1 つずつ実行する。
	QueueMedia = "media"
	// PriorityImported は、取り込んだ録画のためのジョブの優先度 (River の 1〜4 のうち、最も低い 4)。
	// 大量に取り込んでも、新しく録った録画の解析やサムネイルが、取り込んだ分の後ろで待たないようにする (#551)。
	PriorityImported = 4
	// PriorityDefault は、River の既定の優先度 (最も高い 1)。既定で低い優先度のジョブを、頼まれて動かすときに使う。
	PriorityDefault = 1
	// rescueStuckJobsAfter は、実行中のまま止まったとみなして、ほかのクライアントに渡すまでの時間。
	// ジョブの時間の上限のうち最も長いもの (セグメントの結合の 2 時間) より長くする。短いと、動いているジョブが救出され、
	// ほかの nekonote が同じジョブを二重に動かす (#515)。
	rescueStuckJobsAfter = 3 * time.Hour
)

// Options は NewClient の設定。
type Options struct {
	// PollOnly は、LISTEN/NOTIFY を使わずにポーリングだけでジョブを取るかどうか。
	// PgBouncer などのトランザクションプーリングを経由するときは true にする。
	PollOnly bool
	// Workers は、このクライアントで実行するジョブの Worker。nil なら投入だけを行うクライアントになる。
	Workers *river.Workers
	// PeriodicJobs は定期ジョブ。River のリーダーになったクライアントだけが投入する。Workers が nil なら使わない。
	PeriodicJobs []*river.PeriodicJob
}

// NewClient は River のクライアントを作る。ジョブを実行するには、Workers を与えて Start を呼ぶ。
func NewClient(pool *pgxpool.Pool, logger *slog.Logger, opts *Options) (*Client, error) {
	config := &river.Config{
		Logger:               logger,
		PollOnly:             opts.PollOnly,
		RescueStuckJobsAfter: rescueStuckJobsAfter,
	}
	if opts.Workers != nil {
		config.Workers = opts.Workers
		config.Queues = map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
			QueueMedia:         {MaxWorkers: 1},
		}
		config.PeriodicJobs = opts.PeriodicJobs
	}

	client, err := river.NewClient(riverpgxv5.New(pool), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create the River client: %w", err)
	}

	return client, nil
}

// Migrate は River のテーブルのマイグレーションを適用する。適用済みなら何もしない。
func Migrate(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	migrator, err := newMigrator(pool, logger)
	if err != nil {
		return err
	}

	result, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("failed to apply River migrations: %w", err)
	}
	// 適用したマイグレーションは、rivermigrate が 1 つずつログに出す。
	logger.InfoContext(ctx, "River schema is up to date", slog.Int("applied", len(result.Versions)))

	return nil
}

// CheckSchema は、River のマイグレーションがすべて適用されているかを確かめる。
// 適用されていなければ database.ErrSchemaOutdated を返す。
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := newMigrator(pool, slog.New(slog.DiscardHandler))
	if err != nil {
		return err
	}

	result, err := migrator.Validate(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to validate River migrations: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("%w (River: %v)", database.ErrSchemaOutdated, result.Messages)
	}

	return nil
}

func newMigrator(pool *pgxpool.Pool, logger *slog.Logger) (*rivermigrate.Migrator[pgx.Tx], error) {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("failed to create the River migrator: %w", err)
	}

	return migrator, nil
}
