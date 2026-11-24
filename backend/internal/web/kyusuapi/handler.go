// Package kyusuapi は、kyusu がトランスコードした HLS を配信する端点 (/internal/kyusu/*) を、echo のルーターに登録する (設計書 7.3、#2016)。
// tenban が、インスタンスの登録簿で kyusu を引いてプロキシする。
package kyusuapi

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
	"github.com/SlashNephy/kotatsu/internal/web/httpserver"
)

// hlsPath は、配信のパスのプレフィックスに続く、プレイリストかセグメントのパス。
const hlsPath = ":preset/:audio/:file"

// LiveHandler は、ライブ視聴の HLS を配信する kyusu.LivePathPrefix の下のハンドラーを返す。token を付けたリクエストだけを受け付ける。
// トークンは、ルートを探す前に確認する。トークンのないリクエストには、パスに当たるかどうかによらず 401 を返す。
func LiveHandler(logger *slog.Logger, live *kyusu.Live, token *kyusu.Token) http.Handler {
	router := newRouter(logger)
	httpserver.Handle(router, kyusu.LivePathPrefix+":service/"+hlsPath, map[string]http.Handler{http.MethodGet: live})

	return token.Middleware(router)
}

// VODHandler は、録画、タイムシフトのバッファ、ライブラリのファイルの仮想 HLS を配信するハンドラーを返す。
// kyusu.RecordingPathPrefix、kyusu.TimeshiftPathPrefix、kyusu.LibraryPathPrefix の下を受ける。トークンの扱いは LiveHandler と同じ。
func VODHandler(logger *slog.Logger, vod *kyusu.VOD, token *kyusu.Token) http.Handler {
	router := newRouter(logger)
	httpserver.Handle(router, kyusu.RecordingPathPrefix+":id/"+hlsPath, map[string]http.Handler{http.MethodGet: http.HandlerFunc(vod.ServeRecording)})
	httpserver.Handle(router, kyusu.TimeshiftPathPrefix+":id/"+hlsPath, map[string]http.Handler{http.MethodGet: http.HandlerFunc(vod.ServeTimeshift)})
	httpserver.Handle(router, kyusu.LibraryPathPrefix+":id/"+hlsPath, map[string]http.Handler{http.MethodGet: http.HandlerFunc(vod.ServeLibrary)})

	return token.Middleware(router)
}

// newRouter は、http.ServeMux で受けていたときと同じく、崩れたパスをリダイレクトし、当たらないパスに素の 404 を返すルーターを返す。
func newRouter(logger *slog.Logger) *echo.Echo {
	router := httpserver.NewRouter(logger, http.NotFoundHandler())
	router.Pre(httpserver.CanonicalPath())

	return router
}
