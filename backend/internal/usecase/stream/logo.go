package stream

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
)

// logoCacheControl は、サービスのロゴの Cache-Control。ロゴはめったに変わらないので、1 日使い回させる。
// ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const logoCacheControl = "private, max-age=86400"

// LogoFetcher は、サービスのロゴの画像を取ってくる。*mirakurun.Client が満たす。
type LogoFetcher interface {
	ServiceLogo(ctx context.Context, serviceID int64) ([]byte, error)
}

var _ LogoFetcher = new(mirakurun.Client)

// LogoHandler は、チューナーサーバーのサービスのロゴを中継する (GET /media/services/{id}/logo)。
type LogoHandler struct {
	// tuner は、チューナーサーバーのクライアント。設定していなければ nil。
	tuner  LogoFetcher
	logger *slog.Logger
}

var _ http.Handler = new(LogoHandler)

// NewLogoHandler は LogoHandler を返す。tuner が nil なら、すべてのリクエストに 503 を返す。
func NewLogoHandler(tuner LogoFetcher, logger *slog.Logger) *LogoHandler {
	return &LogoHandler{tuner: tuner, logger: logger}
}

func (h *LogoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)

		return
	}
	if h.tuner == nil {
		http.Error(w, "tuner server is not configured", http.StatusServiceUnavailable)

		return
	}

	data, err := h.tuner.ServiceLogo(r.Context(), serviceID)
	switch {
	// Mirakurun は、ロゴをまだ受信していなければ 503 を返す。チューナーが足りないのとは違い、待っても当分は出ないので、ないものとして扱う。
	case mirakurun.IsStatus(err, http.StatusNotFound), mirakurun.IsStatus(err, http.StatusServiceUnavailable):
		http.Error(w, "the service has no logo", http.StatusNotFound)

		return
	case err != nil && r.Context().Err() != nil:
		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "failed to fetch the service logo", slog.Int64("service_id", serviceID), slog.Any("error", err))
		http.Error(w, "failed to fetch the service logo", http.StatusBadGateway)

		return
	}
	// 上流の Content-Type は信じず、中身から決める。画像でなければ、同じオリジンで返すと危ないので返さない。
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		h.logger.WarnContext(r.Context(), "the service logo is not an image", slog.Int64("service_id", serviceID), slog.String("content_type", contentType))
		http.Error(w, "the service logo is not an image", http.StatusBadGateway)

		return
	}

	sum := sha256.Sum256(data)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", logoCacheControl)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// ServeContent は、ETag で If-None-Match に答える。
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}
