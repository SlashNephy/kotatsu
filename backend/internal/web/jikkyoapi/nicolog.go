package jikkyoapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/nicolog"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// noStoreCacheControl は、キャッシュさせないときの Cache-Control。
const noStoreCacheControl = "no-store"

// NicologCommentsFileName は、上映会のコメントの URL の末尾のファイル名。
const NicologCommentsFileName = "jikkyo-nicolog.json"

// NicologFetcher は、NICOLOG のコメントの取得。
type NicologFetcher interface {
	Comments(ctx context.Context, path string) ([]*nicolog.Comment, error)
}

var _ NicologFetcher = new(nicolog.Client)

type nicologCommentsResponse struct {
	// Comments の Time は、上映会の開始からの秒。録画の位置に合わせるのは画面で行う。
	Comments []*commentJSON `json:"comments"`
}

// NicologCommentsHandler は、録画に対応する NICOLOG のコメントを中継する (GET /internal/media/recordings/{id}/jikkyo-nicolog.json、#1363)。
// 全体の設定かユーザーの設定で NICOLOG のソースをオプトインしていなければ、問い合わせずに 404 を返す。
type NicologCommentsHandler struct {
	*CommentsHandler

	fetcher NicologFetcher
}

var _ http.Handler = new(NicologCommentsHandler)

// NewNicologCommentsHandler は NicologCommentsHandler を返す。
func NewNicologCommentsHandler(comments *CommentsHandler, fetcher NicologFetcher) *NicologCommentsHandler {
	return &NicologCommentsHandler{CommentsHandler: comments, fetcher: fetcher}
}

func (h *NicologCommentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	allowed, err := h.sourceAllowed(r.Context())
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	if !allowed {
		http.NotFound(w, r)

		return
	}
	path, err := h.queries.GetRecordingNicologPath(r.Context(), recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		// 見つかっていないだけで、後からジョブが見つけることがあるので、キャッシュさせない。
		h.writeNicolog(w, r, &nicologCommentsResponse{Comments: []*commentJSON{}}, noStoreCacheControl)

		return
	}
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	comments, err := h.fetcher.Comments(r.Context(), path)
	if err != nil {
		// NICOLOG から取得できなくても、ほかのコメントは流せる。この機能だけを止める。
		h.logger.WarnContext(r.Context(), "failed to fetch the nicolog comments", slog.String("recording_id", recordingID.String()), slog.Any("error", err))
		http.Error(w, "failed to fetch the nicolog comments", http.StatusBadGateway)

		return
	}
	converted := make([]*commentJSON, 0, len(comments))
	for _, comment := range comments {
		converted = append(converted, &commentJSON{Time: comment.Time, Mail: comment.Mail, Content: comment.Content, UserID: comment.UserID})
	}
	h.writeNicolog(w, r, &nicologCommentsResponse{Comments: converted}, finishedCacheControl)
}

// sourceAllowed は、リクエストのユーザーが NICOLOG のソースを使えるかどうかを返す。
func (h *NicologCommentsHandler) sourceAllowed(ctx context.Context) (bool, error) {
	principal := auth.PrincipalFrom(ctx)
	if principal == nil {
		return false, nil
	}
	access, err := h.queries.GetCommentSourceAccess(ctx, principal.UserID)
	if err != nil {
		return false, fmt.Errorf("failed to read the comment sources: %w", err)
	}

	return access.For(domain.CommentSourceNicolog).Allowed(), nil
}

func (h *NicologCommentsHandler) writeNicolog(w http.ResponseWriter, r *http.Request, resp *nicologCommentsResponse, cacheControl string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", cacheControl)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.WarnContext(r.Context(), "failed to write the nicolog comments", slog.Any("error", err))
	}
}
