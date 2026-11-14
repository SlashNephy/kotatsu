package cli

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/netactivity"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/usecase/oauth"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// accessParts は、tenban が、セッションの Cookie 以外で認可するのに使う部品 (設計書 9.6、9.8、9.9)。
// 外部プレイヤーの署名付き URL、予約の iCalendar と IPTV の購読、MCP のクライアントのトークンを扱う。
type accessParts struct {
	urlUsers            *auth.URLUsers
	signer              *stream.Signer
	calendarFeeds       calendarFeeds
	iptvFeeds           iptvFeeds
	authorizationServer *oauth.Server
}

// calendarFeeds と iptvFeeds は、予約の iCalendar と IPTV の購読のトークン。同じ型 (*auth.Feeds) なので、wire に渡すために分ける。
type (
	calendarFeeds struct{ *auth.Feeds }
	iptvFeeds     struct{ *auth.Feeds }
)

func networkOf(deps *Deps) *netactivity.Recorder {
	return deps.Network
}

// newURLUsers は、署名付き URL を発行したユーザーを引く部品を作る。DB に届かない間は、覚えた結果を使う (#1084)。
func newURLUsers(pool *pgxpool.Pool, clk clock.Clock, logger *slog.Logger) *auth.URLUsers {
	return auth.NewURLUsers(pool, clk, logger.With(slog.String("component", "stream")))
}

// newStreamSigner は、外部プレイヤー用の映像の URL に署名する部品を作る。アプリの秘密キーから導いたキーで署名する (設計書 9.6)。
func newStreamSigner(secret appSecret) (*stream.Signer, error) {
	return stream.NewSigner(string(secret))
}

func newCalendarFeeds(pool *pgxpool.Pool, clk clock.Clock, recorder audit.Recorder) calendarFeeds {
	return calendarFeeds{auth.NewCalendarFeeds(pool, clk, recorder)}
}

func newIPTVFeeds(pool *pgxpool.Pool, clk clock.Clock, recorder audit.Recorder) iptvFeeds {
	return iptvFeeds{auth.NewIPTVFeeds(pool, clk, recorder)}
}

// newAuthorizationServer は、MCP のクライアントにトークンを発行する認可サーバーを作る (#963、設計書 9.9)。
func newAuthorizationServer(pool *pgxpool.Pool, clk clock.Clock, network *netactivity.Recorder, recorder audit.Recorder) *oauth.Server {
	return oauth.NewServer(pool, clk, oauth.NewFetcher(clk, network), recorder)
}
