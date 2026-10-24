// Package httpserver は、各ロールのプロセスが持つ HTTP サーバーを扱う。
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// ShutdownTimeout は、終了のシグナルを受けてから、処理中のリクエストを待つ時間の上限。
	// Kubernetes の terminationGracePeriodSeconds の既定 (30 秒) より短くする。
	ShutdownTimeout = 20 * time.Second
	// readHeaderTimeout は、リクエストのヘッダーを読み終えるまでの上限。遅いクライアントに接続を占有させない。
	// 本文の読み書きには上限を設けない。録画やライブの TS は何時間も流し続けるからである。
	readHeaderTimeout = 10 * time.Second
	// idleTimeout は、keep-alive の接続を、次のリクエストを待って開けておく時間の上限。
	idleTimeout = 2 * time.Minute
)

// shutdownKey は、停止を始めたら閉じるチャネルを、リクエストの context に入れるキー。
type shutdownKey struct{}

// StopOnShutdown は、サーバーが停止を始めたら、リクエストの context を取りやめ、書き込みも打ち切るミドルウェアを返す。
// ライブ視聴や録画の配信のような、終わらないストリームを包む。包まないリクエストは、停止の間も終わるまで待つ。
func StopOnShutdown(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shutdown, ok := r.Context().Value(shutdownKey{}).(<-chan struct{})
		if !ok {
			next.ServeHTTP(w, r)

			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		controller := http.NewResponseController(w)
		watched := make(chan struct{})
		// ハンドラーが返った後に、応答を触らないよう、見張りが終わるのを待ってから返す。
		defer func() {
			cancel()
			<-watched
		}()
		go func() {
			defer close(watched)
			select {
			case <-ctx.Done():
			case <-shutdown:
				cancel()
				// クライアントが読まずに書き込みが止まっていても、すぐに抜けさせる。
				_ = controller.SetWriteDeadline(time.Now())
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StopEventStreamsOnShutdown は、SSE (Accept: text/event-stream) のリクエストだけを StopOnShutdown で包む。
// GraphQL のように、同じエンドポイントで、すぐ終わるリクエストと終わらない購読を受けるときに使う。
func StopEventStreamsOnShutdown(next http.Handler) http.Handler {
	stoppable := StopOnShutdown(next)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
			stoppable.ServeHTTP(w, r)

			return
		}
		next.ServeHTTP(w, r)
	})
}

// Serve は ln で handler を公開し、ctx が終わったら処理中のリクエストを待ってから止まる。
// ctx が終わって正常に止まったときは nil を返す。
// 終わらないストリームは、StopOnShutdown で包んでおけば、停止を始めたときに打ち切り、待たない。
func Serve(ctx context.Context, logger *slog.Logger, ln net.Listener, handler http.Handler) error {
	shutdown := make(chan struct{})
	startShutdown := sync.OnceFunc(func() { close(shutdown) })
	defer startShutdown()
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		BaseContext: func(net.Listener) context.Context {
			// リクエストの context をシグナルで打ち切らず、Shutdown が処理中のリクエストを待てるようにする。
			return context.WithValue(context.WithoutCancel(ctx), shutdownKey{}, (<-chan struct{})(shutdown))
		},
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	server.RegisterOnShutdown(startShutdown)

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
