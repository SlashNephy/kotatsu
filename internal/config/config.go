// Package config は、YAML の設定ファイルと KOTATSU_ で始まる環境変数から設定を読み込む。
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
)

// Config は起動に必要な設定。
//
// 各項目は、YAML のキーを _ でつないで大文字にした環境変数でも指定できる
// (database.url は KOTATSU_DATABASE_URL)。Secret 型の項目は、名前が _FILE で終わる
// 環境変数でファイルのパスを指定しても渡せる。
//
// required:"true" を付けた項目は、どこからも値が与えられなければ起動を止める。
type Config struct {
	Server   Server   `yaml:"server"`
	Metrics  Metrics  `yaml:"metrics"`
	Database Database `yaml:"database"`
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

	return errors.Join(errs...)
}

func validateListen(addr string) error {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("must be in the form host:port: %w", err)
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
