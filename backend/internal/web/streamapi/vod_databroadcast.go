package streamapi

import (
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// VODDataBroadcastHandler は、録画型の再生元 (録画、タイムシフトのバッファ、ライブラリのファイル) の配信から、
// データ放送の描画に要るパケットだけを流す (GET /internal/streams/{recordings|timeshift|library}/{id}/data-broadcast?start=秒、
// 設計書 7.4、#2270)。同じ要求を映像の配信 inner に渡し、応答をライブと同じフィルターに通す。
// 録画型の再生元は、サービスを指定して読まないので、PAT の最初の番組を選ぶ。
type VODDataBroadcastHandler struct {
	inner http.Handler
}

var _ http.Handler = new(VODDataBroadcastHandler)

// NewVODDataBroadcastHandler は、映像の配信 inner の VODDataBroadcastHandler を返す。
func NewVODDataBroadcastHandler(inner http.Handler) *VODDataBroadcastHandler {
	return &VODDataBroadcastHandler{inner: inner}
}

func (h *VODDataBroadcastHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	// start がないと、録画を終えた録画は Range に答える 1 本のファイルとして返り、フィルターを通すと長さが合わなくなる。
	// ブラウザは、映像と同じ位置から読ませるので、いつも start を付ける。
	if !query.Has("start") {
		http.Error(w, "start is required", http.StatusBadRequest)

		return
	}
	if query.Has("download") {
		http.Error(w, "download cannot be combined with data-broadcast", http.StatusBadRequest)

		return
	}
	h.inner.ServeHTTP(&vodDataBroadcastWriter{dataBroadcastWriter: newFilteredWriter(w, ts.NewFirstProgramDataBroadcastFilter()), wroteHeader: false}, r)
}

// vodDataBroadcastWriter は、映像の配信の応答から、長さとバイトの範囲のヘッダーを外してから書く。
// フィルターで残すパケットだけを書くので、映像の配信が付けた長さとは合わない。
type vodDataBroadcastWriter struct {
	*dataBroadcastWriter

	wroteHeader bool
}

var _ http.ResponseWriter = new(vodDataBroadcastWriter)

// WriteHeader は、長さとバイトの範囲のヘッダーを外してから、status を書く。
func (v *vodDataBroadcastWriter) WriteHeader(status int) {
	if v.wroteHeader {
		return
	}
	v.wroteHeader = true
	header := v.Header()
	header.Del("Content-Length")
	header.Del("Content-Range")
	header.Del("Accept-Ranges")
	v.dataBroadcastWriter.WriteHeader(status)
}

// Write は、ヘッダーをまだ書いていなければ 200 で書いてから、p をフィルターに通して書く。
func (v *vodDataBroadcastWriter) Write(p []byte) (int, error) {
	if !v.wroteHeader {
		v.WriteHeader(http.StatusOK)
	}

	return v.dataBroadcastWriter.Write(p)
}
