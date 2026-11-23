package database

import (
	"context"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrateWithScripts は、1 回だけのスクリプトを、埋め込んだものの代わりに scriptsFS から読む Migrate。
func MigrateWithScripts(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, scriptsFS fs.FS) error {
	return migrate(ctx, logger, pool, scriptsFS)
}
