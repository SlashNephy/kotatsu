package httpserver

import (
	"log/slog"
	"net/http"

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
