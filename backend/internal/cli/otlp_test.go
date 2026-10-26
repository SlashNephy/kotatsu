package cli_test

import (
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"

	"github.com/SlashNephy/kotatsu/internal/cli"
	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/testutil/databasetest"
)

// otlpSink は、OTLP/HTTP で届いたメトリクスの名前と、そのリソースの kotatsu.roles を集める。
type otlpSink struct {
	mu    sync.Mutex
	roles map[string]string
}

func (s *otlpSink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	gz, err := gzip.NewReader(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	raw, err := io.ReadAll(gz)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	req := &colmetricspb.ExportMetricsServiceRequest{}
	if err := proto.Unmarshal(raw, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)

		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rm := range req.GetResourceMetrics() {
		roles := ""
		for _, kv := range rm.GetResource().GetAttributes() {
			if kv.GetKey() == "kotatsu.roles" {
				roles = kv.GetValue().GetStringValue()
			}
		}
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				s.roles[m.GetName()] = roles
			}
		}
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
}

func (s *otlpSink) snapshot() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return maps.Clone(s.roles)
}

// TestServeSendsMetricsOverOTLP は、送信先を設定すると、各ロールが /metrics と同じメトリクスを OTLP で送ることを確かめる (#962)。
// このテストのロールは neko のリースを取らないので、クラスターの範囲 (DB から読む値) は送らない。
func TestServeSendsMetricsOverOTLP(t *testing.T) {
	t.Parallel()

	dbConfig := databasetest.New(t)
	sink := &otlpSink{mu: sync.Mutex{}, roles: map[string]string{}}
	server := httptest.NewServer(sink)
	t.Cleanup(server.Close)

	roles := cli.DefaultRoles()
	for _, role := range roles {
		role.Run = func(ctx context.Context, _ *slog.Logger, _ *cli.Deps) error {
			<-ctx.Done()

			return nil
		}
	}
	env := testEnv()
	env["KOTATSU_TELEMETRY_OTLP_METRICS_ENDPOINT"] = server.URL
	env["KOTATSU_TELEMETRY_OTLP_METRICS_PROTOCOL"] = config.OTLPProtocolHTTP
	env["KOTATSU_TELEMETRY_OTLP_METRICS_INTERVAL_SECONDS"] = "1"
	opts := testOptions(slog.New(slog.DiscardHandler), roles, func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	})
	// ロールを起動するときはマイグレーションを当てないので、開くときに当てておく。
	opts.OpenDatabase = func(ctx context.Context, _ *config.Database) (*pgxpool.Pool, error) {
		pool, err := database.Open(ctx, dbConfig)
		if err != nil {
			return nil, err
		}

		return pool, database.Migrate(ctx, slog.New(slog.DiscardHandler), pool)
	}
	cmd := cli.NewCommand(opts)
	cmd.SetArgs([]string{"all"})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()

	require.Eventually(t, func() bool {
		got := sink.snapshot()
		_, leader := got["kotatsu_neko_leader"]
		_, runtime := got["go_goroutines"]

		return leader && runtime
	}, 30*time.Second, 100*time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("roles did not stop after the context was canceled")
	}

	got := sink.snapshot()
	assert.NotContains(t, got, "kotatsu_recordings")
	// インスタンスの範囲には、このプロセスで動かしているロールを付ける。
	want := make([]string, 0, len(roles))
	for _, role := range roles {
		want = append(want, role.Name)
	}
	assert.ElementsMatch(t, want, strings.Split(got["kotatsu_neko_leader"], ","))
}
