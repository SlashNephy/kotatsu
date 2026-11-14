package httpserver

import (
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/labstack/echo/v5"
)

// NewRouter は、tenban の一番外側のルーターを返す (#1606)。echo のルートに当たらないリクエストは、fallback に渡す。
// ルートは、まとまりごとに、http.ServeMux から echo のハンドラーへ移していく。移し終えていないものは、fallback の mux が受け持つ。
// logger は、リクエストのロガー (echo.Context.Logger) の元にする。
func NewRouter(logger *slog.Logger, fallback http.Handler) *echo.Echo {
	e := echo.New()
	e.Logger = logger
	e.RouteNotFound("/*", echo.WrapHandler(fallback))

	return e
}

// Component は、リクエストのロガーに、部品の名前 (component) を付ける echo のミドルウェア。
// ハンドラーは、ロガーを受け取らずに、echo.Context.Logger から書く。
func Component(name string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.SetLogger(c.Logger().With(slog.String("component", name)))

			return next(c)
		}
	}
}

// Handle は、net/http のハンドラーを、パスとメソッドごとに echo のルーターへ登録する (#1606)。
// http.ServeMux と同じく、GET のハンドラーは HEAD にも答え、パスに当たってメソッドの違うリクエストには、Allow を付けて 405 を返す。
// echo は、メソッドの違うリクエストを、RouteNotFound (移し終えていない mux) に渡してしまうからである。
func Handle(router *echo.Echo, path string, handlers map[string]http.Handler, middleware ...echo.MiddlewareFunc) {
	allowed := make([]string, 0, len(handlers)+1)
	for method, handler := range handlers {
		allowed = append(allowed, method)
		router.Add(method, path, echo.WrapHandler(handler), middleware...)
		if _, ok := handlers[http.MethodHead]; method == http.MethodGet && !ok {
			allowed = append(allowed, http.MethodHead)
			router.Add(http.MethodHead, path, echo.WrapHandler(handler), middleware...)
		}
	}
	slices.Sort(allowed)
	allow := strings.Join(allowed, ", ")
	// メソッドを決めたルートは、Any のルートより先に当たる。
	router.Any(path, echo.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", allow)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})), middleware...)
}

// CanonicalPath は、http.ServeMux と同じく、崩れたパスを整えた先へ 307 でリダイレクトする echo の Pre ミドルウェア (#1606)。
// 二重のスラッシュや . と .. の区切りを取り除く (CONNECT は整えない)。subtrees ("/api/" など) の末尾のスラッシュを
// 除いたパスは、mux が配下を受けるパターンに対してするのと同じく、末尾にスラッシュを付けた先へリダイレクトする。
// echo のルートは崩れたパスに当たらないので、ルートを探す前に整える。
func CanonicalPath(subtrees ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			r := c.Request()
			escaped := r.URL.EscapedPath()
			canonical := escaped
			if r.Method != http.MethodConnect {
				canonical = cleanPath(escaped)
			}
			if slices.Contains(subtrees, canonical+"/") {
				canonical += "/"
			}
			if canonical == escaped {
				return next(c)
			}
			http.Redirect(c.Response(), r, urlFromEscaped(canonical, r.URL.RawQuery).String(), http.StatusTemporaryRedirect)

			return nil
		}
	}
}

// cleanPath は、net/http の ServeMux と同じく、パスを整える。先頭にスラッシュを付け、. と .. と二重のスラッシュを除き、末尾のスラッシュは残す。
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}

	return np
}

// urlFromEscaped は、net/http の ServeMux と同じく、エスケープしたままのパスとクエリから、リダイレクト先の URL を作る。
func urlFromEscaped(escaped, rawQuery string) *url.URL {
	unescaped, err := url.PathUnescape(escaped)
	if err != nil {
		unescaped = escaped
	}

	return &url.URL{Path: unescaped, RawPath: escaped, RawQuery: rawQuery}
}
