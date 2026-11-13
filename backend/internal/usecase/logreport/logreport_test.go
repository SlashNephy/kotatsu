package logreport_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/config"
	"github.com/SlashNephy/kotatsu/internal/lib/logring"
	"github.com/SlashNephy/kotatsu/internal/usecase/logreport"
)

const secret = "s3cr3t-webhook-token"

func staticSecrets(values ...string) logreport.SecretSource {
	return func(context.Context) ([]string, error) { return values, nil }
}

// newTuner は、/api/log に handler で答えるチューナーサーバーの Client を作る。
func newTuner(tb testing.TB, logHandler http.HandlerFunc) *mirakurun.Client {
	tb.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"current":"5.3.2","latest":"","server":"mahiron"}`))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"5.3.2","process":{"arch":"amd64","platform":"linux"}}`))
	})
	mux.HandleFunc("/api/log", logHandler)
	server := httptest.NewServer(mux)
	tb.Cleanup(server.Close)
	client, err := mirakurun.New(&mirakurun.Options{URL: server.URL})
	require.NoError(tb, err)

	return client
}

func TestServiceReport(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(10, 0)
	buffer.Add(logring.Line{Time: 1, Level: logring.LevelWarn, Text: "webhook failed url=https://discord.com/api/webhooks/1/" + secret})
	tuner := newTuner(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("2026-11-12T00:00:00.000Z info: token " + secret + "\n2026-11-12T00:00:01.000Z info: second\n2026-11-12T00:00:02.000Z info: third\n"))
	})

	service := logreport.New(&logreport.Options{
		Buffer: buffer, Tuner: tuner, Hosts: []string{"http://Mirakurun.lan:40772/", "db:5432"},
		InstanceID: "instance-1", Roles: []string{"tenban"}, Secrets: staticSecrets(secret),
		Timeout: 0, Limit: 2, MaxLineLength: 0, Location: nil,
	})
	report, err := service.Report(t.Context(), "kotatsu.example.test:8080")
	require.NoError(t, err)

	assert.Equal(t, []string{"kotatsu.example.test", "mirakurun.lan", "db"}, report.Hosts)
	assert.Equal(t, "instance-1", report.InstanceID)
	assert.Equal(t, []string{"tenban"}, report.Roles)
	require.Len(t, report.Sources, 2)

	kotatsu := report.Sources[0]
	assert.Equal(t, logreport.SourceKindKotatsu, kotatsu.Kind)
	assert.Equal(t, []logreport.Line{
		{Time: 1, Level: logring.LevelWarn, Inferred: false, Text: "webhook failed url=https://discord.com/api/webhooks/1/[REDACTED_SECRET]"},
	}, kotatsu.Lines)
	assert.NotEmpty(t, kotatsu.OS)

	tunerSource := report.Sources[1]
	assert.Equal(t, logreport.SourceKindTunerServer, tunerSource.Kind)
	assert.Equal(t, logreport.FailureNone, tunerSource.Failure)
	assert.Equal(t, "5.3.2", tunerSource.Version)
	assert.Equal(t, "mahiron", tunerSource.Runtime)
	assert.Equal(t, "linux", tunerSource.OS)
	assert.Equal(t, "amd64", tunerSource.Arch)
	// 受け取るのは、新しい方から Limit 行まで。全体の行数は別に返す。
	assert.Equal(t, 3, tunerSource.Total)
	texts := make([]string, 0, len(tunerSource.Lines))
	for _, line := range tunerSource.Lines {
		texts = append(texts, line.Text)
	}
	assert.Equal(t, []string{"second", "third"}, texts)
}

func TestServiceReportRedactsTunerLines(t *testing.T) {
	t.Parallel()

	tuner := newTuner(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("2026-11-12T00:00:00.000Z info: token " + secret + "\n"))
	})
	service := logreport.New(&logreport.Options{
		Buffer: logring.NewBuffer(1, 0), Tuner: tuner, Hosts: nil, InstanceID: "", Roles: nil,
		Secrets: staticSecrets(secret), Timeout: 0, Limit: 0, MaxLineLength: 0, Location: nil,
	})
	report, err := service.Report(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, report.Sources[1].Lines, 1)
	assert.Equal(t, "token [REDACTED_SECRET]", report.Sources[1].Lines[0].Text)
	assert.Equal(t, []string{}, report.Hosts)
}

func TestServiceReportFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		timeout time.Duration
		want    logreport.Failure
	}{
		{
			name:    "ログの API が無ければ UNSUPPORTED",
			handler: func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) },
			want:    logreport.FailureUnsupported,
		},
		{
			name:    "ほかの状態は HTTP_ERROR",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
			want:    logreport.FailureHTTPError,
		},
		{
			name: "待ち時間を過ぎたら TIMEOUT",
			handler: func(_ http.ResponseWriter, r *http.Request) {
				<-r.Context().Done()
			},
			timeout: 50 * time.Millisecond,
			want:    logreport.FailureTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := logreport.New(&logreport.Options{
				Buffer: logring.NewBuffer(1, 0), Tuner: newTuner(t, tt.handler), Hosts: nil, InstanceID: "", Roles: nil,
				Secrets: staticSecrets(), Timeout: tt.timeout, Limit: 0, MaxLineLength: 0, Location: nil,
			})
			report, err := service.Report(t.Context(), "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, report.Sources[1].Failure)
			assert.Empty(t, report.Sources[1].Lines)
		})
	}
}

func TestServiceReportUnreachable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	tuner, err := mirakurun.New(&mirakurun.Options{URL: url})
	require.NoError(t, err)
	service := logreport.New(&logreport.Options{
		Buffer: logring.NewBuffer(1, 0), Tuner: tuner, Hosts: nil, InstanceID: "", Roles: nil,
		Secrets: staticSecrets(), Timeout: 0, Limit: 0, MaxLineLength: 0, Location: nil,
	})
	report, err := service.Report(t.Context(), "")
	require.NoError(t, err)
	assert.Equal(t, logreport.FailureUnreachable, report.Sources[1].Failure)
}

func TestServiceReportFailsWithoutSecrets(t *testing.T) {
	t.Parallel()

	buffer := logring.NewBuffer(1, 0)
	buffer.Add(logring.Line{Time: 1, Level: logring.LevelInfo, Text: secret})
	errSecrets := errors.New("db down")
	service := logreport.New(&logreport.Options{
		Buffer: buffer, Tuner: nil, Hosts: nil, InstanceID: "", Roles: nil,
		Secrets: func(context.Context) ([]string, error) { return nil, errSecrets },
		Timeout: 0, Limit: 0, MaxLineLength: 0, Location: nil,
	})
	// 伏せる値を集められなければ、伏せていないログを返さない。
	report, err := service.Report(t.Context(), "")
	require.ErrorIs(t, err, errSecrets)
	assert.Nil(t, report)
}

func TestConfigSecrets(t *testing.T) {
	t.Parallel()

	var cfg config.Config
	cfg.Auth.SecretKey = "app-secret-key-0123456789abcdef0123"
	cfg.Auth.OIDC.ClientSecret = "oidc-client-secret"
	cfg.Database.URL = "postgres://kotatsu:db-password@db:5432/kotatsu"
	cfg.Tuner.URL = "http://user:tuner-password@mirakurun:40772/"
	cfg.AnimeLinking.ArmURL = "https://example.test/arm.json?key=arm-secret"
	cfg.Telemetry.OTLPMetricsHeaders = "Mackerel-Api-Key=mackerel-key"
	env := map[string]string{"OTEL_EXPORTER_OTLP_HEADERS": "authorization=Bearer otel-token"}

	secrets := logreport.ConfigSecrets(&cfg, func(key string) (string, bool) {
		value, ok := env[key]

		return value, ok
	})
	for _, want := range []string{
		"app-secret-key-0123456789abcdef0123", "oidc-client-secret", "db-password", "tuner-password",
		"https://example.test/arm.json?key=arm-secret", "mackerel-key", "Bearer otel-token",
	} {
		assert.Contains(t, secrets, want)
	}
	// 秘密でない部分 (ユーザー名だけ、ホスト名) は、値として入れない。
	assert.NotContains(t, secrets, "db")
	assert.NotContains(t, secrets, "mirakurun")
}
