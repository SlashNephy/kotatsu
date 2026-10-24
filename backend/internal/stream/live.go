package stream

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

const (
	// contentTypeTS は、MPEG-2 TS の Content-Type。
	contentTypeTS = "video/mp2t"
	// copyBufferSize は、上流から読んでクライアントに書く単位。TS のパケット 1024 個分。
	copyBufferSize = 188 * 1024
	// livePriority は、ライブ視聴でチューナーを確保するときの優先度 (設計書 7.2)。
	// 録画は正の優先度で確保するので、チューナーが足りなければ、ライブ視聴が奪われる。
	livePriority = 0
)

// ServiceOpener は、サービスのストリームを開く。*mirakurun.Client が満たす。
type ServiceOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error)
}

var _ ServiceOpener = new(mirakurun.Client)

// LiveHandler は、チューナーサーバーのサービスストリームを中継する (GET /streams/services/{id})。
type LiveHandler struct {
	// tuner は、チューナーサーバーのクライアント。設定していなければ nil。
	tuner ServiceOpener
	// dataBroadcast は、データ放送の描画に要るパケットだけを流すかどうか (設計書 7.4)。
	dataBroadcast bool
	logger        *slog.Logger
}

var _ http.Handler = new(LiveHandler)

// NewLiveHandler は LiveHandler を返す。tuner が nil なら、すべてのリクエストに 503 を返す。
func NewLiveHandler(tuner ServiceOpener, logger *slog.Logger) *LiveHandler {
	return &LiveHandler{tuner: tuner, dataBroadcast: false, logger: logger}
}

// NewDataBroadcastHandler は、サービスストリームから、データ放送の描画に要るパケットだけを流す LiveHandler を返す
// (GET /streams/services/{id}/data-broadcast、設計書 7.4)。ブラウザは、これを web-bml に渡す。
// tuner が nil なら、すべてのリクエストに 503 を返す。
func NewDataBroadcastHandler(tuner ServiceOpener, logger *slog.Logger) *LiveHandler {
	return &LiveHandler{tuner: tuner, dataBroadcast: true, logger: logger}
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

	upstream, err := h.tuner.OpenServiceStream(r.Context(), serviceID, &mirakurun.StreamOptions{Decode: true, Priority: livePriority})
	switch {
	case mirakurun.IsStatus(err, http.StatusNotFound):
		http.Error(w, "service not found", http.StatusNotFound)

		return
	case mirakurun.IsStatus(err, http.StatusServiceUnavailable):
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
	if err := copyFlushing(dst, upstream); err != nil && r.Context().Err() == nil {
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

// copyFlushing は、src を読み終えるまで、読んだ分を w に書いて送り出す。w が HTTP の応答なら、書くたびに送り出す。
func copyFlushing(w io.Writer, src io.Reader) error {
	flush := func() error { return nil }
	if rw, ok := w.(http.ResponseWriter); ok {
		controller := http.NewResponseController(rw)
		flush = func() error {
			if err := controller.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return err
			}

			return nil
		}
	}
	buf := make([]byte, copyBufferSize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if ferr := flush(); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
