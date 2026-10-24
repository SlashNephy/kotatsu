package oauth

import (
	"net/http"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// NewFetcherWithTransport は、テストのサーバー (ループバックのアドレス) に接続できる Fetcher を返す。
func NewFetcherWithTransport(clk clock.Clock, transport http.RoundTripper) *Fetcher {
	return newFetcher(clk, transport)
}

// Freshness は freshness をテストから呼ぶ。
func Freshness(cacheControl string) time.Duration {
	return freshness(cacheControl)
}
