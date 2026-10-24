package epgstation

import (
	"context"
	"database/sql"
)

// ExecInReadOnlyTx は、Read と同じ読み取り専用のトランザクションで query を実行する。書き込みが拒まれるかを確かめるのに使う。
func (s *Source) ExecInReadOnlyTx(ctx context.Context, query string) error {
	return s.readOnly(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query)

		return err
	})
}

// ParseSourceURL は parseSourceURL をテストから呼ぶ。
func ParseSourceURL(rawURL string) (string, string, error) {
	return parseSourceURL(rawURL)
}
