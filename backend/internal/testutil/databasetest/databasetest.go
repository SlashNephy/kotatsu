// Package databasetest は、本物の PostgreSQL を使うテストのために、テストごとに空のデータベースを用意する。
//
// 接続先は環境変数 KOTATSU_TEST_DATABASE_URL で与える。データベースを作れる権限が要る。
// 未設定なら、ローカルではテストを飛ばし、CI (環境変数 CI がある) では失敗させる。
// CI で黙って飛ばすと、DB を使うテストが一度も走らないまま通ってしまうからである。
package databasetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

const (
	// EnvURL は、テストに使う PostgreSQL の接続先を与える環境変数。
	EnvURL = "KOTATSU_TEST_DATABASE_URL"
	// maxConnsPerTest は、1 つのテストの接続プールの上限。
	maxConnsPerTest = 4
)

// New は空のデータベースを作り、その接続先を返す。データベースはテストの終了時に消す。
func New(tb testing.TB) *config.Database {
	tb.Helper()

	adminURL, ok := os.LookupEnv(EnvURL)
	if !ok || adminURL == "" {
		if _, inCI := os.LookupEnv("CI"); inCI {
			tb.Fatalf("%s must be set in CI", EnvURL)
		}
		tb.Skipf("%s is not set; skipping a test that needs PostgreSQL", EnvURL)
	}

	// 管理用の接続は、作るときと消すときだけつなぐ。テストの間つなぎっぱなしにすると、
	// 並行に走るテストの数だけ接続を使い、PostgreSQL の接続数の上限に当たる。
	name := "kotatsu_test_" + randomSuffix(tb)
	require.NoError(tb, execAdmin(tb.Context(), adminURL, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()))
	tb.Cleanup(func() {
		// tb.Context() はクリーンアップの時点で終わっているので、別の context を使う。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := execAdmin(ctx, adminURL, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			tb.Errorf("failed to drop the test database %s: %v", name, err)
		}
	})

	u, err := url.Parse(adminURL)
	require.NoError(tb, err)
	u.Path = "/" + name
	// 接続プールの上限の既定値は CPU の数なので、CPU の多いマシンで多くのテストを並べて走らせると、
	// PostgreSQL の接続数の上限に当たる。テストでは小さく抑える。
	query := u.Query()
	query.Set("pool_max_conns", strconv.Itoa(maxConnsPerTest))
	u.RawQuery = query.Encode()

	return &config.Database{URL: config.Secret(u.String()), Pooler: false}
}

// execAdmin は、管理用の接続で sql を実行し、接続を閉じる。
func execAdmin(ctx context.Context, adminURL, sql string) error {
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		return fmt.Errorf("failed to connect to the admin database: %w", err)
	}
	defer func() {
		_ = admin.Close(context.WithoutCancel(ctx))
	}()
	if _, err := admin.Exec(ctx, sql); err != nil {
		return fmt.Errorf("failed to run %q: %w", sql, err)
	}

	return nil
}

func randomSuffix(tb testing.TB) string {
	tb.Helper()

	b := make([]byte, 8)
	_, err := rand.Read(b)
	require.NoError(tb, err)

	return hex.EncodeToString(b)
}
