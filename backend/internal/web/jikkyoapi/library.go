package jikkyoapi

import (
	"log/slog"
	"net/http"

	"github.com/SlashNephy/kotatsu/internal/usecase/filelib"
)

// LibraryCommentsHandler は、ライブラリのファイル (録画として登録していない TS) の時間帯の実況の過去ログを、過去ログ API から取ってプロキシする
// (GET /internal/media/library/{id}/jikkyo.json、設計書 5.8、7.4)。id は filelib.Ref.Encode で作ったもの。
// 局と時間帯は、TS の中の SDT と TOT から読んだもの (filelib.Broadcast) を使い、Kotatsu のサービスがなくても引く。
// 返す形とコメントの時刻の基準 (TOT で読んだ録り始め) は、録画の過去ログ (CommentsHandler) と同じ。
type LibraryCommentsHandler struct {
	comments *CommentsHandler
	source   *filelib.Source
}

var _ http.Handler = new(LibraryCommentsHandler)

// NewLibraryCommentsHandler は LibraryCommentsHandler を返す。
func NewLibraryCommentsHandler(comments *CommentsHandler, source *filelib.Source) *LibraryCommentsHandler {
	return &LibraryCommentsHandler{comments: comments, source: source}
}

func (h *LibraryCommentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	allowed, err := h.comments.allowed(r.Context())
	if err != nil {
		h.comments.internalError(w, r, err)

		return
	}
	if !allowed {
		http.NotFound(w, r)

		return
	}
	ref, ok := filelib.DecodeRef(r.PathValue("id"))
	if !ok {
		http.Error(w, "invalid file ID", http.StatusBadRequest)

		return
	}
	file, err := h.source.Open(r.Context(), ref)
	if err != nil {
		// ないファイルも、再生できないファイルも、見つからないものとして答える。
		http.Error(w, "file not found", http.StatusNotFound)

		return
	}
	s, ok := librarySpan(file)
	if !ok {
		// 局か時間帯が分からなければ、チャンネルなしとして返す。インデックスを作り終えるまでは、後で分かるので使い回させない。
		h.comments.write(w, r, &commentsResponse{Channel: nil, Origin: nil, Comments: []*commentJSON{}}, file.Index.Complete)

		return
	}
	h.comments.serve(w, r, s, slog.String("library_file", ref.Library+"/"+ref.Path))
}

// librarySpan は、ライブラリのファイルの局と時間帯を返す。局か録り始めが分からなければ false を返す。
func librarySpan(file *filelib.File) (*span, bool) {
	broadcast := file.Index.Broadcast
	if broadcast == nil || broadcast.NetworkID == nil || broadcast.RecordedFrom == nil {
		return nil, false
	}
	// 終わりは、終わりで最後に読んだ TOT と、インデックスから求めた長さの終わりの、遅い方にする。短いファイルでは、TOT を 1 度しか読めず、
	// 終わりが始まりと同じになるからである。どちらも分からなければ false を返す。
	until := broadcast.RecordedUntil
	if _, duration, ok := file.Timeline(); ok {
		if end := broadcast.RecordedFrom.Add(duration); until == nil || end.After(*until) {
			until = &end
		}
	}
	if until == nil {
		return nil, false
	}

	// インデックスを作り終えるまでは、長さが伸びるので、使い回させない。
	return &span{
		networkID: int64(*broadcast.NetworkID), sid: int64(broadcast.ServiceID), from: *broadcast.RecordedFrom, until: until, final: file.Index.Complete,
	}, true
}
