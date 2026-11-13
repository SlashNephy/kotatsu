// Package iptvapi は、ライブ視聴の一覧 (M3U) と番組表 (XMLTV) を、IPTV のクライアント (Kodi、Jellyfin、Plex など) に配信する
// (#1324、設計書 9.8)。IPTV のクライアントは Cookie も Authorization も送れないので、パスに入れた秘密の値で認証する。
package iptvapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// PathPrefix は、ライブ視聴の M3U、番組表の XMLTV、配信、ロゴのパスの頭。この後ろに秘密の値が続く (#1324)。
// 前段の認証のプロキシから外す /api の下に置く (設計書 9.1、#1605)。
const PathPrefix = "/api/iptv/"

// echo のルート (#1606)。:token が秘密の値、:id がサービスの ID。
const (
	PlaylistRoute = PathPrefix + ":token/channels.m3u"
	GuideRoute    = PathPrefix + ":token/guide.xml"
	StreamRoute   = PathPrefix + ":token/services/:id/stream.m2ts"
	LogoRoute     = PathPrefix + ":token/services/:id/logo"
)

const (
	// guidePast は、番組表に載せる、始まった番組を探す長さ。放送中の番組を載せる。
	guidePast = time.Hour
	// guideFuture は、番組表に載せる、これからの番組の長さ。番組表の画面と同じく、一度に引ける長さに収める。
	guideFuture = 7 * 24 * time.Hour
)

// Authenticator は、配信の URL の秘密の値から、発行したユーザーを引く。*auth.Feeds が満たす。
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*auth.Principal, error)
}

var _ Authenticator = new(auth.Feeds)

// Queries は、サービスと番組を読む。*query.Queries が満たす。
type Queries interface {
	ListGuideServices(ctx context.Context, arg query.ListGuideServicesParams) ([]domain.Service, error)
	ListProgramsInRange(ctx context.Context, arg query.ListProgramsInRangeParams) ([]domain.Program, error)
}

var _ Queries = new(query.Queries)

// Handler は、M3U と XMLTV を返し、ストリームとロゴのリクエストを認証する。
type Handler struct {
	auth    Authenticator
	queries Queries
	clock   clock.Clock
	// origin は、リクエストが届いたオリジン (scheme://host)。M3U と XMLTV には、ストリームとロゴの絶対 URL を書く。
	origin func(r *http.Request) string
}

// NewHandler は Handler を返す。ログは、リクエストのロガー (echo.Context.Logger) に書く (#1606)。
func NewHandler(authenticator Authenticator, queries Queries, clk clock.Clock, origin func(r *http.Request) string) *Handler {
	return &Handler{auth: authenticator, queries: queries, clock: clk, origin: origin}
}

// Authenticate は、パスの秘密の値 (:token) で認証し、発行したユーザーとして next を呼ぶ echo のミドルウェア。
// 秘密の値が違うか、ユーザーがライブ視聴を見られなければ、URL があることを知らせないよう 404 を返す。
// URL に秘密の値を含むので、途中のキャッシュに残させない。
// エラーは echo に返さず、ここで応答を書く。echo の既定のエラーの応答 (JSON) にしないためである。
func (h *Handler) Authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		w, r := c.Response(), c.Request()
		ctx := r.Context()
		principal, err := h.auth.Authenticate(ctx, c.Param("token"))
		if errors.Is(err, auth.ErrUnauthenticated) {
			http.NotFound(w, r)

			return nil
		}
		if err != nil {
			fail(ctx, c.Logger(), w, "failed to authenticate the IPTV feed", err)

			return nil
		}
		if !principal.Role.Can(domain.PermissionWatch) {
			http.NotFound(w, r)

			return nil
		}
		w.Header().Set("Cache-Control", "private, no-store")
		c.SetRequest(r.WithContext(auth.WithPrincipal(ctx, principal)))

		return next(c)
	}
}

// services は、番組表に出すサービス (非表示と、映像も音声も流さないサービスを除く) を返す。
func (h *Handler) services(ctx context.Context) ([]domain.Service, error) {
	return h.queries.ListGuideServices(ctx, query.ListGuideServicesParams{
		IncludeHidden: false, MediaServiceTypes: domain.MediaServiceTypes(), IDs: []int64{},
	})
}

// base は、秘密の値 token の配信の URL の頭 (scheme://host/api/iptv/<token>) を返す。
// token は、リクエストの値をそのまま返すので、パスの一部としてエスケープする。発行する値は base64url なので変わらない。
func (h *Handler) base(c *echo.Context) string {
	return h.origin(c.Request()) + PathPrefix + url.PathEscape(c.Param("token"))
}

func fail(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, message string, err error) {
	logger.ErrorContext(ctx, message, slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
