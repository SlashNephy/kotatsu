package streamapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/SlashNephy/kotatsu/internal/infra/medialib"
	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
)

// LibraryHandler は、ライブラリのファイル (録画として登録していない TS) を配信する (GET /internal/streams/library/{id}?start=秒、設計書 5.8、7.2)。
// id は、ライブラリの名前とパスを符号化したもの (filelib.Ref.Encode)。
// 録画の再生と同じく、インデックスを使って start 秒の位置の手前の I フレームから流し、実際に流し始めた位置 (ファイルの始まりからの秒) を
// X-Kotatsu-Start で返す。HEAD でも返す。インデックスがまだできていなければ、作り始め、できた所までの I フレームから流す。
type LibraryHandler struct {
	source *filelib.Source
	logger *slog.Logger
}

var _ http.Handler = new(LibraryHandler)

// NewLibraryHandler は LibraryHandler を返す。
func NewLibraryHandler(source *filelib.Source, logger *slog.Logger) *LibraryHandler {
	return &LibraryHandler{source: source, logger: logger}
}

func (h *LibraryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ref, ok := filelib.DecodeRef(r.PathValue("id"))
	if !ok {
		http.Error(w, "invalid file ID", http.StatusBadRequest)

		return
	}
	start, ok := parseStart(r.URL.Query().Get("start"))
	if !ok {
		http.Error(w, "start must be a non-negative number of seconds", http.StatusBadRequest)

		return
	}
	file, err := h.source.Open(r.Context(), ref)
	switch {
	case errors.Is(err, medialib.ErrNotFound), errors.Is(err, filelib.ErrNotAFile):
		http.Error(w, "file not found", http.StatusNotFound)

		return
	case errors.Is(err, medialib.ErrInvalidPath):
		http.Error(w, "invalid file ID", http.StatusBadRequest)

		return
	case err != nil:
		h.logger.ErrorContext(r.Context(), "failed to prepare the library stream", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		return
	}
	pos := file.Recording.Seek(start)
	w.Header().Set(StartHeader, strconv.FormatFloat(pos.At.Seconds(), 'f', 3, 64))
	writeStreamHeader(w)
	if r.Method == http.MethodHead {
		return
	}
	if err := file.Recording.Write(r.Context(), w, pos); err != nil && r.Context().Err() == nil {
		h.logger.WarnContext(r.Context(), "failed to stream the library file",
			slog.String("library", ref.Library), slog.String("path", ref.Path), slog.Any("error", err))
	}
}
