// Package config は、YAML の設定ファイルと KOTATSU_ で始まる環境変数から設定を読み込む。
package config

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
)

// Config は起動に必要な設定。
//
// 各項目は、YAML のキーを _ でつないで大文字にした環境変数でも指定できる
// (database.url は KOTATSU_DATABASE_URL)。Secret 型の項目は、名前が _FILE で終わる
// 環境変数でファイルのパスを指定しても渡せる。
//
// required:"true" を付けた項目は、どこからも値が与えられなければ起動を止める。
type Config struct {
	Server    Server    `yaml:"server"`
	Metrics   Metrics   `yaml:"metrics"`
	Database  Database  `yaml:"database"`
	Telemetry Telemetry `yaml:"telemetry"`
	Audit     Audit     `yaml:"audit"`
	Storage   Storage   `yaml:"storage"`
	Tuner     Tuner     `yaml:"tuner"`
}

// Server は tenban の HTTP サーバーの設定。
type Server struct {
	// Listen は待ち受けアドレス。
	Listen string `yaml:"listen"`
}

// Metrics は /metrics の設定。
type Metrics struct {
	// Listen は /metrics の待ち受けアドレス。API とは別のアドレスで公開する。
	Listen string `yaml:"listen"`
}

// Database は PostgreSQL への接続の設定。
type Database struct {
	// URL は接続先。パスワードを含みうるので秘密情報として扱う。
	URL Secret `required:"true" yaml:"url"`
	// Pooler は、PgBouncer などのトランザクションプーリングを経由して接続するかどうか。
	// 経由する場合は LISTEN が使えないので、変更通知やジョブの取得をポーリングだけで行う。
	Pooler bool `yaml:"pooler"`
}

// Telemetry は OpenTelemetry の設定。
type Telemetry struct {
	// OTLPEndpoint は、トレースを送る OTLP/HTTP の送信先のベースの URL (http://collector:4318 など)。
	// OTEL_EXPORTER_OTLP_ENDPOINT と同じく、トレースは /v1/traces を付けた URL に送る。
	// 空ならトレースを送らない。
	OTLPEndpoint string `yaml:"otlp_endpoint"`
}

// Audit は監査ログの設定。
type Audit struct {
	// RetentionDays は監査ログの保持期間 (日)。これを過ぎた記録は定期的に消す。
	RetentionDays int `yaml:"retention_days"`
}

// Storage は保存先プール (kago) の設定 (設計書 5.6)。
type Storage struct {
	// Pools は、プール ID からマウント先のディレクトリへの対応。
	// 録画のパスは「プール ID ＋ プール内の相対パス」で DB に記録するので、ロールごとにマウント先が違っていてもよい。
	// 環境変数では、KOTATSU_STORAGE_POOLS=main=/mnt/main,sub=/mnt/sub のように「ID=パス」をカンマで区切って渡す。
	Pools map[string]string `yaml:"pools"`
}

// Tuner はチューナーサーバーの設定 (設計書 3.1)。
type Tuner struct {
	// URL はチューナーサーバーの URL (http://mirakurun:40772/ など)。BASIC 認証の資格情報を含めてよい。
	// 資格情報を含みうるので秘密情報として扱う。空なら、neko は EPG を同期しない。
	URL Secret `yaml:"url"`
}

// Default は既定値を入れた設定を返す。
func Default() *Config {
	return &Config{
		Server: Server{
			Listen: ":8080",
		},
		Metrics: Metrics{
			Listen: ":9090",
		},
		Database: Database{
			URL:    "",
			Pooler: false,
		},
		Telemetry: Telemetry{
			OTLPEndpoint: "",
		},
		Audit: Audit{
			// 設計書の付録 A の初期値。
			RetentionDays: 365,
		},
		Storage: Storage{
			Pools: map[string]string{},
		},
		Tuner: Tuner{
			URL: "",
		},
	}
}

// validate は値の形式を検査する。必須の項目の検査は読み込みの段階で済ませている。
func (c *Config) validate() error {
	errs := []error{}
	if err := validateListen(c.Server.Listen); err != nil {
		errs = append(errs, fieldError("server.listen", err))
	}
	if err := validateListen(c.Metrics.Listen); err != nil {
		errs = append(errs, fieldError("metrics.listen", err))
	}
	if err := validateDatabaseURL(c.Database.URL); err != nil {
		errs = append(errs, fieldError("database.url", err))
	}
	if err := validateOTLPEndpoint(c.Telemetry.OTLPEndpoint); err != nil {
		errs = append(errs, fieldError("telemetry.otlp_endpoint", err))
	}
	if c.Audit.RetentionDays < 1 {
		errs = append(errs, fieldError("audit.retention_days", errors.New("must be at least 1")))
	}
	for _, id := range slices.Sorted(maps.Keys(c.Storage.Pools)) {
		if err := validatePool(id, c.Storage.Pools[id]); err != nil {
			errs = append(errs, fieldError("storage.pools", err))
		}
	}

	return errors.Join(errs...)
}

func validateListen(addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("must be in the form host:port: %w", err)
	}

	return nil
}

// poolIDPattern は、プール ID に使える文字。DB に記録し、環境変数でも渡すので、記号は - と _ に限る。
var poolIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func validatePool(id, mount string) error {
	if !poolIDPattern.MatchString(id) {
		return fmt.Errorf("pool ID %q must consist of lowercase letters, digits, - and _ (up to 63 characters)", id)
	}
	if !filepath.IsAbs(mount) {
		return fmt.Errorf("mount point of pool %q must be an absolute path", id)
	}

	return nil
}

func validateDatabaseURL(secret Secret) error {
	// url.Parse のエラーは URL の全文を含むので、秘密情報を漏らさないよう理由だけを返す。
	u, err := url.Parse(secret.Value())
	if err != nil {
		return errors.New("must be a valid URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return errors.New("must use the postgres:// or postgresql:// scheme")
	}

	return nil
}

func validateOTLPEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("must be a valid URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("must be an http:// or https:// URL")
	}

	return nil
}
