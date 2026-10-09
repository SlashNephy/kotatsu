// Package database は PostgreSQL への接続と、スキーマのマイグレーションを扱う。
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/config"
)

// Open は PostgreSQL に接続する。接続できることを確かめてから返す。
func Open(ctx context.Context, cfg *config.Database) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL.Value())
	if err != nil {
		// pgx のエラーは接続文字列のパスワードを伏せている。
		return nil, fmt.Errorf("failed to parse the database URL: %w", err)
	}

	if cfg.Pooler {
		// トランザクションプーリングでは、トランザクションごとに別のサーバー接続に割り当てられる。
		// pgx の既定はサーバー側の名前付きプリペアドステートメントを使い回すので、使わない方式に変える。
		poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	}

	// 時刻は内部では UTC で扱う (設計書 8.1)。pgx の既定はローカルのタイムゾーンで返す。
	poolConfig.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name:  "timestamptz",
			OID:   pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})

		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create the database pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	return pool, nil
}
