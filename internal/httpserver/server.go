// Package httpserver は、各ロールのプロセスが持つ HTTP サーバーを扱う。
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	// ShutdownTimeout は、終了のシグナルを受けてから、処理中のリクエストを待つ時間の上限。
	// Kubernetes の terminationGracePeriodSeconds の既定 (30 秒) より短くする。
	ShutdownTimeout = 20 * time.Second
	// readHeaderTimeout は、リクエストのヘッダーを読み終えるまでの上限。遅いクライアントに接続を占有させない。
	// 本文の読み書きには上限を設けない。録画やライブの TS は何時間も流し続けるからである。
	readHeaderTimeout = 10 * time.Second
)

// Serve は ln で handler を公開し、ctx が終わったら処理中のリクエストを待ってから止まる。
// ctx が終わって正常に止まったときは nil を返す。
func Serve(ctx context.Context, logger *slog.Logger, ln net.Listener, handler http.Handler) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext: func(net.Listener) context.Context {
			// リクエストの context をシグナルで打ち切らず、Shutdown が処理中のリクエストを待てるようにする。
			return context.WithoutCancel(ctx)
		},
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	logger.InfoContext(ctx, "http server started", slog.String("addr", ln.Addr().String()))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server on %s stopped: %w", ln.Addr(), err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		// 待ちきれなかった接続を残さないよう、強制的に閉じる。
		_ = server.Close()

		return fmt.Errorf("failed to shut down the http server on %s gracefully: %w", ln.Addr(), err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server on %s stopped: %w", ln.Addr(), err)
	}

	logger.InfoContext(ctx, "http server stopped", slog.String("addr", ln.Addr().String()))

	return nil
}

// Listen は addr で TCP の待ち受けを始める。
// ロールを起動する前に待ち受けを始めておき、ポートが使えなければ起動を止められるようにする。
func Listen(ctx context.Context, addr string) (net.Listener, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	return ln, nil
}
