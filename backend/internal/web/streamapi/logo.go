package streamapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/usecase/servicelogo"
)

// logoCacheControl は、サービスのロゴの Cache-Control。ロゴはめったに変わらないので、1 日使い回させる。
// ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const logoCacheControl = "private, max-age=86400"

// LogoFetcher は、サービスのロゴの画像を取ってくる。*mirakurun.Client が満たす。
type LogoFetcher interface {
	ServiceLogo(ctx context.Context, serviceID int64) ([]byte, error)
}

var (
	_ LogoFetcher = new(mirakurun.Client)
	_ LogoFetcher = new(servicelogo.Store)
)

// LogoHandler は、チューナーサーバーのサービスのロゴを中継する (GET /media/services/{id}/logo)。
// チューナーサーバーがロゴを返さなければ、録画している TS から集めたロゴを返す (#1308)。
type LogoHandler struct {
	// tuner は、チューナーサーバーのクライアント。設定していなければ nil。
	tuner LogoFetcher
	// collected は、録画している TS から集めたロゴ。ロゴがなければ servicelogo.ErrNoLogo を返す。nil なら使わない。
	collected LogoFetcher
	logger    *slog.Logger
}

var _ http.Handler = new(LogoHandler)

// NewLogoHandler は LogoHandler を返す。tuner も collected もロゴを返さなければ、tuner が nil なら 503 を、そうでなければ 404 を返す。
func NewLogoHandler(tuner, collected LogoFetcher, logger *slog.Logger) *LogoHandler {
	return &LogoHandler{tuner: tuner, collected: collected, logger: logger}
}

func (h *LogoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)

		return
	}
	data, status, message := h.fetch(r.Context(), serviceID)
	if data == nil {
		if status != 0 {
			http.Error(w, message, status)
		}

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

// fetch は、サービスのロゴを、チューナーサーバーから取り、なければ集めたロゴから取る。
// 取れなければ nil と、返す状態のコードと文面を返す。リクエストが取り消されていれば、状態のコードは 0 にする。
func (h *LogoHandler) fetch(ctx context.Context, serviceID int64) ([]byte, int, string) {
	var tunerErr error
	if h.tuner != nil {
		data, err := h.tuner.ServiceLogo(ctx, serviceID)
		if err == nil {
			return data, http.StatusOK, ""
		}
		tunerErr = err
	}
	if h.collected != nil {
		data, err := h.collected.ServiceLogo(ctx, serviceID)
		if err == nil {
			return data, http.StatusOK, ""
		}
		if !errors.Is(err, servicelogo.ErrNoLogo) && ctx.Err() == nil {
			h.logger.WarnContext(ctx, "failed to get the collected service logo", slog.Int64("service_id", serviceID), slog.Any("error", err))
		}
	}
	switch {
	case ctx.Err() != nil:
		return nil, 0, ""
	case h.tuner == nil:
		return nil, http.StatusServiceUnavailable, "tuner server is not configured"
	// Mirakurun は、ロゴをまだ受信していなければ 503 を返す。チューナーが足りないのとは違い、待っても当分は出ないので、ないものとして扱う。
	case mirakurun.IsStatus(tunerErr, http.StatusNotFound), mirakurun.IsStatus(tunerErr, http.StatusServiceUnavailable):
		return nil, http.StatusNotFound, "the service has no logo"
	default:
		h.logger.ErrorContext(ctx, "failed to fetch the service logo", slog.Int64("service_id", serviceID), slog.Any("error", tunerErr))

		return nil, http.StatusBadGateway, "failed to fetch the service logo"
	}
}
