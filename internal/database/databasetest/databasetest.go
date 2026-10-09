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
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/config"
)

// EnvURL は、テストに使う PostgreSQL の接続先を与える環境変数。
const EnvURL = "KOTATSU_TEST_DATABASE_URL"

// New は空のデータベースを作り、その接続先を返す。データベースはテストの終了時に消す。
func New(t *testing.T) *config.Database {
	t.Helper()

	adminURL, ok := os.LookupEnv(EnvURL)
	if !ok || adminURL == "" {
		if _, inCI := os.LookupEnv("CI"); inCI {
			t.Fatalf("%s must be set in CI", EnvURL)
		}
		t.Skipf("%s is not set; skipping a test that needs PostgreSQL", EnvURL)
	}

	ctx := t.Context()
	admin, err := pgx.Connect(ctx, adminURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = admin.Close(context.Background())
	})

	name := "kotatsu_test_" + randomSuffix(t)
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, err)
	t.Cleanup(func() {
		// t.Context() はクリーンアップの時点で終わっているので、別の context を使う。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		if err != nil {
			t.Errorf("failed to drop the test database %s: %v", name, err)
		}
	})

	u, err := url.Parse(adminURL)
	require.NoError(t, err)
	u.Path = "/" + name

	return &config.Database{URL: config.Secret(u.String()), Pooler: false}
}

func randomSuffix(t *testing.T) string {
	t.Helper()

	b := make([]byte, 8)
	_, err := rand.Read(b)
	require.NoError(t, err)

	return hex.EncodeToString(b)
}
