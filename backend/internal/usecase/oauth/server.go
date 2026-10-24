package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
)

const (
	// AccessTokenLifetime は、アクセストークンの有効期限。
	AccessTokenLifetime = time.Hour
	// RefreshTokenLifetime は、リフレッシュトークンの有効期限。使うたびに入れ替え、そこから数え直す。
	RefreshTokenLifetime = 30 * 24 * time.Hour
	// AuthorizationCodeLifetime は、認可コードの有効期限。
	AuthorizationCodeLifetime = 10 * time.Minute
	// ResourcePath は、トークンを使える MCP サーバーのパス。
	ResourcePath = "/mcp"
	// ConsentPath は、同意の画面のパス。認可のエンドポイントは、受け取ったクエリをそのまま付けて、この画面に送る。
	ConsentPath = "/authorize"

	// accessMark、refreshMark、codeMark は、アクセストークン、リフレッシュトークン、認可コードの頭に付ける印。
	// 漏れたものを見つけやすくし、API トークン (kotatsu_) と見分けられるようにする。
	accessMark  = "kotatsu_at_"
	refreshMark = "kotatsu_rt_"
	codeMark    = "kotatsu_ac_"
	// secretBytes は、トークンと認可コードのランダムな部分の長さ。
	secretBytes = 32
	// grantTouchInterval は、連携を使った日時を書き直す間隔。
	grantTouchInterval = time.Minute

	tokenKindAccess  = "access"
	tokenKindRefresh = "refresh"

	actionGrantCreate = "oauth_grant.create"
	targetOAuthClient = "oauth_client"
)

// ErrDisabled は、管理者が MCP を有効にしていないことを表す。
var ErrDisabled = errors.New("MCP is disabled")

// Database は、読み出しと、トランザクションでの書き込みに使う DB。*pgxpool.Pool が満たす。
type Database interface {
	sqlc.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Server は、MCP のクライアントにトークンを発行する、OAuth の認可サーバー (#963、設計書 9.9)。
// 認可サーバーの issuer は、クライアントが接続したオリジンにする。LAN の中の名前と、外から接続する名前のように、
// 複数のオリジンで公開していても、それぞれのオリジンで発行したトークンは、そのオリジンの MCP サーバーでしか使えない。
type Server struct {
	db       Database
	clock    clock.Clock
	fetcher  *Fetcher
	recorder audit.Recorder
}

// NewServer は Server を返す。
func NewServer(db Database, clk clock.Clock, fetcher *Fetcher, recorder audit.Recorder) *Server {
	return &Server{db: db, clock: clk, fetcher: fetcher, recorder: recorder}
}

// Enabled は、管理者が MCP を有効にしているかどうかを返す。既定では無効。
func (s *Server) Enabled(ctx context.Context) (bool, error) {
	system, err := settings.Load(ctx, s.db)
	if err != nil {
		return false, err
	}

	return system.MCPEnabled, nil
}

// requireEnabled は、MCP が無効なら ErrDisabled を返す。
func (s *Server) requireEnabled(ctx context.Context) error {
	enabled, err := s.Enabled(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrDisabled
	}

	return nil
}

// ResourceURL は、origin の MCP サーバーの URL を返す。トークンは、この URL 宛てにだけ発行する (RFC 8707)。
func ResourceURL(origin string) string {
	return strings.TrimSuffix(origin, "/") + ResourcePath
}

// sameResource は、クライアントが送った resource が、want (ResourceURL の値) と同じ MCP サーバーを指すかどうかを返す。
// スキームとホストの大文字と小文字、末尾の / の有無は区別しない。
func sameResource(got, want string) bool {
	normalize := func(raw string) (string, bool) {
		u, err := url.Parse(raw)
		if err != nil || u.Fragment != "" || u.RawQuery != "" || u.User != nil {
			return "", false
		}

		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.EscapedPath(), "/"), true
	}
	g, ok := normalize(got)
	if !ok {
		return false
	}
	w, _ := normalize(want)

	return g == w
}

// newSecret は、印を付けたランダムなトークンを返す。
func newSecret(mark string) (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate a token: %w", err)
	}

	return mark + base64.RawURLEncoding.EncodeToString(raw), nil
}

// hashSecret は、DB に保存するトークンのハッシュを返す。
func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))

	return sum[:]
}
