package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
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
// ただし、最後に DB に接続できてから grace の間は、接続できなくても成功を返す。DB が短い間だけ止まっても、
// tenban を Service から外さず、覚えた結果で視聴を続けられるようにする (#1084)。起動してから一度も接続できていなければ、失敗を返す。
func RegisterHealth(mux *http.ServeMux, logger *slog.Logger, db Pinger, grace time.Duration) {
	var mu sync.Mutex
	// lastReady は、最後に DB に接続できた時刻。壁時計が飛んでも測り違えないよう、time.Now の単調時計の読みを使う。
	var lastReady time.Time

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, http.StatusOK)
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()

		err := db.Ping(ctx)
		mu.Lock()
		if err == nil {
			lastReady = time.Now()
		}
		withinGrace := !lastReady.IsZero() && time.Since(lastReady) < grace
		mu.Unlock()
		if err != nil && !withinGrace {
			logger.WarnContext(ctx, "readiness check failed", slog.Any("error", err))
			writeStatus(w, http.StatusServiceUnavailable)

			return
		}
		if err != nil {
			logger.WarnContext(ctx, "the database is unreachable; staying ready within the grace period", slog.Any("error", err))
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
