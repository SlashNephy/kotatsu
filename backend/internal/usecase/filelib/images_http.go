package filelib

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/lib/sharedfile"
)

// imageCacheControl は、画像の Cache-Control。URL にファイルの ID を付けて変わったら別の URL にするので、録画の画像と同じく少しの間使い回させる。
// ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const imageCacheControl = "private, max-age=3600"

// ImageHandler は、ライブラリのファイルの代表画像と、シークバー用のプレビュー画像とその説明を配信する
// (GET /internal/media/library/{id}/{file}、file は thumbnail.jpg、preview.jpg、preview.json のどれか。設計書 5.8)。
// 今のファイル (大きさと更新日時) から作った画像だけを返す。書き換えた後のファイルに、前のファイルの画像を出さない。
type ImageHandler struct {
	source  *Source
	indexer *Indexer
	logger  *slog.Logger
}

var _ http.Handler = new(ImageHandler)

// NewImageHandler は ImageHandler を返す。
func NewImageHandler(source *Source, indexer *Indexer, logger *slog.Logger) *ImageHandler {
	return &ImageHandler{source: source, indexer: indexer, logger: logger}
}

func (h *ImageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 見つからないなどの失敗の応答は、覚えさせない。返せたときは、下で上書きする。
	w.Header().Set("Cache-Control", "no-store")
	ref, ok := DecodeRef(r.PathValue("id"))
	if !ok {
		http.Error(w, "invalid file ID", http.StatusBadRequest)

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
	file, err := h.source.Open(r.Context(), ref)
	if errors.Is(err, medialib.ErrNotFound) || errors.Is(err, ErrNotAFile) || errors.Is(err, medialib.ErrInvalidPath) {
		http.Error(w, "file not found", http.StatusNotFound)

		return
	}
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	if !h.indexer.HasImages(ref, file.ID()) {
		http.Error(w, "the file has no such image yet", http.StatusNotFound)

		return
	}
	f, err := sharedfile.Open(h.indexer.ImagePath(ref, name))
	if errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "the file has no such image", http.StatusNotFound)

		return
	}
	if err != nil {
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
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (h *ImageHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to serve the library file image", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
