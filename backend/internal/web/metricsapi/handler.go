// Package metricsapi は、/metrics の待ち受けで返すメトリクスと pprof のハンドラーを用意する (設計書 11.3)。
package metricsapi

import (
	"log/slog"
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

const pprofPathPrefix = "/debug/pprof/"

// NewHandler は、gatherer の内容を Prometheus 形式で返す /metrics を、echo で受けるハンドラーを作る (#2008)。
// registry には、/metrics 自身のエラーの数などを登録する。
// withPprof なら、プロセスの中を調べるための /debug/pprof/ も同じ待ち受けで返す (設計書 11.3、#1959)。
// http.ServeMux で受けていたときと同じく、崩れたパスはリダイレクトし、当たらないパスには素の 404 を返す。
func NewHandler(logger *slog.Logger, gatherer prometheus.Gatherer, registry *prometheus.Registry, withPprof bool) http.Handler {
	router := httpserver.NewRouter(logger, http.NotFoundHandler())
	subtrees := []string{}
	if withPprof {
		subtrees = append(subtrees, pprofPathPrefix)
	}
	router.Pre(httpserver.CanonicalPath(subtrees...))

	httpserver.Handle(router, "/metrics", map[string]http.Handler{
		http.MethodGet: promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{
			Registry: registry,
		}),
	})
	if withPprof {
		// Index は、/debug/pprof/heap のような名前の付いたプロファイルも返す。
		httpserver.Handle(router, pprofPathPrefix+"*", map[string]http.Handler{http.MethodGet: http.HandlerFunc(pprof.Index)})
		httpserver.Handle(router, pprofPathPrefix+"cmdline", map[string]http.Handler{http.MethodGet: http.HandlerFunc(pprof.Cmdline)})
		httpserver.Handle(router, pprofPathPrefix+"profile", map[string]http.Handler{http.MethodGet: http.HandlerFunc(pprof.Profile)})
		httpserver.Handle(router, pprofPathPrefix+"symbol", map[string]http.Handler{
			http.MethodGet:  http.HandlerFunc(pprof.Symbol),
			http.MethodPost: http.HandlerFunc(pprof.Symbol),
		})
		httpserver.Handle(router, pprofPathPrefix+"trace", map[string]http.Handler{http.MethodGet: http.HandlerFunc(pprof.Trace)})
	}

	return router
}
