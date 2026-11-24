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

// NewRouter は、http.ServeMux から移したルートを受ける echo のルーターを返す。tenban (#1606) や /metrics の待ち受け (#2008) で使う。
// echo のルートに当たらないリクエストは、fallback に渡す。
// logger は、リクエストのロガー (echo.Context.Logger) の元にする。
func NewRouter(logger *slog.Logger, fallback http.Handler) *echo.Echo {
	e := echo.New()
	e.Logger = logger
	e.RouteNotFound("/*", echo.WrapHandler(fallback))
	e.Use(pathParams(fallback))

	return e
}

// pathParams は、名前を付けたパスの部分 (:id) を、http.ServeMux の {id} と同じく読む echo のミドルウェア (#2016)。
//   - echo は、末尾の名前の部分を、スラッシュをまたいで当てる (/a/:id が /a/b/c に当たり、id は b/c)。
//     ServeMux の {id} は 1 つの区切りにしか当たらないので、スラッシュを含む値は、どのルートにも当たらなかったものとして fallback に渡す。
//   - echo は、エスケープしたパス (URL.RawPath) で当てたときに、値を戻さない (%31 は %31 のまま)。ServeMux と同じく戻す。
//   - 値が (戻した後も)「/」を含むか、「.」か「..」なら、どのルートにも当たらなかったものとして fallback に渡す。ServeMux は %2F を
//     値の中の「/」として渡すが、ハンドラーは値をパスに組み直すことがある (kyusu の中継など) ので、ほかのパスを指させない。
//     ルートの ID (UUID、数、base64url) は、どれも「/」と「.」だけの値を持たない。
//
// 名前のない残り (*) は、スラッシュを含んでよいので、そのままにする。
func pathParams(fallback http.Handler) echo.MiddlewareFunc {
	notFound := echo.WrapHandler(fallback)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			// PathValues は、Context の持つ値をそのまま返すので、戻した値はその場で書き換える。
			values := c.PathValues()
			rawPath := c.Request().URL.RawPath != ""
			for i, p := range values {
				if p.Name == "*" {
					continue
				}
				if strings.Contains(p.Value, "/") || p.Value == "." || p.Value == ".." {
					return notFound(c)
				}
				if rawPath {
					value, err := url.PathUnescape(p.Value)
					if err != nil || strings.Contains(value, "/") || value == "." || value == ".." {
						return notFound(c)
					}
					values[i].Value = value
				}
			}

			return next(c)
		}
	}
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
	// ブラウザは、先頭の「/\」を「//」と読み、ほかのホストへのリダイレクトにする。EscapedPath は \ を %5C にするので来ないが、
	// 生の \ が来ても、リダイレクト先のホストにならないよう、%5C にする。
	if strings.HasPrefix(p, `/\`) {
		p = "/%5C" + p[2:]
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
