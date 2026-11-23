package database

import (
	"context"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/db"
)

// MigrateWithScripts は、1 回だけのスクリプトを、埋め込んだものの代わりに scriptsFS から読む Migrate。
func MigrateWithScripts(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, scriptsFS fs.FS) error {
	return migrate(ctx, logger, pool, scriptsFS)
}

// ScriptVersions は、埋め込んだ SQL と Go の 1 回だけのスクリプトの版を、古い順に返す。
func ScriptVersions() ([]int64, error) {
	scripts, err := loadScripts(db.Scripts)
	if err != nil {
		return nil, err
	}
	versions := make([]int64, 0, len(scripts))
	for _, s := range scripts {
		versions = append(versions, s.version)
	}

	return versions, nil
}

// RenormalizeDashVariantsVersion は、正規化し直すスクリプトの版。
const RenormalizeDashVariantsVersion = renormalizeDashVariantsVersion
