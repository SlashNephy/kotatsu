package cli

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/config"
	"github.com/SlashNephy/kotatsu/internal/httpserver"
	"github.com/SlashNephy/kotatsu/internal/jobs"
	"github.com/SlashNephy/kotatsu/internal/neko"
	"github.com/SlashNephy/kotatsu/internal/rule"
	"github.com/SlashNephy/kotatsu/internal/tuner/mirakurun"
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
	// Jobs は River のクライアント。ジョブはドメインの書き込みと同じトランザクションで InsertTx で投入する。
	Jobs *jobs.Client
	// JobWorkers は、Jobs で実行するジョブの Worker。nekonote を動かさないプロセスや、Worker がないときは nil。
	JobWorkers *river.Workers
}

// defaultTimeZone は、表示のタイムゾーンの既定値 (設計書 8.1)。
const defaultTimeZone = "Asia/Tokyo"

// nekonoteRoleName は、ジョブを実行するロールの名前。
const nekonoteRoleName = "nekonote"

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
			Name:  nekonoteRoleName,
			Alias: "worker",
			Short: "Run background jobs such as thumbnails and notifications",
			Run:   runNekonote,
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

	var tuner *mirakurun.Client
	if url := deps.Config.Tuner.URL.Value(); url != "" {
		var err error
		tuner, err = mirakurun.New(&mirakurun.Options{
			URL:            url,
			Username:       "",
			Password:       "",
			UserAgent:      "",
			HTTPClient:     nil,
			RequestTimeout: 0,
			DetectInterval: 0,
			Clock:          deps.Clock,
		})
		if err != nil {
			return err
		}
	}

	// 表示のタイムゾーンは設定で決める (設計書 8.1) が、設定の項目はまだないので、既定の Asia/Tokyo を使う。
	location, err := time.LoadLocation(defaultTimeZone)
	if err != nil {
		return fmt.Errorf("failed to load the time zone %s: %w", defaultTimeZone, err)
	}

	return neko.Run(ctx, logger, &neko.Options{
		DB:         deps.DB,
		Clock:      deps.Clock,
		InstanceID: deps.InstanceID,
		Tuner:      tuner,
		EPG:        nil,
		Archive:    nil,
		Tuners:     nil,
		Location:   location,
		Rules: &rule.RunnerOptions{
			Debounce:     0,
			Interval:     0,
			PollOnly:     deps.Config.Database.Pooler,
			PollInterval: 0,
		},
	})
}

// runNekonote は、ctx が終わるまで River のジョブを実行する。止まるときは、実行中のジョブが終わるのを待つ。
func runNekonote(ctx context.Context, logger *slog.Logger, deps *Deps) error {
	logger.InfoContext(ctx, "role started")
	defer logger.InfoContext(ctx, "role stopped")

	if deps.JobWorkers == nil {
		logger.InfoContext(ctx, "no job workers are registered yet")
		<-ctx.Done()

		return nil
	}

	if err := deps.Jobs.Start(ctx); err != nil {
		return fmt.Errorf("failed to start the River client: %w", err)
	}
	<-ctx.Done()

	// 終了のシグナルで ctx は終わっているので、実行中のジョブを待つための context を別に作る。
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpserver.ShutdownTimeout)
	defer cancel()
	if err := deps.Jobs.Stop(stopCtx); err != nil {
		return fmt.Errorf("failed to stop the River client gracefully: %w", err)
	}

	return nil
}

func waitUntilDone(ctx context.Context, logger *slog.Logger, _ *Deps) error {
	logger.InfoContext(ctx, "role started")
	<-ctx.Done()
	logger.InfoContext(ctx, "role stopped")

	return nil
}
