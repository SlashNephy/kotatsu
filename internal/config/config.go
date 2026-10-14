// Package config は、YAML の設定ファイルと KOTATSU_ で始まる環境変数から設定を読み込む。
package config

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
	Auth      Auth      `yaml:"auth"`
	WebPush   WebPush   `yaml:"web_push"`
	Thumbnail Thumbnail `yaml:"thumbnail"`
	Kyusu     Kyusu     `yaml:"kyusu"`
	Jikkyo    Jikkyo    `yaml:"jikkyo"`
}

// Server は tenban の HTTP サーバーの設定。
type Server struct {
	// Listen は待ち受けアドレス。
	Listen string `yaml:"listen"`
	// AllowedNetworks は、接続を受け付ける送信元のネットワーク (CIDR か IP アドレス) (設計書 9.5)。
	// 既定はプライベートアドレスだけで、インターネットから受け付けるには 0.0.0.0/0 と ::/0 を明示する。
	// 環境変数では、カンマで区切って渡す。
	AllowedNetworks []string `yaml:"allowed_networks"`
	// TrustedProxies は、X-Forwarded-For などを信頼するリバースプロキシのネットワーク (CIDR か IP アドレス)。
	// ここに含まれない送信元から X-Forwarded-For が届いたら、設定方法を案内するエラーを返す。
	TrustedProxies []string `yaml:"trusted_proxies"`
	// AdvertiseHost は、インスタンスの登録簿に載せる、ほかのインスタンスからこのインスタンスに届くホスト (設計書 1.2)。
	// 空なら、待ち受けのホストを使い、全アドレスで待ち受けるならホスト名を使う。
	// Kubernetes では、ホスト名で Pod に届かないので、Pod の IP を渡す。
	AdvertiseHost string `yaml:"advertise_host"`
}

// DefaultAllowedNetworks は、接続を受け付ける送信元の既定値 (設計書 9.5)。
// RFC 1918 のアドレス、ループバック、IPv6 のユニークローカルアドレスとリンクローカルアドレス、Tailscale が使う 100.64.0.0/10。
func DefaultAllowedNetworks() []string {
	return []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "100.64.0.0/10",
		"::1/128", "fc00::/7", "fe80::/10",
	}
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
	// PoolOrder は、Pools を書いた順のプール ID。優先順位が同じプールは、この順に選ぶ (設計書 5.6)。
	// 対応 (map) では順が失われるので、読み込むときに設定ファイルか環境変数から求める。
	PoolOrder []string `yaml:"-"`
	// SidecarDir は、新しい録画の付属ファイル (索引、代表画像、プレビュー画像) をまとめて置くディレクトリ (設計書 5.6)。
	// 空なら、今までどおり録画ファイルの横 (番組名.m2ts.kotatsu/) に置く。すでにある付属ファイルは移さない。
	// 付属ファイルを読み書きするロール (mikan、nekonote、tenban) で、同じディレクトリをマウントする。
	SidecarDir string `yaml:"sidecar_dir"`
}

// Tuner はチューナーサーバーの設定 (設計書 3.1)。
type Tuner struct {
	// URL はチューナーサーバーの URL (http://mirakurun:40772/ など)。BASIC 認証の資格情報を含めてよい。
	// 資格情報を含みうるので秘密情報として扱う。空なら、neko は EPG を同期しない。
	URL Secret `yaml:"url"`
}

// Thumbnail は、録画の代表画像とシークバー用のプレビュー画像の設定 (設計書 6.1)。nekonote が使う。
// 画像は FFmpeg で作るので、FFmpeg の入ったイメージ (-ffmpeg) を使うか、PATH に FFmpeg を入れる。
type Thumbnail struct {
	// DisablePreview は、プレビュー画像を作らないかどうか。低スペック機では、代表画像だけにして負荷を減らせる。
	DisablePreview bool `yaml:"disable_preview"`
}

// kyusu が映像のエンコードに使うエンコーダー (設計書 7.3)。
const (
	// EncoderX264 は、ソフトウェアの x264 (HEVC では x265)。GPU がなくても動く。
	EncoderX264 = "x264"
	// EncoderVAAPI は、VAAPI (Intel と AMD の GPU)。
	EncoderVAAPI = "vaapi"
	// EncoderQSV は、Intel Quick Sync Video。
	EncoderQSV = "qsv"
	// EncoderNVENC は、NVIDIA NVENC。
	EncoderNVENC = "nvenc"
)

// Jikkyo は、ニコニコ実況の設定 (設計書 7.4)。
type Jikkyo struct {
	// Channels は、同梱した対応表に重ねる、サービスと実況のチャンネルの対応。
	// キーは「ネットワーク ID:サービス ID」、値は jk1 などの実況のチャンネルの ID。
	// 環境変数では、4:101=jk101,32736:1024=jk1 のようにカンマで区切って渡す。
	Channels map[string]string `yaml:"channels"`
}

// Kyusu は、kyusu のトランスコードの設定 (設計書 7.3)。
// kyusu は FFmpeg と tsreadex を使うので、FFmpeg の入ったイメージ (-ffmpeg) を使うか、PATH に入れる。
type Kyusu struct {
	// Encoder は、映像のエンコーダー (x264、vaapi、qsv、nvenc のどれか)。既定は x264。
	Encoder string `yaml:"encoder"`
	// VAAPIDevice は、VAAPI で使うデバイス。
	VAAPIDevice string `yaml:"vaapi_device"`
	// WorkDir は、HLS のセグメントを置く一時領域。Kubernetes では emptyDir を指す。空なら、OS の一時ディレクトリの下に作る。
	WorkDir string `yaml:"work_dir"`
}

// WebPush は Web Push の通知の設定 (設計書 6.2)。
type WebPush struct {
	// Subject は、プッシュサービスに示す、送り手の連絡先 (mailto: か https: の URI) (RFC 8292)。
	// プッシュサービスは、送り手に問題があるときに、ここに連絡する。空なら、Kotatsu のリポジトリの URL にする。
	Subject string `yaml:"subject"`
}

// Auth は認証の設定 (設計書 9.3)。
type Auth struct {
	// SecretKey はアプリの秘密キー (設計書 8.4)。TOTP の秘密キーなどを暗号化して DB に保存するのに使う。
	// 32 バイト以上のランダムな文字列にする (openssl rand -base64 48 など)。tenban は、これがないと起動しない。
	// 変えると、暗号化して保存した値を読めなくなる。
	SecretKey Secret `yaml:"secret_key"`
	HIBP      HIBP   `yaml:"hibp"`
	OIDC      OIDC   `yaml:"oidc"`
}

// OIDC は、OpenID Connect の IdP でのログインの設定 (設計書 9.4)。Issuer が空なら、OIDC でのログインを受け付けない。
type OIDC struct {
	// Issuer は IdP の issuer の URL。/.well-known/openid-configuration から、エンドポイントを引く。
	Issuer       string `yaml:"issuer"`
	ClientID     string `yaml:"client_id"`
	ClientSecret Secret `yaml:"client_secret"`
	// RedirectURL は、IdP に登録したリダイレクト URI。空なら、接続したオリジンに /auth/oidc/callback を付けたものにする。
	RedirectURL string `yaml:"redirect_url"`
	// Scopes は、IdP に求めるスコープ。openid は必ず含める。環境変数では、カンマで区切って渡す。
	Scopes []string `yaml:"scopes"`
	// DisplayName は、ログインの画面のボタンに出す IdP の名前。
	DisplayName string `yaml:"display_name"`
	// UsernameClaim は、自動で作るユーザーのユーザー名にする claim。
	UsernameClaim string     `yaml:"username_claim"`
	AutoCreate    AutoCreate `yaml:"auto_create"`
}

// AutoCreate は、OIDC で初めてログインしたユーザーを自動で作る設定 (設計書 9.4)。
//
// 有効にしても、GroupsClaim のグループのどれかが、AdminGroups、MemberGroups、ViewerGroups のどれかに含まれる
// ユーザーだけを作る。ロールは、含まれる中で最も強いものにする。ロールは作るときだけ決め、後のログインでは変えない。
type AutoCreate struct {
	Enabled bool `yaml:"enabled"`
	// GroupsClaim は、ユーザーのグループを入れた claim。文字列の配列か、1 つの文字列を受け付ける。
	GroupsClaim string `yaml:"groups_claim"`
	// 環境変数では、カンマで区切って渡す。
	AdminGroups  []string `yaml:"admin_groups"`
	MemberGroups []string `yaml:"member_groups"`
	ViewerGroups []string `yaml:"viewer_groups"`
}

// HIBP は、パスワードを設定するときに、Have I Been Pwned に漏洩済みかを照会する設定。
// 外部への通信になるので、既定では照会しない (設計書 9.3)。
type HIBP struct {
	Enabled bool `yaml:"enabled"`
	// URL は Pwned Passwords の API。空なら https://api.pwnedpasswords.com にする。
	URL string `yaml:"url"`
}

// MinSecretKeyLength は、アプリの秘密キーの長さ (バイト) の下限。
const MinSecretKeyLength = 32

// Default は既定値を入れた設定を返す。
func Default() *Config {
	return &Config{
		Server: Server{
			Listen:          ":8080",
			AdvertiseHost:   "",
			AllowedNetworks: DefaultAllowedNetworks(),
			TrustedProxies:  []string{},
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
			Pools:      map[string]string{},
			PoolOrder:  []string{},
			SidecarDir: "",
		},
		Tuner: Tuner{
			URL: "",
		},
		Auth: Auth{
			SecretKey: "",
			HIBP:      HIBP{Enabled: false, URL: ""},
			OIDC: OIDC{
				Issuer: "", ClientID: "", ClientSecret: "", RedirectURL: "",
				Scopes: []string{"openid", "profile", "email"}, DisplayName: "OIDC", UsernameClaim: "preferred_username",
				AutoCreate: AutoCreate{
					Enabled: false, GroupsClaim: "groups", AdminGroups: []string{}, MemberGroups: []string{}, ViewerGroups: []string{},
				},
			},
		},
		WebPush:   WebPush{Subject: ""},
		Thumbnail: Thumbnail{DisablePreview: false},
		Kyusu:     Kyusu{Encoder: EncoderX264, VAAPIDevice: "/dev/dri/renderD128", WorkDir: ""},
		Jikkyo:    Jikkyo{Channels: map[string]string{}},
	}
}

// validate は値の形式を検査する。必須の項目の検査は読み込みの段階で済ませている。
func (c *Config) validate() error {
	errs := []error{}
	if err := validateListen(c.Server.Listen); err != nil {
		errs = append(errs, fieldError("server.listen", err))
	}
	if err := validateNetworks(c.Server.AllowedNetworks); err != nil {
		errs = append(errs, fieldError("server.allowed_networks", err))
	}
	if err := validateNetworks(c.Server.TrustedProxies); err != nil {
		errs = append(errs, fieldError("server.trusted_proxies", err))
	}
	if err := validateListen(c.Metrics.Listen); err != nil {
		errs = append(errs, fieldError("metrics.listen", err))
	}
	if err := validateDatabaseURL(c.Database.URL); err != nil {
		errs = append(errs, fieldError("database.url", err))
	}
	if err := validateTunerURL(c.Tuner.URL); err != nil {
		errs = append(errs, fieldError("tuner.url", err))
	}
	if err := validateHTTPURL(c.Telemetry.OTLPEndpoint); err != nil {
		errs = append(errs, fieldError("telemetry.otlp_endpoint", err))
	}
	if key := c.Auth.SecretKey.Value(); key != "" && len(key) < MinSecretKeyLength {
		errs = append(errs, fieldError("auth.secret_key", fmt.Errorf("must be at least %d bytes", MinSecretKeyLength)))
	}
	if err := validateHTTPURL(c.Auth.HIBP.URL); err != nil {
		errs = append(errs, fieldError("auth.hibp.url", err))
	}
	errs = append(errs, c.Auth.OIDC.validate()...)
	if subject := c.WebPush.Subject; subject != "" && !strings.HasPrefix(subject, "mailto:") && !strings.HasPrefix(subject, "https://") {
		errs = append(errs, fieldError("web_push.subject", errors.New("must be a mailto: or https:// URI")))
	}
	if !slices.Contains([]string{EncoderX264, EncoderVAAPI, EncoderQSV, EncoderNVENC}, c.Kyusu.Encoder) {
		errs = append(errs, fieldError("kyusu.encoder", errors.New("must be one of x264, vaapi, qsv, nvenc")))
	}
	if c.Audit.RetentionDays < 1 {
		errs = append(errs, fieldError("audit.retention_days", errors.New("must be at least 1")))
	}
	for _, id := range slices.Sorted(maps.Keys(c.Storage.Pools)) {
		if err := validatePool(id, c.Storage.Pools[id]); err != nil {
			errs = append(errs, fieldError("storage.pools", err))
		}
	}
	if c.Storage.SidecarDir != "" && !filepath.IsAbs(c.Storage.SidecarDir) {
		errs = append(errs, fieldError("storage.sidecar_dir", errors.New("must be an absolute path")))
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

// IsValidPoolID は、プール ID が使える文字だけでできているかどうかを返す。
// 録画設定でプールを指定するときも、同じ規則で検査する。
func IsValidPoolID(id string) bool {
	return poolIDPattern.MatchString(id)
}

func validatePool(id, mount string) error {
	if !IsValidPoolID(id) {
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

// ParseNetworks は、CIDR か IP アドレスの一覧を、ネットワークの一覧にする。IP アドレスは、そのアドレスだけのネットワークにする。
func ParseNetworks(networks []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(networks))
	for _, network := range networks {
		if prefix, err := netip.ParsePrefix(network); err == nil {
			prefixes = append(prefixes, prefix.Masked())

			continue
		}
		addr, err := netip.ParseAddr(network)
		if err != nil {
			return nil, fmt.Errorf("%q must be a CIDR or an IP address", network)
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
	}

	return prefixes, nil
}

func validateNetworks(networks []string) error {
	_, err := ParseNetworks(networks)

	return err
}

// validateTunerURL は、チューナーサーバーの URL を検査する。空なら、EPG を同期しないものとして通す。
func validateTunerURL(secret Secret) error {
	if secret.Value() == "" {
		return nil
	}

	// url.Parse のエラーは URL の全文を含むので、資格情報を漏らさないよう理由だけを返す。
	u, err := url.Parse(secret.Value())
	if err != nil {
		return errors.New("must be a valid URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("must be an http:// or https:// URL")
	}

	return nil
}

// validateHTTPURL は、空でなければ http:// か https:// の URL であることを確かめる。
func validateHTTPURL(endpoint string) error {
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

// Enabled は、OIDC でのログインを受け付けるかどうかを返す。
func (o *OIDC) Enabled() bool {
	return o.Issuer != ""
}

func (o *OIDC) validate() []error {
	errs := []error{}
	if !o.Enabled() {
		return errs
	}
	if err := validateHTTPURL(o.Issuer); err != nil {
		errs = append(errs, fieldError("auth.oidc.issuer", err))
	}
	if err := validateHTTPURL(o.RedirectURL); err != nil {
		errs = append(errs, fieldError("auth.oidc.redirect_url", err))
	}
	if o.ClientID == "" {
		errs = append(errs, fieldError("auth.oidc.client_id", errors.New("is required when auth.oidc.issuer is set")))
	}
	if !slices.Contains(o.Scopes, "openid") {
		errs = append(errs, fieldError("auth.oidc.scopes", errors.New("must include openid")))
	}
	if o.AutoCreate.Enabled {
		if o.UsernameClaim == "" {
			errs = append(errs, fieldError("auth.oidc.username_claim", errors.New("is required when auto_create is enabled")))
		}
		if o.AutoCreate.GroupsClaim == "" {
			errs = append(errs, fieldError("auth.oidc.auto_create.groups_claim", errors.New("is required when auto_create is enabled")))
		}
		if len(o.AutoCreate.AdminGroups)+len(o.AutoCreate.MemberGroups)+len(o.AutoCreate.ViewerGroups) == 0 {
			errs = append(errs, fieldError("auth.oidc.auto_create", errors.New("needs at least one of admin_groups, member_groups or viewer_groups")))
		}
	}

	return errs
}
