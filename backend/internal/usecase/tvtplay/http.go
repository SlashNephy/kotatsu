package tvtplay

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"slices"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usecase/cmdetect"
	"github.com/SlashNephy/kotatsu/internal/usecase/settings"
	"github.com/SlashNephy/kotatsu/internal/usecase/transcode"
)

// ChapterFileName は、.chapter ファイルを返す URL の末尾のファイル名。
const ChapterFileName = "tvtplay.chapter"

// segmentKind は、録画のファイルのうち、TS のセグメントの種類。
const segmentKind = "segment"

// DownloadGate は、管理者が録画のダウンロードを有効にしているかを返す (#1762)。*settings.URLGate が満たす。
type DownloadGate interface {
	RecordingDownloadsEnabled(ctx context.Context) (bool, error)
}

var _ DownloadGate = new(settings.URLGate)

// ChapterHandler は、録画をダウンロードした TS と組で使う、TvtPlay の .chapter ファイルを返す
// (GET /internal/media/recordings/{id}/tvtplay.chapter、#1737)。
// 位置は、ダウンロードした 1 本の TS (セグメントをつなぎ、引き継ぎで重なった分を除いたもの、設計書 7.5) の始まりからの時間にする。
// 名前は、ダウンロードした TS (最初のセグメントの名前) の拡張子を .chapter に替えたもので、TvtPlay は TS と同じフォルダーに置けば読む。
// 録画のダウンロードと組で使うものなので、管理者がダウンロードを有効にしていなければ断る。
type ChapterHandler struct {
	db     sqlc.DBTX
	gate   DownloadGate
	logger *slog.Logger
}

var _ http.Handler = new(ChapterHandler)

// NewChapterHandler は ChapterHandler を返す。
func NewChapterHandler(db sqlc.DBTX, gate DownloadGate, logger *slog.Logger) *ChapterHandler {
	return &ChapterHandler{db: db, gate: gate, logger: logger}
}

func (h *ChapterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	if !h.downloadsEnabled(w, r) {
		return
	}
	q := sqlc.New(h.db)
	recording, err := q.GetRecording(r.Context(), recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)

		return
	}
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	// 録画中は、長さも CM の区間も決まらない。ダウンロードと同じく断る。
	if recording.RecordedUntil == nil {
		http.Error(w, "the recording is still in progress; download it after it finishes", http.StatusConflict)

		return
	}
	files, err := q.ListRecordingFiles(r.Context(), recordingID)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	first := slices.IndexFunc(files, func(f sqlc.RecordingFile) bool { return f.Kind == segmentKind })
	if first < 0 {
		http.Error(w, "the recording has no TS segment", http.StatusNotFound)

		return
	}
	sections, err := cmdetect.RecordingSections(r.Context(), h.db, recordingID)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	tl := &Timeline{RecordedFrom: recording.RecordedFrom, Gaps: []transcode.Gap{}, Duration: 0}
	if err := json.Unmarshal(recording.Gaps, &tl.Gaps); err != nil {
		h.internalError(w, r, err)

		return
	}
	tl.Duration = transcode.PlaybackPosition(tl.RecordedFrom, tl.Gaps, *recording.RecordedUntil)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": FileName(path.Base(files[first].Path)),
	}))
	_, _ = w.Write(Build(sections, tl))
}

// downloadsEnabled は、管理者が録画のダウンロードを有効にしているかを確かめる。有効でなければ、応答を書いて false を返す。
func (h *ChapterHandler) downloadsEnabled(w http.ResponseWriter, r *http.Request) bool {
	enabled, err := h.gate.RecordingDownloadsEnabled(r.Context())
	switch {
	case errors.Is(err, settings.ErrDatabaseUnavailable):
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)

		return false
	case err != nil:
		h.internalError(w, r, err)

		return false
	case !enabled:
		http.Error(w, "recording downloads are disabled by the administrator", http.StatusForbidden)

		return false
	}

	return true
}

func (h *ChapterHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to build the TvtPlay chapter file", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
