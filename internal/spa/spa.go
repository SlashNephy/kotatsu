// Package spa は、埋め込んだ SPA を配信する (設計書 2.3、10.4)。
package spa

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

const (
	indexFile = "index.html"
	// noncePlaceholder は、vite.config.ts の html.cspNonce。index.html の配信ごとに nonce に置き換える。
	noncePlaceholder = "__KOTATSU_CSP_NONCE__"
	// nonceBytes は、nonce の乱数の長さ。
	nonceBytes = 16
	// NXJikkyoOrigin は、実況のコメントを受け取る NX-Jikkyo の WebSocket の接続先 (設計書 10.4)。
	NXJikkyoOrigin = "wss://nx-jikkyo.tsukumijima.net"
)

// ContentSecurityPolicy は、nonce を使う、index.html の Content Security Policy を返す (設計書 10.4)。
// 明示的に許すのは、WebAssembly の実行と NX-Jikkyo への WebSocket の接続だけにする。
// nonce は、Mantine が差し込む <style> と、Vite が出力する <script> と <link> に付く。
func ContentSecurityPolicy(nonce string) string {
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'nonce-" + nonce + "' 'wasm-unsafe-eval'",
		"style-src 'self' 'nonce-" + nonce + "'",
		"img-src 'self' data: blob:",
		"media-src 'self' blob:",
		"font-src 'self'",
		"connect-src 'self' " + NXJikkyoOrigin,
		"worker-src 'self' blob:",
		"manifest-src 'self'",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'self'",
		"frame-ancestors 'none'",
	}, "; ")
}

// Handler は SPA を配信する。
//
//   - ファイルがあれば、そのファイルを返す。ファイル名にハッシュの付く assets/ は、変わらないので長くキャッシュさせる。
//   - ファイルがなく、拡張子もないパスは、クライアント側のルーティングに任せるため index.html を返す。
//   - SPA がビルドされていなければ、503 で、ビルドの仕方を案内する。
type Handler struct {
	app    fs.FS
	files  http.Handler
	index  []byte
	logger *slog.Logger
}

// NewHandler は、app のファイルを配信する Handler を返す。
func NewHandler(app fs.FS, logger *slog.Logger) (*Handler, error) {
	index, err := fs.ReadFile(app, indexFile)
	if errors.Is(err, fs.ErrNotExist) {
		// 未ビルドのバイナリでも API は使えるように、起動は止めない。
		logger.Warn("the frontend is not built; build it with `mise run web:build` before go build")

		return &Handler{app: app, files: http.FileServerFS(app), index: nil, logger: logger}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the frontend index: %w", err)
	}

	return &Handler{app: app, files: http.FileServerFS(app), index: index, logger: logger}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

		return
	}
	if h.index == nil {
		http.Error(w, "the frontend is not built", http.StatusServiceUnavailable)

		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name != "" && name != indexFile {
		if stat, err := fs.Stat(h.app, name); err == nil && !stat.IsDir() {
			h.serveFile(w, r, name)

			return
		}
		// 拡張子のあるパスは、ファイルを探している。index.html を返すと、壊れたファイルとして扱われる。
		if path.Ext(name) != "" {
			http.NotFound(w, r)

			return
		}
	}
	h.serveIndex(w, r)
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	// http.FileServerFS も、パスを整えてから、app の中だけを探す。
	h.files.ServeHTTP(w, r)
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	raw := make([]byte, nonceBytes)
	if _, err := rand.Read(raw); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to generate a CSP nonce", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}
	nonce := base64.StdEncoding.EncodeToString(raw)

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	// nonce が毎回変わるので、キャッシュさせない。
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", ContentSecurityPolicy(nonce))
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	body := bytes.ReplaceAll(h.index, []byte(noncePlaceholder), []byte(nonce))
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		h.logger.DebugContext(r.Context(), "failed to write the frontend index", slog.Any("error", err))
	}
}
