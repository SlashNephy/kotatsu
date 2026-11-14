package httpserver

import (
	"log/slog"
	"net/http"
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
