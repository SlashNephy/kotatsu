package thumbnail

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"path"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// imageCacheControl は、録画の画像の Cache-Control。画像は、録画を解析し直すまで変わらないので、しばらく使い回させる。
// ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const imageCacheControl = "private, max-age=3600"

// ImageHandler は、録画の代表画像と、シークバー用のプレビュー画像とその説明を配信する
// (GET /media/recordings/{id}/{file}、file は thumbnail.jpg、preview.jpg、preview.json のどれか)。
// ファイルは、このインスタンスにマウントしたプールから読む。
type ImageHandler struct {
	db     sqlc.DBTX
	store  *kago.Store
	logger *slog.Logger
}

var _ http.Handler = new(ImageHandler)

// NewImageHandler は ImageHandler を返す。
func NewImageHandler(db sqlc.DBTX, store *kago.Store, logger *slog.Logger) *ImageHandler {
	return &ImageHandler{db: db, store: store, logger: logger}
}

func (h *ImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	// 説明のファイルは、recording_files に載せず、プレビュー画像と同じディレクトリに置く。
	var kind, contentType string
	manifest := false
	switch r.PathValue("file") {
	case ThumbnailFileName:
		kind, contentType = kindThumbnail, "image/jpeg"
	case PreviewFileName:
		kind, contentType = kindPreview, "image/jpeg"
	case ManifestFileName:
		kind, contentType, manifest = kindPreview, "application/json", true
	default:
		http.NotFound(w, r)

		return
	}

	files, err := sqlc.New(h.db).ListRecordingFiles(r.Context(), recordingID)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	var file *sqlc.RecordingFile
	for i := range files {
		if files[i].Kind == kind {
			file = &files[i]
		}
	}
	if file == nil {
		http.Error(w, "the recording has no such image", http.StatusNotFound)

		return
	}
	rel := file.Path
	if manifest {
		rel = path.Join(path.Dir(rel), ManifestFileName)
	}

	f, err := h.store.Open(file.PoolID, rel)
	switch {
	case errors.Is(err, kago.ErrPoolNotMounted):
		http.Error(w, "the pool is not mounted on this instance", http.StatusServiceUnavailable)

		return
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, "the image file is missing", http.StatusNotFound)

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

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", imageCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// ServeContent は、更新日時で If-Modified-Since に答え、Range にも答える。
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (h *ImageHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to serve the recording image", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
