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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/SlashNephy/kotatsu/internal/database"
)

// Client は River のクライアント。トランザクションには pgx.Tx を使う。
type Client = river.Client[pgx.Tx]

// maxWorkers は、nekonote が同時に実行するジョブの数の上限。
const maxWorkers = 10

// Options は NewClient の設定。
type Options struct {
	// PollOnly は、LISTEN/NOTIFY を使わずにポーリングだけでジョブを取るかどうか。
	// PgBouncer などのトランザクションプーリングを経由するときは true にする。
	PollOnly bool
	// Workers は、このクライアントで実行するジョブの Worker。nil なら投入だけを行うクライアントになる。
	Workers *river.Workers
}

// NewClient は River のクライアントを作る。ジョブを実行するには、Workers を与えて Start を呼ぶ。
func NewClient(pool *pgxpool.Pool, logger *slog.Logger, opts *Options) (*Client, error) {
	config := &river.Config{
		Logger:   logger,
		PollOnly: opts.PollOnly,
	}
	if opts.Workers != nil {
		config.Workers = opts.Workers
		config.Queues = map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: maxWorkers},
		}
	}

	client, err := river.NewClient(riverpgxv5.New(pool), config)
	if err != nil {
		return nil, fmt.Errorf("failed to create the River client: %w", err)
	}

	return client, nil
}

// Workers は、nekonote で実行するジョブの Worker を返す。まだ Worker がないので nil を返す。
// Worker を足したら、ここで river.AddWorker する。
func Workers() *river.Workers {
	return nil
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
