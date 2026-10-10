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

	envKeyURL   = "KOTATSU_DATABASE_URL"
	envKeyPools = "KOTATSU_STORAGE_POOLS"
	// envKeyTunerURL は、チューナーサーバーの URL を渡す環境変数。
	envKeyTunerURL = "KOTATSU_TUNER_URL"
	// leakedWord は、エラーに漏れてはいけない、URL に含めた資格情報の一部。
	leakedWord = "hunter2"

	// Default の既定値。:9090 は設計書の付録 A で定めている。
	defaultServerListen  = ":8080"
	defaultMetricsListen = ":9090"
	// 設計書の付録 A の初期値。
	defaultAuditRetentionDays = 365

	testIssuer    = "https://idp.example.com"
	envOIDCIssuer = "KOTATSU_AUTH_OIDC_ISSUER"
	defaultGroups = "groups"
	testClientID  = "kotatsu"
	envOIDCClient = "KOTATSU_AUTH_OIDC_CLIENT_ID"
	envTrue       = "true"
)

// defaultOIDC は、OIDC の既定値 (OIDC でのログインは無効)。
func defaultOIDC() config.OIDC {
	return config.OIDC{
		Issuer: "", ClientID: "", ClientSecret: "", RedirectURL: "",
		Scopes: []string{"openid", "profile", "email"}, DisplayName: "OIDC", UsernameClaim: "preferred_username",
		AutoCreate: config.AutoCreate{
			Enabled: false, GroupsClaim: defaultGroups, AdminGroups: []string{}, MemberGroups: []string{}, ViewerGroups: []string{},
		},
	}
}

// noPools は、プールを設定していないときの Storage。
func noPools() config.Storage {
	return config.Storage{Pools: map[string]string{}}
}

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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "otlp endpoint",
			env: map[string]string{
				envKeyURL:                         envURL,
				"KOTATSU_TELEMETRY_OTLP_ENDPOINT": "http://collector:4318",
			},
			want: &config.Config{
				Server:    config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:   config.Metrics{Listen: defaultMetricsListen},
				Database:  config.Database{URL: envURL, Pooler: false},
				Telemetry: config.Telemetry{OTLPEndpoint: "http://collector:4318"},
				Audit:     config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:   noPools(),
				Tuner:     config.Tuner{URL: ""},
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
				Server:   config.Server{Listen: "127.0.0.1:3000", AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: ":9100"},
				Database: config.Database{URL: envURL, Pooler: true},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "postgresql scheme",
			env:  map[string]string{envKeyURL: "postgresql://db/kotatsu"},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: "postgresql://db/kotatsu", Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "have i been pwned",
			yaml: "auth:\n  hibp:\n    url: https://hibp.example.com\n",
			env:  map[string]string{envKeyURL: envURL, "KOTATSU_AUTH_HIBP_ENABLED": envTrue},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
				Auth:     config.Auth{HIBP: config.HIBP{Enabled: true, URL: "https://hibp.example.com"}},
			},
		},
		{
			name: "oidc",
			yaml: "auth:\n  oidc:\n    issuer: https://idp.example.com\n    client_id: kotatsu\n    auto_create:\n      enabled: true\n",
			env: map[string]string{
				envKeyURL: envURL, "KOTATSU_AUTH_OIDC_SCOPES": "openid, groups",
				"KOTATSU_AUTH_OIDC_AUTO_CREATE_ADMIN_GROUPS": "admins", "KOTATSU_AUTH_OIDC_AUTO_CREATE_VIEWER_GROUPS": "family,guests",
			},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
				Auth: config.Auth{OIDC: config.OIDC{
					Issuer: testIssuer, ClientID: testClientID, ClientSecret: "", RedirectURL: "",
					Scopes: []string{"openid", "groups"}, DisplayName: "OIDC", UsernameClaim: "preferred_username",
					AutoCreate: config.AutoCreate{
						Enabled: true, GroupsClaim: defaultGroups, AdminGroups: []string{"admins"}, MemberGroups: []string{}, ViewerGroups: []string{"family", "guests"},
					},
				}},
			},
		},
		{
			name: "audit retention",
			yaml: "audit:\n  retention_days: 30\n",
			env:  map[string]string{envKeyURL: envURL},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: 30},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "storage pools in yaml",
			yaml: "storage:\n  pools:\n    main: /mnt/main\n    sub-2: /mnt/sub\n",
			env:  map[string]string{envKeyURL: envURL},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  config.Storage{Pools: map[string]string{"main": "/mnt/main", "sub-2": "/mnt/sub"}},
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "env replaces storage pools in yaml",
			yaml: "storage:\n  pools:\n    main: /mnt/main\n",
			env: map[string]string{
				envKeyURL:   envURL,
				envKeyPools: "main=/data/main, archive=/data/archive",
			},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  config.Storage{Pools: map[string]string{"main": "/data/main", "archive": "/data/archive"}},
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "networks from yaml and env",
			yaml: "server:\n  allowed_networks: [0.0.0.0/0]\n",
			env: map[string]string{
				envKeyURL:                        envURL,
				"KOTATSU_SERVER_TRUSTED_PROXIES": " 10.0.0.1, 10.0.1.0/24 ,",
			},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: []string{"0.0.0.0/0"}, TrustedProxies: []string{"10.0.0.1", "10.0.1.0/24"}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "tuner server url from env",
			yaml: "",
			env: map[string]string{
				envKeyURL:      envURL,
				envKeyTunerURL: "http://mirakurun:40772/",
			},
			want: &config.Config{
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: "http://mirakurun:40772/"},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: defaultAuditRetentionDays},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
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
			// OIDC を扱わないケースでは、既定値のままであることを確かめる。
			if tt.want.Auth.OIDC.Scopes == nil {
				tt.want.Auth.OIDC = defaultOIDC()
			}
			// kyusu の設定は TestLoadKyusu で確かめるので、ここでは既定値のままであることだけを確かめる。
			tt.want.Kyusu = config.Kyusu{Encoder: config.EncoderX264, VAAPIDevice: defaultVAAPIDevice, WorkDir: ""}
			// jikkyo の設定は TestLoadJikkyo で確かめる。
			tt.want.Jikkyo = config.Jikkyo{Channels: map[string]string{}}
			assert.Equal(t, tt.want, cfg)
		})
	}
}

// TestLoadWebPushAndThumbnail は、Web Push とサムネイルの設定を、YAML と環境変数から読むことを確かめる。
func TestLoadWebPushAndThumbnail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		yaml          string
		env           map[string]string
		wantWebPush   config.WebPush
		wantThumbnail config.Thumbnail
	}{
		{
			name: "defaults", env: map[string]string{envKeyURL: envURL},
			wantWebPush: config.WebPush{Subject: ""}, wantThumbnail: config.Thumbnail{DisablePreview: false},
		},
		{
			name: "web push subject", env: map[string]string{envKeyURL: envURL, "KOTATSU_WEB_PUSH_SUBJECT": "mailto:admin@kotatsu.example"},
			wantWebPush: config.WebPush{Subject: "mailto:admin@kotatsu.example"}, wantThumbnail: config.Thumbnail{DisablePreview: false},
		},
		{
			name: "disable preview in yaml", yaml: "thumbnail:\n  disable_preview: true\n", env: map[string]string{envKeyURL: envURL},
			wantWebPush: config.WebPush{Subject: ""}, wantThumbnail: config.Thumbnail{DisablePreview: true},
		},
		{
			name: "disable preview in env", env: map[string]string{envKeyURL: envURL, "KOTATSU_THUMBNAIL_DISABLE_PREVIEW": envTrue},
			wantWebPush: config.WebPush{Subject: ""}, wantThumbnail: config.Thumbnail{DisablePreview: true},
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
			assert.Equal(t, tt.wantWebPush, cfg.WebPush)
			assert.Equal(t, tt.wantThumbnail, cfg.Thumbnail)
		})
	}
}

// defaultVAAPIDevice は、VAAPI で使うデバイスの既定値。
const defaultVAAPIDevice = "/dev/dri/renderD128"

// TestLoadKyusu は、kyusu の設定を、YAML と環境変数から読み、知らないエンコーダーを拒むことを確かめる。
func TestLoadKyusu(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		env     map[string]string
		want    config.Kyusu
		wantErr string
	}{
		{
			name: "default encoder", env: map[string]string{envKeyURL: envURL},
			want: config.Kyusu{Encoder: config.EncoderX264, VAAPIDevice: defaultVAAPIDevice, WorkDir: ""},
		},
		{
			name: "yaml", yaml: "kyusu:\n  encoder: vaapi\n  vaapi_device: /dev/dri/renderD129\n  work_dir: /var/kyusu\n",
			env:  map[string]string{envKeyURL: envURL},
			want: config.Kyusu{Encoder: config.EncoderVAAPI, VAAPIDevice: "/dev/dri/renderD129", WorkDir: "/var/kyusu"},
		},
		{
			name: "env", env: map[string]string{envKeyURL: envURL, "KOTATSU_KYUSU_ENCODER": "nvenc", "KOTATSU_KYUSU_WORK_DIR": "/tmp/kyusu"},
			want: config.Kyusu{Encoder: config.EncoderNVENC, VAAPIDevice: defaultVAAPIDevice, WorkDir: "/tmp/kyusu"},
		},
		{
			name: "unknown encoder", env: map[string]string{envKeyURL: envURL, "KOTATSU_KYUSU_ENCODER": "amf"},
			wantErr: "kyusu.encoder (KOTATSU_KYUSU_ENCODER): must be one of x264, vaapi, qsv, nvenc",
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
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.Kyusu)
		})
	}
}

func TestLoadJikkyo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		env  map[string]string
		want map[string]string
	}{
		{name: "既定では、同梱した対応表だけを使う", env: map[string]string{envKeyURL: envURL}, want: map[string]string{}},
		{
			name: "yaml", yaml: "jikkyo:\n  channels:\n    \"4:101\": jk101\n",
			env:  map[string]string{envKeyURL: envURL},
			want: map[string]string{"4:101": "jk101"},
		},
		{
			name: "env", env: map[string]string{envKeyURL: envURL, "KOTATSU_JIKKYO_CHANNELS": "4:101=jk101, 32736:1024=jk1"},
			want: map[string]string{"4:101": "jk101", "32736:1024": "jk1"},
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
			assert.Equal(t, tt.want, cfg.Jikkyo.Channels)
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
			name: "invalid networks",
			env: map[string]string{
				envKeyURL:                         envURL,
				"KOTATSU_SERVER_ALLOWED_NETWORKS": "10.0.0.0/8,example.com",
			},
			wantErrs: []string{`server.allowed_networks (KOTATSU_SERVER_ALLOWED_NETWORKS): "example.com" must be a CIDR or an IP address`},
		},
		{
			name: "invalid tuner server url",
			env: map[string]string{
				envKeyURL:      envURL,
				envKeyTunerURL: "mirakurun:40772",
			},
			wantErrs: []string{"tuner.url (KOTATSU_TUNER_URL): must be an http:// or https:// URL"},
		},
		{
			name: "short secret key",
			env: map[string]string{
				envKeyURL:                 envURL,
				"KOTATSU_AUTH_SECRET_KEY": "too-short",
			},
			wantErrs: []string{"auth.secret_key (KOTATSU_AUTH_SECRET_KEY): must be at least 32 bytes"},
		},
		{
			name: "invalid have i been pwned url",
			env: map[string]string{
				envKeyURL:               envURL,
				"KOTATSU_AUTH_HIBP_URL": "api.pwnedpasswords.com",
			},
			wantErrs: []string{"auth.hibp.url (KOTATSU_AUTH_HIBP_URL): must be an http:// or https:// URL"},
		},
		{
			name: "invalid web push subject",
			env: map[string]string{
				envKeyURL:                  envURL,
				"KOTATSU_WEB_PUSH_SUBJECT": "admin@kotatsu.example",
			},
			wantErrs: []string{"web_push.subject (KOTATSU_WEB_PUSH_SUBJECT): must be a mailto: or https:// URI"},
		},
		{
			name: "oidc without client id and openid scope",
			env: map[string]string{
				envKeyURL: envURL, envOIDCIssuer: testIssuer, "KOTATSU_AUTH_OIDC_SCOPES": "profile",
			},
			wantErrs: []string{
				"auth.oidc.client_id (KOTATSU_AUTH_OIDC_CLIENT_ID): is required when auth.oidc.issuer is set",
				"auth.oidc.scopes (KOTATSU_AUTH_OIDC_SCOPES): must include openid",
			},
		},
		{
			name: "oidc auto_create without groups",
			env: map[string]string{
				envKeyURL: envURL, envOIDCIssuer: testIssuer, envOIDCClient: testClientID,
				"KOTATSU_AUTH_OIDC_AUTO_CREATE_ENABLED": envTrue,
			},
			wantErrs: []string{"auth.oidc.auto_create (KOTATSU_AUTH_OIDC_AUTO_CREATE): needs at least one of admin_groups, member_groups or viewer_groups"},
		},
		{
			name: "oidc auto_create without username claim",
			yaml: "auth:\n  oidc:\n    username_claim: \"\"\n",
			env: map[string]string{
				envKeyURL: envURL, envOIDCIssuer: testIssuer, envOIDCClient: testClientID,
				"KOTATSU_AUTH_OIDC_AUTO_CREATE_ENABLED": envTrue, "KOTATSU_AUTH_OIDC_AUTO_CREATE_MEMBER_GROUPS": "family",
			},
			wantErrs: []string{"auth.oidc.username_claim (KOTATSU_AUTH_OIDC_USERNAME_CLAIM): is required when auto_create is enabled"},
		},
		{
			name: "invalid oidc issuer",
			env: map[string]string{
				envKeyURL: envURL, envOIDCIssuer: "idp.example.com", envOIDCClient: testClientID,
			},
			wantErrs: []string{"auth.oidc.issuer (KOTATSU_AUTH_OIDC_ISSUER): must be an http:// or https:// URL"},
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
			name: "invalid pool ID and relative mount point",
			yaml: "storage:\n  pools:\n    Main: /mnt/main\n    sub: mnt/sub\n",
			env:  map[string]string{envKeyURL: envURL},
			wantErrs: []string{
				`storage.pools (KOTATSU_STORAGE_POOLS): pool ID "Main" must consist of`,
				`storage.pools (KOTATSU_STORAGE_POOLS): mount point of pool "sub" must be an absolute path`,
			},
		},
		{
			name: "malformed pools in env",
			env: map[string]string{
				envKeyURL:   envURL,
				envKeyPools: "main:/mnt/main",
			},
			wantErrs: []string{"storage.pools (KOTATSU_STORAGE_POOLS): must be a comma-separated list of key=value"},
		},
		{
			name: "duplicated pool in env",
			env: map[string]string{
				envKeyURL:   envURL,
				envKeyPools: "main=/a,main=/b",
			},
			wantErrs: []string{`storage.pools (KOTATSU_STORAGE_POOLS): key "main" is given more than once`},
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

func TestTunerURLErrorDoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	// パスワードに "%zz" のような不正なエスケープがあると、url.Parse は URL の全文を含むエラーを返す。
	tunerURL := "http://user:" + leakedWord + "%zz@mirakurun:40772/"

	_, err := config.Load("", lookup(map[string]string{envKeyURL: envURL, envKeyTunerURL: tunerURL}))
	require.ErrorContains(t, err, "tuner.url (KOTATSU_TUNER_URL): must be a valid URL")
	assert.NotContains(t, err.Error(), leakedWord)
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
