package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// readinessTimeout は、/readyz で DB の応答を待つ上限。
const readinessTimeout = 2 * time.Second

// Pinger は、DB に接続できるかを確かめる。*pgxpool.Pool が満たす。
type Pinger interface {
	Ping(ctx context.Context) error
}

// RegisterHealth は /healthz と /readyz を mux に登録する。
//
// どちらも外部からの死活監視に使えるよう認証なしで公開するので、成否のほかは返さない (設計書 11.3)。
// /readyz は DB に接続できるかだけで判定し、チューナーサーバーの状態は含めない (設計書 11.3)。
func RegisterHealth(mux *http.ServeMux, logger *slog.Logger, db Pinger) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusOK)
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "readiness check failed", slog.Any("error", err))
			writeStatus(w, http.StatusServiceUnavailable)

			return
		}

		writeStatus(w, http.StatusOK)
	})
}

func writeStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(http.StatusText(status) + "\n"))
}
