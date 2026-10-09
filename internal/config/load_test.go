package config_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/config"
)

const (
	yamlURL = "postgres://yaml@db/kotatsu"
	envURL  = "postgres://env@db/kotatsu"
	fileURL = "postgres://file@db/kotatsu"

	envKeyURL = "KOTATSU_DATABASE_URL"

	// Default の既定値。:9090 は設計書の付録 A で定めている。
	defaultServerListen  = ":8080"
	defaultMetricsListen = ":9090"
	// 設計書の付録 A の初期値。
	defaultAuditRetentionDays = 365
)

func TestLoadPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		env     map[string]string
		file    string
		wantURL string
		wantErr string
	}{
		{
			name:    "yaml only",
			yaml:    "database:\n  url: " + yamlURL + "\n",
			wantURL: yamlURL,
		},
		{
			name:    "env overrides yaml",
			yaml:    "database:\n  url: " + yamlURL + "\n",
			env:     map[string]string{envKeyURL: envURL},
			wantURL: envURL,
		},
		{
			name:    "file overrides yaml",
			yaml:    "database:\n  url: " + yamlURL + "\n",
			file:    fileURL + "\n",
			wantURL: fileURL,
		},
		{
			name:    "env and file conflict",
			yaml:    "database:\n  url: " + yamlURL + "\n",
			env:     map[string]string{envKeyURL: envURL},
			file:    fileURL,
			wantErr: "both KOTATSU_DATABASE_URL and KOTATSU_DATABASE_URL_FILE are set",
		},
		{
			name:    "env without yaml",
			env:     map[string]string{envKeyURL: envURL},
			wantURL: envURL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			env := maps.Clone(tt.env)
			if env == nil {
				env = map[string]string{}
			}
			if tt.file != "" {
				env["KOTATSU_DATABASE_URL_FILE"] = writeFile(t, dir, "database_url", tt.file)
			}
			path := ""
			if tt.yaml != "" {
				path = writeFile(t, dir, "config.yaml", tt.yaml)
			}

			cfg, err := config.Load(path, lookup(env))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, cfg.Database.URL.Value())
		})
	}
}

func TestLoadAppliesDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		env  map[string]string
		want *config.Config
	}{
		{
			name: "defaults",
			env:  map[string]string{envKeyURL: envURL},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
			},
		},
		{
			name: "otlp endpoint",
			env: map[string]string{
				envKeyURL:                         envURL,
				"KOTATSU_TELEMETRY_OTLP_ENDPOINT": "http://collector:4318",
			},
			want: &config.Config{
				Server:    config.Server{Listen: defaultServerListen},
				Metrics:   config.Metrics{Listen: defaultMetricsListen},
				Database:  config.Database{URL: envURL, Pooler: false},
				Telemetry: config.Telemetry{OTLPEndpoint: "http://collector:4318"},
				Audit:     config.Audit{RetentionDays: defaultAuditRetentionDays},
			},
		},
		{
			name: "yaml and env for different fields",
			yaml: "server:\n  listen: 127.0.0.1:3000\ndatabase:\n  pooler: true\n",
			env: map[string]string{
				envKeyURL:                envURL,
				"KOTATSU_METRICS_LISTEN": ":9100",
			},
			want: &config.Config{
				Server:   config.Server{Listen: "127.0.0.1:3000"},
				Metrics:  config.Metrics{Listen: ":9100"},
				Database: config.Database{URL: envURL, Pooler: true},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
			},
		},
		{
			name: "postgresql scheme",
			env:  map[string]string{envKeyURL: "postgresql://db/kotatsu"},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: "postgresql://db/kotatsu", Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
			},
		},
		{
			name: "audit retention",
			yaml: "audit:\n  retention_days: 30\n",
			env:  map[string]string{envKeyURL: envURL},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: 30},
			},
		},
		{
			name: "env overrides yaml boolean",
			yaml: "database:\n  pooler: true\n",
			env: map[string]string{
				envKeyURL:                 envURL,
				"KOTATSU_DATABASE_POOLER": "false",
			},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := ""
			if tt.yaml != "" {
				path = writeFile(t, t.TempDir(), "config.yaml", tt.yaml)
			}

			cfg, err := config.Load(path, lookup(tt.env))
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		env      map[string]string
		wantErrs []string
	}{
		{
			name:     "missing required field",
			wantErrs: []string{"database.url (KOTATSU_DATABASE_URL): is required"},
		},
		{
			name: "invalid boolean",
			env: map[string]string{
				envKeyURL:                 envURL,
				"KOTATSU_DATABASE_POOLER": "maybe",
			},
			wantErrs: []string{"database.pooler (KOTATSU_DATABASE_POOLER): must be a boolean"},
		},
		{
			name: "invalid listen addresses",
			env: map[string]string{
				envKeyURL:                envURL,
				"KOTATSU_SERVER_LISTEN":  "8080",
				"KOTATSU_METRICS_LISTEN": "localhost",
			},
			wantErrs: []string{
				"server.listen (KOTATSU_SERVER_LISTEN): must be in the form host:port",
				"metrics.listen (KOTATSU_METRICS_LISTEN): must be in the form host:port",
			},
		},
		{
			name: "invalid audit retention",
			env: map[string]string{
				envKeyURL:                      envURL,
				"KOTATSU_AUDIT_RETENTION_DAYS": "0",
			},
			wantErrs: []string{"audit.retention_days (KOTATSU_AUDIT_RETENTION_DAYS): must be at least 1"},
		},
		{
			name: "non-integer audit retention",
			env: map[string]string{
				envKeyURL:                      envURL,
				"KOTATSU_AUDIT_RETENTION_DAYS": "a year",
			},
			wantErrs: []string{"audit.retention_days (KOTATSU_AUDIT_RETENTION_DAYS): must be an integer"},
		},
		{
			name: "invalid otlp endpoint",
			env: map[string]string{
				envKeyURL:                         envURL,
				"KOTATSU_TELEMETRY_OTLP_ENDPOINT": "collector:4318",
			},
			wantErrs: []string{"telemetry.otlp_endpoint (KOTATSU_TELEMETRY_OTLP_ENDPOINT): must be an http:// or https:// URL"},
		},
		{
			name:     "unknown key in yaml",
			yaml:     "database:\n  uri: " + yamlURL + "\n",
			wantErrs: []string{"field uri not found"},
		},
		{
			name:     "missing secret file",
			env:      map[string]string{"KOTATSU_DATABASE_URL_FILE": "/nonexistent/database_url"},
			wantErrs: []string{"database.url (KOTATSU_DATABASE_URL): failed to read the file given by KOTATSU_DATABASE_URL_FILE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := ""
			if tt.yaml != "" {
				path = writeFile(t, t.TempDir(), "config.yaml", tt.yaml)
			}

			_, err := config.Load(path, lookup(tt.env))
			require.Error(t, err)
			for _, want := range tt.wantErrs {
				assert.ErrorContains(t, err, want)
			}
		})
	}
}

func TestLoadErrorDoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	secretURL := (&url.URL{Scheme: "mysql", User: url.UserPassword("user", "hunter2"), Host: "db", Path: "/kotatsu"}).String()

	_, err := config.Load("", lookup(map[string]string{envKeyURL: secretURL}))
	require.ErrorContains(t, err, "database.url (KOTATSU_DATABASE_URL): must use the postgres:// or postgresql:// scheme")
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestSecretIsRedactedInOutput(t *testing.T) {
	t.Parallel()

	value := (&url.URL{Scheme: "postgres", User: url.UserPassword("user", "hunter2"), Host: "db", Path: "/kotatsu"}).String()
	cfg := config.Default()
	cfg.Database.URL = config.Secret(value)

	var logBuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&logBuf, nil)).Info("config", slog.Any("config", cfg))
	jsonBytes, err := json.Marshal(cfg.Database.URL)
	require.NoError(t, err)

	tests := []struct {
		name string
		out  string
	}{
		{name: "slog", out: logBuf.String()},
		{name: "json", out: string(jsonBytes)},
		{name: "%v", out: fmt.Sprintf("%v", cfg)},
		{name: "%+v", out: fmt.Sprintf("%+v", cfg)},
		{name: "%#v", out: fmt.Sprintf("%#v", cfg)},
		{name: "%q", out: fmt.Sprintf("%q", cfg.Database.URL)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.NotContains(t, tt.out, "hunter2")
			assert.Contains(t, tt.out, "[REDACTED]")
		})
	}

	assert.Equal(t, value, cfg.Database.URL.Value())
}

func lookup(env map[string]string) config.LookupEnv {
	return func(key string) (string, bool) {
		v, ok := env[key]

		return v, ok
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
