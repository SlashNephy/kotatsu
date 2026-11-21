package httpserver

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/labstack/echo/v5"
)

// NewAPIRouter は、API の待ち受けの一番外側のルーターを返す (#2016)。
//   - health は、ヘルスチェック (/healthz、/readyz) のハンドラーを、パスで引くもの。GET と HEAD に答える。
//   - subtrees は、kyusu と nabe のように、パスの頭 ("/internal/kyusu/live/" など) の下を、どのメソッドも受け持つハンドラー。
//   - app は、tenban のハンドラー。どのルートにも当たらないリクエストを受ける。tenban のいないインスタンスでは nil を渡し、素の 404 を返す。
//
// http.ServeMux で受けていたときと同じく、ルートを探す前に崩れたパスを整え、subtrees の頭から末尾のスラッシュを除いたパスは、
// スラッシュを付けた先へリダイレクトする。ヘルスチェックに別のメソッドで来たリクエストは、app があれば app に渡し
// (ServeMux では、tenban を載せた「/」のパターンが受けていた)、なければ Allow を付けて 405 を返す。
func NewAPIRouter(logger *slog.Logger, health, subtrees map[string]http.Handler, app http.Handler) *echo.Echo {
	fallback := app
	if fallback == nil {
		fallback = http.NotFoundHandler()
	}
	router := NewRouter(logger, fallback)
	prefixes := make([]string, 0, len(subtrees))
	for prefix := range subtrees {
		prefixes = append(prefixes, prefix)
	}
	slices.Sort(prefixes)
	router.Pre(CanonicalPath(prefixes...))

	for path, handler := range health {
		if app == nil {
			Handle(router, path, map[string]http.Handler{http.MethodGet: handler})

			continue
		}
		router.GET(path, echo.WrapHandler(handler))
		router.HEAD(path, echo.WrapHandler(handler))
	}
	for prefix, handler := range subtrees {
		router.Any(prefix+"*", echo.WrapHandler(handler))
	}

	return router
}
