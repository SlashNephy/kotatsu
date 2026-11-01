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
	"regexp"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/lib/config"
)

const (
	yamlURL = "postgres://yaml@db/kotatsu"
	envURL  = "postgres://env@db/kotatsu"
	fileURL = "postgres://file@db/kotatsu"

	envKeyURL   = "KOTATSU_DATABASE_URL"
	envKeyPools = "KOTATSU_STORAGE_POOLS"
	// envKeyDataDir と envKeyTimeshiftDir は、データディレクトリとタイムシフト用のディレクトリを渡す環境変数。
	envKeyDataDir      = "KOTATSU_STORAGE_DATA_DIR"
	envKeyTimeshiftDir = "KOTATSU_STORAGE_TIMESHIFT_DIR"
	// poolMain は、プールの設定を確かめるテストで使うプール ID。
	poolMain = "main"
	// envKeyTunerURL は、チューナーサーバーの URL を渡す環境変数。
	envKeyTunerURL = "KOTATSU_TUNER_URL"
	// envKeyArmURL は、作品 ID の対応を取り込む URL を渡す環境変数。
	envKeyArmURL = "KOTATSU_ANIME_LINKING_ARM_URL"
	// errTimeshiftOverlapsDataDir と errTimeshiftOverlapsMain は、タイムシフトのディレクトリが、データディレクトリかプール main と重なるときのエラー。
	errTimeshiftOverlapsDataDir = "storage.timeshift_dir (KOTATSU_STORAGE_TIMESHIFT_DIR): must not be the same as, inside, or a parent of storage.data_dir"
	errTimeshiftOverlapsMain    = `storage.timeshift_dir (KOTATSU_STORAGE_TIMESHIFT_DIR): must not be the same as, inside, or a parent of the mount point of pool "main"`
	// leakedWord は、エラーに漏れてはいけない、URL に含めた資格情報の一部。
	leakedWord = "hunter2"

	// Default の既定値。:9090 は設計書の付録 A で定めている。
	defaultServerListen  = ":8080"
	defaultMetricsListen = ":9090"
	// 設計書の付録 A の初期値。
	defaultAuditRetentionDays         = 365
	defaultOTLPMetricsIntervalSeconds = 60

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
	return config.Storage{Pools: map[string]string{}, PoolOrder: []string{}}
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				TimeZone:  config.DefaultTimeZone,
				Server:    config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				Server:   config.Server{Listen: "127.0.0.1:3000", AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
				Metrics:  config.Metrics{Listen: defaultMetricsListen},
				Database: config.Database{URL: envURL, Pooler: false},
				Audit:    config.Audit{RetentionDays: 30},
				Storage:  noPools(),
				Tuner:    config.Tuner{URL: ""},
			},
		},
		{
			name: "networks from yaml and env",
			yaml: "server:\n  allowed_networks: [0.0.0.0/0]\n",
			env: map[string]string{
				envKeyURL:                         envURL,
				"KOTATSU_SERVER_TRUSTED_PROXIES":  " 10.0.0.1, 10.0.1.0/24 ,",
				"KOTATSU_SERVER_CLIENT_IP_HEADER": "X-Real-IP",
				"KOTATSU_SERVER_ALLOWED_HOSTS":    "tv.example.com, kotatsu.local",
			},
			want: &config.Config{
				Server: config.Server{
					Listen: defaultServerListen, AllowedNetworks: []string{"0.0.0.0/0"}, TrustedProxies: []string{"10.0.0.1", "10.0.1.0/24"},
					ClientIPHeader: config.ClientIPHeaderXRealIP, AllowedHosts: []string{"tv.example.com", "kotatsu.local"},
				},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
				Server:   config.Server{Listen: defaultServerListen, AllowedNetworks: config.DefaultAllowedNetworks(), TrustedProxies: []string{}, ClientIPHeader: config.ClientIPHeaderXForwardedFor, AllowedHosts: []string{}},
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
			// OTLP のメトリクスの設定は TestLoadTelemetry で確かめるので、ここでは既定値のままであることだけを確かめる。
			tt.want.Telemetry.OTLPMetricsProtocol = config.OTLPProtocolGRPC
			tt.want.Telemetry.OTLPMetricsIntervalSeconds = defaultOTLPMetricsIntervalSeconds
			tt.want.Telemetry.OTLPMetricsExclude = []string{}
			// タイムゾーンは、この表では変えないので、既定値になる。読み込みは TestLoadTimeZone で確かめる。
			want := *tt.want
			want.TimeZone = config.DefaultTimeZone
			assert.Equal(t, &want, cfg)
		})
	}
}

// TestLoadStorage は、保存先プールとデータディレクトリを、YAML と環境変数から読むことを確かめる。
func TestLoadStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		env  map[string]string
		want config.Storage
	}{
		{
			name: "storage pools in yaml",
			// 書いた順を、ID の昇順ではなく、そのまま PoolOrder に残す。
			yaml: "storage:\n  pools:\n    sub-2: /mnt/sub\n    main: /mnt/main\n  data_dir: /var/lib/kotatsu\n",
			env:  map[string]string{envKeyURL: envURL},
			want: config.Storage{
				Pools: map[string]string{poolMain: "/mnt/main", "sub-2": "/mnt/sub"}, PoolOrder: []string{"sub-2", poolMain}, DataDir: "/var/lib/kotatsu",
			},
		},
		{
			name: "env replaces storage pools in yaml",
			yaml: "storage:\n  pools:\n    main: /mnt/main\n",
			env: map[string]string{
				envKeyURL:     envURL,
				envKeyPools:   "main=/data/main, archive=/data/archive",
				envKeyDataDir: "/data/kotatsu",
			},
			// 環境変数は設定ファイルの対応を丸ごと置き換えるので、順も環境変数に書いた順になる。
			want: config.Storage{
				Pools: map[string]string{poolMain: "/data/main", "archive": "/data/archive"}, PoolOrder: []string{poolMain, "archive"}, DataDir: "/data/kotatsu",
			},
		},
		{
			name: "env sets the timeshift directory",
			env: map[string]string{
				envKeyURL:          envURL,
				envKeyDataDir:      "/srv/kotatsu",
				envKeyTimeshiftDir: "/data/timeshift",
			},
			want: config.Storage{
				Pools: map[string]string{}, PoolOrder: []string{}, DataDir: "/srv/kotatsu", TimeshiftDir: "/data/timeshift",
			},
		},
		{
			// 名前の先頭が同じでも、別のディレクトリなら重ならない。
			name: "timeshift directory next to the pool and the data directory",
			yaml: "storage:\n  pools:\n    main: /mnt/main\n  data_dir: /mnt/main-data\n  timeshift_dir: /mnt/main2\n",
			env:  map[string]string{envKeyURL: envURL},
			want: config.Storage{
				Pools: map[string]string{poolMain: "/mnt/main"}, PoolOrder: []string{poolMain}, DataDir: "/mnt/main-data", TimeshiftDir: "/mnt/main2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.Load(writeFile(t, t.TempDir(), "config.yaml", nativePaths(tt.yaml)), lookup(nativeEnv(tt.env)))
			require.NoError(t, err)
			want := tt.want
			want.Pools = nativeEnv(want.Pools)
			want.DataDir, want.TimeshiftDir = nativePaths(want.DataDir), nativePaths(want.TimeshiftDir)
			assert.Equal(t, want, cfg.Storage)
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

func TestLoadTimeZone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		env     map[string]string
		want    string
		wantErr string
	}{
		{name: "default time zone", env: map[string]string{envKeyURL: envURL}, want: "Asia/Tokyo"},
		{name: "time zone in yaml", yaml: "time_zone: Europe/London\n", env: map[string]string{envKeyURL: envURL}, want: "Europe/London"},
		{name: "time zone in env", env: map[string]string{envKeyURL: envURL, "KOTATSU_TIME_ZONE": "America/New_York"}, want: "America/New_York"},
		{
			name: "unknown", env: map[string]string{envKeyURL: envURL, "KOTATSU_TIME_ZONE": "Asia/Edo"},
			wantErr: "time_zone (KOTATSU_TIME_ZONE): must be an IANA time zone name such as Asia/Tokyo",
		},
		{
			// time.LoadLocation は空を UTC とみなすが、書き忘れと見分けられないので誤りにする。
			name: "empty", yaml: "time_zone: \"\"\n", env: map[string]string{envKeyURL: envURL},
			wantErr: "time_zone (KOTATSU_TIME_ZONE): must be an IANA time zone name such as Asia/Tokyo",
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
			location, err := cfg.Location()
			require.NoError(t, err)
			assert.Equal(t, tt.want, location.String())
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

// TestLoadAnimeLinking は、作品 ID の対応を取り込む URL を、YAML、環境変数、ファイルから読み、誤りを起動時に知らせることを確かめる (設計書 3.8)。
func TestLoadAnimeLinking(t *testing.T) {
	t.Parallel()

	const armURL = "https://example.com/private/arm.json?token=abc"
	dir := t.TempDir()
	tests := []struct {
		name    string
		yaml    string
		env     map[string]string
		want    string
		wantErr string
	}{
		{name: "既定では、空にして kawaiioverflow/arm を使う", env: map[string]string{envKeyURL: envURL}, want: ""},
		{name: "YAML から読む", yaml: "anime_linking:\n  arm_url: " + armURL + "\n", env: map[string]string{envKeyURL: envURL}, want: armURL},
		{name: "環境変数から読む", env: map[string]string{envKeyURL: envURL, envKeyArmURL: armURL}, want: armURL},
		{
			name: "ファイルから読む",
			env:  map[string]string{envKeyURL: envURL, envKeyArmURL + "_FILE": writeFile(t, dir, "arm_url", armURL+"\n")},
			want: armURL,
		},
		{
			name:    "http でも https でもない URL は拒む",
			env:     map[string]string{envKeyURL: envURL, envKeyArmURL: "example.com/arm.json"},
			wantErr: "anime_linking.arm_url (KOTATSU_ANIME_LINKING_ARM_URL): must be an http:// or https:// URL",
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
			assert.Equal(t, tt.want, cfg.AnimeLinking.ArmURL.Value())
		})
	}
}

// TestLoadTelemetry は、OTLP でメトリクスを送る設定を、YAML、環境変数、ファイルから読み、誤りを起動時に知らせることを確かめる (#962)。
func TestLoadTelemetry(t *testing.T) {
	t.Parallel()

	headersFile := writeFile(t, t.TempDir(), "headers", "Mackerel-Api-Key=xxx\n")
	tests := []struct {
		name        string
		yaml        string
		env         map[string]string
		want        config.Telemetry
		wantHeaders map[string]string
		wantErr     string
	}{
		{
			name: "metrics defaults", env: map[string]string{envKeyURL: envURL},
			want: config.Telemetry{
				OTLPEndpoint: "", OTLPMetricsEndpoint: "", OTLPMetricsProtocol: config.OTLPProtocolGRPC, OTLPMetricsHeaders: "",
				OTLPMetricsIntervalSeconds: defaultOTLPMetricsIntervalSeconds, OTLPMetricsExclude: []string{},
			},
			wantHeaders: map[string]string{},
		},
		{
			name: "metrics env",
			env: map[string]string{
				envKeyURL: envURL,
				"KOTATSU_TELEMETRY_OTLP_METRICS_ENDPOINT":         "https://otlp.mackerelio.com:4317",
				"KOTATSU_TELEMETRY_OTLP_METRICS_PROTOCOL":         "http/protobuf",
				"KOTATSU_TELEMETRY_OTLP_METRICS_INTERVAL_SECONDS": "30",
				"KOTATSU_TELEMETRY_OTLP_METRICS_EXCLUDE":          "go_*,process_*",
				"KOTATSU_TELEMETRY_OTLP_METRICS_HEADERS_FILE":     headersFile,
			},
			want: config.Telemetry{
				OTLPEndpoint: "", OTLPMetricsEndpoint: "https://otlp.mackerelio.com:4317", OTLPMetricsProtocol: config.OTLPProtocolHTTP,
				OTLPMetricsHeaders: "Mackerel-Api-Key=xxx", OTLPMetricsIntervalSeconds: 30, OTLPMetricsExclude: []string{"go_*", "process_*"},
			},
			wantHeaders: map[string]string{"Mackerel-Api-Key": "xxx"},
		},
		{
			name: "metrics yaml",
			yaml: "telemetry:\n  otlp_metrics_endpoint: http://collector:4317\n  otlp_metrics_exclude: [go_gc_duration_seconds]\n",
			env:  map[string]string{envKeyURL: envURL},
			want: config.Telemetry{
				OTLPEndpoint: "", OTLPMetricsEndpoint: "http://collector:4317", OTLPMetricsProtocol: config.OTLPProtocolGRPC, OTLPMetricsHeaders: "",
				OTLPMetricsIntervalSeconds: defaultOTLPMetricsIntervalSeconds, OTLPMetricsExclude: []string{"go_gc_duration_seconds"},
			},
			wantHeaders: map[string]string{},
		},
		{
			name: "invalid endpoint", env: map[string]string{envKeyURL: envURL, "KOTATSU_TELEMETRY_OTLP_METRICS_ENDPOINT": "ftp://x"},
			wantErr: "telemetry.otlp_metrics_endpoint (KOTATSU_TELEMETRY_OTLP_METRICS_ENDPOINT): must be an http:// or https:// URL",
		},
		{
			name: "invalid protocol", env: map[string]string{envKeyURL: envURL, "KOTATSU_TELEMETRY_OTLP_METRICS_PROTOCOL": "udp"},
			wantErr: "telemetry.otlp_metrics_protocol (KOTATSU_TELEMETRY_OTLP_METRICS_PROTOCOL): must be grpc or http/protobuf",
		},
		{
			name: "invalid headers", env: map[string]string{envKeyURL: envURL, "KOTATSU_TELEMETRY_OTLP_METRICS_HEADERS": "novalue"},
			wantErr: "telemetry.otlp_metrics_headers (KOTATSU_TELEMETRY_OTLP_METRICS_HEADERS):",
		},
		{
			name: "invalid interval", env: map[string]string{envKeyURL: envURL, "KOTATSU_TELEMETRY_OTLP_METRICS_INTERVAL_SECONDS": "0"},
			wantErr: "telemetry.otlp_metrics_interval_seconds (KOTATSU_TELEMETRY_OTLP_METRICS_INTERVAL_SECONDS): must be positive",
		},
		{
			name: "invalid exclude pattern", env: map[string]string{envKeyURL: envURL, "KOTATSU_TELEMETRY_OTLP_METRICS_EXCLUDE": "["},
			wantErr: `telemetry.otlp_metrics_exclude (KOTATSU_TELEMETRY_OTLP_METRICS_EXCLUDE): "[" is not a valid pattern`,
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
			assert.Equal(t, tt.want, cfg.Telemetry)
			headers, err := cfg.Telemetry.OTLPMetricsHeaderMap()
			require.NoError(t, err)
			assert.Equal(t, tt.wantHeaders, headers)
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
			name: "invalid client ip header",
			env: map[string]string{
				envKeyURL:                         envURL,
				"KOTATSU_SERVER_CLIENT_IP_HEADER": "Forwarded",
			},
			wantErrs: []string{`server.client_ip_header (KOTATSU_SERVER_CLIENT_IP_HEADER): must be X-Forwarded-For or X-Real-IP`},
		},
		{
			name: "allowed host with a port",
			env: map[string]string{
				envKeyURL:                      envURL,
				"KOTATSU_SERVER_ALLOWED_HOSTS": "tv.example.com:443",
			},
			wantErrs: []string{`server.allowed_hosts (KOTATSU_SERVER_ALLOWED_HOSTS): "tv.example.com:443" must be a host name without a scheme, port, or path`},
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
			yaml: "storage:\n  pools:\n    Main: /mnt/main\n    sub: mnt/sub\n  data_dir: /var/lib/kotatsu\n",
			env:  map[string]string{envKeyURL: envURL},
			wantErrs: []string{
				`storage.pools (KOTATSU_STORAGE_POOLS): pool ID "Main" must consist of`,
				`storage.pools (KOTATSU_STORAGE_POOLS): mount point of pool "sub" must be an absolute path`,
			},
		},
		{
			name:     "pools without the data directory",
			yaml:     "storage:\n  pools:\n    main: /mnt/main\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{"storage.data_dir (KOTATSU_STORAGE_DATA_DIR): is required when storage.pools is set"},
		},
		{
			name:     "relative data directory",
			yaml:     "storage:\n  pools:\n    main: /mnt/main\n  data_dir: var/lib/kotatsu\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{"storage.data_dir (KOTATSU_STORAGE_DATA_DIR): must be an absolute path"},
		},
		{
			name:     "relative timeshift directory",
			yaml:     "storage:\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: var/timeshift\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{"storage.timeshift_dir (KOTATSU_STORAGE_TIMESHIFT_DIR): must be an absolute path"},
		},
		{
			name:     "timeshift directory without the data directory",
			yaml:     "storage:\n  timeshift_dir: /var/timeshift\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{"storage.timeshift_dir (KOTATSU_STORAGE_TIMESHIFT_DIR): requires storage.data_dir"},
		},
		{
			name:     "timeshift directory same as the data directory",
			yaml:     "storage:\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: /var/lib/kotatsu/\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsDataDir},
		},
		{
			name:     "timeshift directory inside the data directory",
			yaml:     "storage:\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: /var/lib/kotatsu/timeshift\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsDataDir},
		},
		{
			name:     "timeshift directory containing the data directory",
			yaml:     "storage:\n  data_dir: /var/lib/kotatsu/data\n  timeshift_dir: /var/lib/kotatsu/../kotatsu\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsDataDir},
		},
		{
			name:     "timeshift directory same as a pool",
			yaml:     "storage:\n  pools:\n    main: /mnt/main\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: /mnt/main\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsMain},
		},
		{
			name:     "timeshift directory inside a pool",
			yaml:     "storage:\n  pools:\n    main: /mnt/main\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: /mnt/main/timeshift\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsMain},
		},
		{
			name:     "root timeshift directory containing everything",
			yaml:     "storage:\n  pools:\n    main: /mnt/main\n    sub: /srv/sub\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: /\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsDataDir},
		},
		{
			name:     "timeshift directory containing a pool but not the data directory",
			yaml:     "storage:\n  pools:\n    main: /mnt/main\n    sub: /mnt/sub\n  data_dir: /var/lib/kotatsu\n  timeshift_dir: /mnt\n",
			env:      map[string]string{envKeyURL: envURL},
			wantErrs: []string{errTimeshiftOverlapsMain},
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
				path = writeFile(t, t.TempDir(), "config.yaml", nativePaths(tt.yaml))
			}

			_, err := config.Load(path, lookup(nativeEnv(tt.env)))
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

// TestSecretURLErrorDoesNotLeakSecret は、秘密情報として扱う URL が不正なときに、エラーに URL を含めないことを確かめる。
func TestSecretURLErrorDoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		envKey string
		want   string
	}{
		{envKey: envKeyTunerURL, want: "tuner.url (KOTATSU_TUNER_URL): must be a valid URL"},
		{envKey: envKeyArmURL, want: "anime_linking.arm_url (KOTATSU_ANIME_LINKING_ARM_URL): must be a valid URL"},
	}
	for _, tt := range tests {
		t.Run(tt.envKey, func(t *testing.T) {
			t.Parallel()

			// パスワードに "%zz" のような不正なエスケープがあると、url.Parse は URL の全文を含むエラーを返す。
			secretURL := "http://user:" + leakedWord + "%zz@example.com:40772/"

			_, err := config.Load("", lookup(map[string]string{envKeyURL: envURL, tt.envKey: secretURL}))
			require.ErrorContains(t, err, tt.want)
			assert.NotContains(t, err.Error(), leakedWord)
		})
	}
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

// TestLoadRejectsTimeshiftDirectoryReachedByAnotherPath は、タイムシフト用のディレクトリが、名前の上では重ならなくても、
// ファイルシステムが同じディレクトリとみなすものの中にあれば、拒否することを確かめる (#1097)。
// 大文字と小文字を区別しない NTFS や APFS では、大文字と小文字だけが違うパスが同じディレクトリになる。その代わりに、シンボリックリンクを置く。
func TestLoadRejectsTimeshiftDirectoryReachedByAnotherPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	pool := filepath.Join(root, "main")
	require.NoError(t, os.Mkdir(pool, 0o755))
	require.NoError(t, os.Symlink(pool, filepath.Join(root, "alias")))

	tests := []struct {
		name         string
		timeshiftDir string
		wantErr      bool
	}{
		{name: "プールの中を別のパスで指す", timeshiftDir: filepath.Join(root, "alias", "timeshift"), wantErr: true},
		{name: "プールそのものを別のパスで指す", timeshiftDir: filepath.Join(root, "alias"), wantErr: true},
		{name: "重ならない", timeshiftDir: filepath.Join(root, "timeshift"), wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load("", lookup(map[string]string{
				envKeyURL:          envURL,
				envKeyPools:        "main=" + pool,
				envKeyDataDir:      filepath.Join(root, "data"),
				envKeyTimeshiftDir: tt.timeshiftDir,
			}))
			if tt.wantErr {
				require.ErrorContains(t, err, errTimeshiftOverlapsMain)

				return
			}
			require.NoError(t, err)
		})
	}
}

// rootedPath は、テストの値の中の、/ から始まる絶対パス。
var rootedPath = regexp.MustCompile(`(^|[\s=,])/`)

// nativePaths は、テストの値の中の、/ から始まる絶対パスを、この OS の絶対パスにする。
// Windows では、/mnt/main はドライブ文字がないので絶対パスにならない。C: を前に付ける。ほかの OS では、そのまま返す。
func nativePaths(s string) string {
	if runtime.GOOS != "windows" {
		return s
	}

	return rootedPath.ReplaceAllString(s, "${1}C:/")
}

// nativeEnv は、値の中の絶対パスを nativePaths で直した写しを返す。
func nativeEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	native := make(map[string]string, len(env))
	for k, v := range env {
		native[k] = nativePaths(v)
	}

	return native
}
