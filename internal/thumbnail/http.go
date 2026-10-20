package thumbnail

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// imageCacheControl は、録画の画像の Cache-Control。画像は、録画を解析し直すまで変わらないので、しばらく使い回させる。
// ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const imageCacheControl = "private, max-age=3600"

// ImageHandler は、録画の代表画像と、シークバー用のプレビュー画像とその説明を配信する
// (GET /media/recordings/{id}/{file}、file は thumbnail.jpg、preview.jpg、preview.json のどれか)。
// ファイルは、データディレクトリの、画像を置くセグメント (ImageSegment) のディレクトリから読む。
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
	// 見つからない、マウントしていない、などの失敗の応答は、ブラウザにも前段のプロキシにも覚えさせない (#426)。
	// 覚えられると、ファイルを戻した後も、キャッシュを無効にして読み直すまで画像が出ない。返せたときは、下で上書きする。
	w.Header().Set("Cache-Control", "no-store")
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	var contentType string
	name := r.PathValue("file")
	switch name {
	case ThumbnailFileName, PreviewFileName:
		contentType = "image/jpeg"
	case ManifestFileName:
		contentType = "application/json"
	default:
		http.NotFound(w, r)

		return
	}

	files, err := sqlc.New(h.db).ListRecordingFiles(r.Context(), recordingID)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	owner, ok := ImageSegment(files)
	if !ok {
		http.Error(w, "the recording has no such image", http.StatusNotFound)

		return
	}
	image := kago.DataLocation(owner, name)
	f, err := h.store.Open(image.PoolID, image.Path)
	switch {
	case errors.Is(err, kago.ErrPoolNotMounted):
		http.Error(w, "the data directory is not mounted on this instance", http.StatusServiceUnavailable)

		return
	case errors.Is(err, fs.ErrNotExist):
		http.Error(w, "the recording has no such image", http.StatusNotFound)

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
