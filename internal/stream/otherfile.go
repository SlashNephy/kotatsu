package stream

import (
	"errors"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// kindOther は、EPGStation から取り込んだ、TS 以外のファイルの種類 (設計書 6.3)。
const kindOther = "other"

// contentTypeOf は、拡張子 ext のファイルの種類を返す。分からなければ application/octet-stream にする。
// 取り込むことの多い動画は、自分で持つ。コンテナイメージには、拡張子から種類を引く mime.types がないからである。
func contentTypeOf(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".ts", ".m2ts":
		return "video/mp2t"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}

	return "application/octet-stream"
}

// OtherFileURL は、録画のその他のファイルを流す URL のパスを返す。
func OtherFileURL(recordingID, fileID uuid.UUID) string {
	return "/streams/recordings/" + recordingID.String() + "/files/" + fileID.String()
}

// OtherFileHandler は、EPGStation から取り込んだ、エンコード済みの MP4 のような TS 以外のファイルを、そのまま流す
// (GET /streams/recordings/{id}/files/{file}、設計書 6.3)。ブラウザがそのコーデックに対応していれば、そのまま再生できる。
// ファイルは、このインスタンスにマウントしたプールから読む。
type OtherFileHandler struct {
	db     sqlc.DBTX
	store  *kago.Store
	logger *slog.Logger
}

var _ http.Handler = new(OtherFileHandler)

// NewOtherFileHandler は OtherFileHandler を返す。
func NewOtherFileHandler(db sqlc.DBTX, store *kago.Store, logger *slog.Logger) *OtherFileHandler {
	return &OtherFileHandler{db: db, store: store, logger: logger}
}

func (h *OtherFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	fileID, err := uuid.Parse(r.PathValue("file"))
	if err != nil {
		http.Error(w, "invalid file ID", http.StatusBadRequest)

		return
	}
	files, err := sqlc.New(h.db).ListRecordingFiles(r.Context(), recordingID)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	var file *sqlc.RecordingFile
	for i := range files {
		if files[i].ID == fileID && files[i].Kind == kindOther {
			file = &files[i]
		}
	}
	if file == nil {
		http.Error(w, "the recording has no such file", http.StatusNotFound)

		return
	}

	f, err := h.store.Open(file.PoolID, file.Path)
	switch {
	case errors.Is(err, kago.ErrPoolNotMounted):
		http.Error(w, "the pool is not mounted on this instance", http.StatusServiceUnavailable)

		return
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, "the file is missing", http.StatusNotFound)

		return
	case err != nil:
		h.internalError(w, r, err)

		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		h.internalError(w, r, err)

		return
	}

	w.Header().Set("Content-Type", contentTypeOf(path.Ext(file.Path)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// ServeContent は、Range に答えるので、シークできる。
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (h *OtherFileHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to serve the recording file", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
