package stream

import (
	"errors"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// RecordingHandler は、録画を配信する (GET /streams/recordings/{id}?start=秒)。download を付けると、録画ファイルの名前で保存させる。
// 録画を終えた録画を start なしで要求されたら、長さを返し、HTTP の Range に答える。外部プレイヤーは Range でシークする。
//
// 索引を使って、録画の始まりから start 秒の位置を GOP の先頭のバイト位置に変換し、そこからセグメントを連結して流す。
// 引き継ぎで重なった区間は切り詰める。録画中の録画は、ファイルの末尾まで読んだら、ファイルが伸びるのを待ちながら流し続ける。
type RecordingHandler struct {
	source *RecordingSource
	logger *slog.Logger
}

var _ http.Handler = new(RecordingHandler)

// NewRecordingHandler は RecordingHandler を返す。store は、このインスタンスにマウントされたプール。
// pollInterval が 0 なら 500 ミリ秒にする。
func NewRecordingHandler(db sqlc.DBTX, store *kago.Store, logger *slog.Logger, pollInterval time.Duration) *RecordingHandler {
	return &RecordingHandler{source: NewRecordingSource(db, store, pollInterval), logger: logger}
}

func (h *RecordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	start, ok := parseStart(r.URL.Query().Get("start"))
	if !ok {
		http.Error(w, "start must be a non-negative number of seconds", http.StatusBadRequest)

		return
	}
	recording, err := h.source.Open(r.Context(), recordingID)
	switch {
	case errors.Is(err, ErrRecordingNotFound), errors.Is(err, ErrNoSegment):
		http.Error(w, err.Error(), http.StatusNotFound)

		return
	case errors.Is(err, ErrNotMounted):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)

		return
	case err != nil:
		h.internalError(w, r, err)

		return
	}

	// download を付けたら、録画ファイルの名前で保存させる (#474)。録画中の録画は、長さが決まらないので保存させない。
	download := r.URL.Query().Has("download")
	if download && r.URL.Query().Has("start") {
		http.Error(w, "start cannot be combined with download; the whole recording is downloaded", http.StatusBadRequest)

		return
	}
	if download && recording.InProgress {
		http.Error(w, "the recording is still in progress; download it after it finishes", http.StatusConflict)

		return
	}
	if !recording.InProgress && r.URL.Query().Get("start") == "" {
		if download {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": recording.FileName()}))
		}
		h.serveContent(w, r, recording)

		return
	}
	writeStreamHeader(w)
	if r.Method == http.MethodHead {
		return
	}
	if err := recording.Write(r.Context(), w, recording.Seek(start)); err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "failed to stream the recording", slog.String("recording_id", recordingID.String()), slog.Any("error", err))
	}
}

// serveContent は、録画を終えた録画を、長さの決まった 1 本のファイルとして返す。Range に答えるので、外部プレイヤーがシークできる (#535)。
func (h *RecordingHandler) serveContent(w http.ResponseWriter, r *http.Request, recording *Recording) {
	content, err := recording.Content()
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	defer content.Close()
	w.Header().Set("Content-Type", contentTypeTS)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "", time.Time{}, content)
}

// parseStart は、開始位置の秒を読む。空なら 0 にする。
func parseStart(raw string) (time.Duration, bool) {
	if raw == "" {
		return 0, true
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(seconds) || seconds < 0 || seconds > float64(24*time.Hour/time.Second) {
		return 0, false
	}

	return time.Duration(seconds * float64(time.Second)), true
}

func (h *RecordingHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to prepare the recording stream", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
