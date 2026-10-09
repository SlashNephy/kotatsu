package cli

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/neko"
)

// Deps は、ロールを起動する前に用意し、全ロールで共有するもの。
type Deps struct {
	Config *config.Config
	// DB は、スキーマのバージョンを確かめた後の接続。
	DB *pgxpool.Pool
	// Metrics は /metrics で公開するレジストリ。各ロールのメトリクスはここに登録する。
	Metrics *prometheus.Registry
	Clock   clock.Clock
	// InstanceID は、このプロセスのインスタンスの ID。リースの持ち主や、インスタンスの登録簿に使う。
	InstanceID uuid.UUID
}

// Runner はロールの本体。ctx が終わるまで動き続ける。
type Runner func(ctx context.Context, logger *slog.Logger, deps *Deps) error

// Role は kotatsu のサブコマンドで起動できるロール。
type Role struct {
	// Name はコードネーム (neko など)。
	Name string
	// Alias は役割名 (scheduler など)。サブコマンドとしても受け付ける。
	Alias string
	// Short は --help に出す説明。
	Short string
	Run   Runner
}

// DisplayName は --help とログに出す名前。役割名を必ず併記する。
func (r *Role) DisplayName() string {
	return r.Name + " (" + r.Alias + ")"
}

// DefaultRoles は全ロールを返す。呼び出しのたびに新しい値を返すので、呼び出し側で書き換えてよい。
// neko 以外のロールの本体は未実装で、起動ログを出して終了を待つだけである。
func DefaultRoles() []*Role {
	return []*Role{
		{
			Name:  "tenban",
			Alias: "server",
			Short: "Serve the GraphQL API, the web UI, and media streams",
			Run:   waitUntilDone,
		},
		{
			Name:  "neko",
			Alias: "scheduler",
			Short: "Sync the EPG, match rules, and assign recordings (one active instance)",
			Run:   runNeko,
		},
		{
			Name:  "mikan",
			Alias: "recorder",
			Short: "Record broadcast streams to files",
			Run:   waitUntilDone,
		},
		{
			Name:  "nekonote",
			Alias: "worker",
			Short: "Run background jobs such as thumbnails and notifications",
			Run:   waitUntilDone,
		},
		{
			Name:  "kyusu",
			Alias: "transcoder",
			Short: "Transcode streams on demand",
			Run:   waitUntilDone,
		},
	}
}

func runNeko(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")

	return neko.Run(ctx, logger, deps.DB, deps.Clock, deps.InstanceID)
}

func waitUntilDone(ctx context.Context, logger *slog.Logger, _ *Deps) error {
	logger.InfoContext(ctx, "role started")
	<-ctx.Done()
	logger.InfoContext(ctx, "role stopped")

	return nil
}
