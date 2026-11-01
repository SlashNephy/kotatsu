package streamapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/kago"
	"github.com/SlashNephy/kotatsu/internal/usecase/stream"
)

// TimeshiftHandler は、タイムシフトのバッファを配信する (GET /streams/timeshift/{id}?from=RFC 3339 の時刻&start=秒、設計書 7.2)。
// id はサービス ID。from の時刻を再生の時間の起点にし、そこから start 秒の位置の手前の I フレームから流す。
// 時間は、録画の再生と同じく、途切れた区間を含めない。実際に流し始めた位置 (起点からの秒) を X-Kotatsu-Start で返し、HEAD でも返す。
// バッファは終わらないので、書いている途中のチャンクの末尾まで読んだら、伸びるのと、次のチャンクができるのを待ちながら流し続ける。
type TimeshiftHandler struct {
	source *stream.TimeshiftSource
	logger *slog.Logger
}

var _ http.Handler = new(TimeshiftHandler)

// NewTimeshiftHandler は TimeshiftHandler を返す。store は、このインスタンスのタイムシフトのディレクトリとデータディレクトリ。
// pollInterval が 0 なら 500 ミリ秒にする。
func NewTimeshiftHandler(pool *pgxpool.Pool, store *kago.Store, logger *slog.Logger, pollInterval time.Duration) *TimeshiftHandler {
	return &TimeshiftHandler{source: stream.NewTimeshiftSource(pool, store, pollInterval), logger: logger}
}

func (h *TimeshiftHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	serviceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid service ID", http.StatusBadRequest)

		return
	}
	from, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("from"))
	if err != nil {
		http.Error(w, "from must be an RFC 3339 time", http.StatusBadRequest)

		return
	}
	start, ok := parseStart(r.URL.Query().Get("start"))
	if !ok {
		http.Error(w, "start must be a non-negative number of seconds", http.StatusBadRequest)

		return
	}
	buffer, err := h.source.Open(r.Context(), serviceID, from)
	switch {
	case errors.Is(err, stream.ErrTimeshiftNotFound), errors.Is(err, stream.ErrNoSegment):
		http.Error(w, err.Error(), http.StatusNotFound)

		return
	case errors.Is(err, stream.ErrNotMounted):
		http.Error(w, "timeshift is not configured on this server", http.StatusServiceUnavailable)

		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "failed to prepare the timeshift stream", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	pos := buffer.Seek(start)
	w.Header().Set(StartHeader, strconv.FormatFloat(pos.At.Seconds(), 'f', 3, 64))
	writeStreamHeader(w)
	if r.Method == http.MethodHead {
		return
	}
	if err := buffer.Write(r.Context(), w, pos); err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "failed to stream the timeshift buffer", slog.Int64("service_id", serviceID), slog.Any("error", err))
	}
}
