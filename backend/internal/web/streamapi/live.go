package streamapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/SlashNephy/kotatsu/internal/domain/tunerserver"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

const (
	// contentTypeTS は、MPEG-2 TS の Content-Type。
	contentTypeTS = "video/mp2t"
	// livePriority は、ライブ視聴でチューナーを確保するときの優先度 (設計書 7.2)。
	// 録画は正の優先度で確保するので、チューナーが足りなければ、ライブ視聴が奪われる。
	livePriority = 0
)

// ServiceOpener は、サービスのストリームを開く。tunerserver.Client が満たす。
type ServiceOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *tunerserver.StreamOptions) (io.ReadCloser, error)
}

var _ ServiceOpener = tunerserver.Client(nil)

// ScrambleWatcher は、中継する TS が、スクランブルされたまま届いているかを判定する (設計書 7.1、#1517)。*scramble.Reporter が満たす。
type ScrambleWatcher interface {
	// Watch は、src を判定しながら読む io.Reader を返す。スクランブルされたままと決めたら、理由を記録してから ts.ErrScrambled で読むのを止める。
	Watch(ctx context.Context, serviceID int64, src io.Reader) (io.Reader, *ts.ScrambleDetector)
}

// LiveHandler は、チューナーサーバーのサービスストリームを中継する (GET /streams/services/{id})。
type LiveHandler struct {
	// tuner は、チューナーサーバーのクライアント。設定していなければ nil。
	tuner ServiceOpener
	// dataBroadcast は、データ放送の描画に要るパケットだけを流すかどうか (設計書 7.4)。
	dataBroadcast bool
	logger        *slog.Logger
	// scramble は、スクランブルされたまま届くかを判定する。nil なら判定しない。
	scramble ScrambleWatcher
}

var _ http.Handler = new(LiveHandler)

// NewLiveHandler は LiveHandler を返す。tuner が nil なら、すべてのリクエストに 503 を返す。
func NewLiveHandler(tuner ServiceOpener, logger *slog.Logger) *LiveHandler {
	return &LiveHandler{tuner: tuner, dataBroadcast: false, logger: logger, scramble: nil}
}

// NewDataBroadcastHandler は、サービスストリームから、データ放送の描画に要るパケットだけを流す LiveHandler を返す
// (GET /streams/services/{id}/data-broadcast、設計書 7.4)。ブラウザは、これを web-bml に渡す。
// tuner が nil なら、すべてのリクエストに 503 を返す。
func NewDataBroadcastHandler(tuner ServiceOpener, logger *slog.Logger) *LiveHandler {
	return &LiveHandler{tuner: tuner, dataBroadcast: true, logger: logger, scramble: nil}
}

// WithScramble は、中継しながら、スクランブルされたまま届いているかを判定する LiveHandler を返す (設計書 7.1、#1517)。
// スクランブルされたままと決めたら、理由をプレイヤーに届けるよう記録してから、ストリームを閉じる。
// 判定は、画面のプレイヤーに流す映像の中継でだけ行う。データ放送と、外部のプレイヤーに流す中継では行わない。
func (h *LiveHandler) WithScramble(watcher ScrambleWatcher) *LiveHandler {
	copied := *h
	copied.scramble = watcher

	return &copied
}

// ServeHTTP は、スクランブルを解除したストリームを、クライアントが切断するまで流す。
// 上流のストリームは、リクエストの context で開くので、クライアントが切断すると閉じ、チューナーが解放される。
func (h *LiveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)

		return
	}
	dst := w
	if h.dataBroadcast {
		number, ok := programNumber(serviceID)
		if !ok {
			http.Error(w, "invalid service ID", http.StatusBadRequest)

			return
		}
		dst = newDataBroadcastWriter(w, number)
	}
	if h.tuner == nil {
		http.Error(w, "tuner server is not configured", http.StatusServiceUnavailable)

		return
	}
	// GET のパターンは HEAD にも一致する。HEAD でストリームを開くと、本文を捨てながらチューナーを持ち続けるので、開かない。
	if r.Method == http.MethodHead {
		writeStreamHeader(w)

		return
	}

	upstream, err := h.tuner.OpenServiceStream(r.Context(), serviceID, &tunerserver.StreamOptions{Decode: true, Priority: livePriority})
	switch {
	case errors.Is(err, tunerserver.ErrNotFound):
		http.Error(w, "service not found", http.StatusNotFound)

		return
	case errors.Is(err, tunerserver.ErrTunerUnavailable):
		http.Error(w, "no tuner is available", http.StatusServiceUnavailable)

		return
	case err != nil && r.Context().Err() != nil:
		// 開いている間にクライアントが切断した。
		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "failed to open the service stream", slog.Int64("service_id", serviceID), slog.Any("error", err))
		http.Error(w, "failed to open the service stream", http.StatusBadGateway)

		return
	}
	defer func() {
		_ = upstream.Close()
	}()

	writeStreamHeader(w)
	var src io.Reader = upstream
	if h.scramble != nil && !h.dataBroadcast {
		src, _ = h.scramble.Watch(r.Context(), serviceID, upstream)
	}
	err = stream.CopyFlushing(dst, src)
	switch {
	case errors.Is(err, ts.ErrScrambled):
		// 理由は、記録して subscription でプレイヤーに届けたので、ここでは閉じるだけにする。
	case err != nil && r.Context().Err() == nil:
		// クライアントの切断ではなく、上流が途切れた。視聴の再接続はクライアントに任せる。
		h.logger.WarnContext(r.Context(), "the service stream ended", slog.Int64("service_id", serviceID), slog.Any("error", err))
	}
}

// writeStreamHeader は、TS を流す応答のヘッダーを書く。
func writeStreamHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", contentTypeTS)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}
