package jikkyo

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// CommentsFileName は、過去ログの URL の末尾のファイル名。
const CommentsFileName = "jikkyo.json"

// finishedCacheControl は、録画を終えた後の過去ログの Cache-Control。過去ログは後から増えることがまれにあるので、
// 少しの間だけ使い回させる。ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const finishedCacheControl = "private, max-age=600"

// CommentsHandler は、録画の時間帯の実況の過去ログを、過去ログ API から取って中継する
// (GET /media/recordings/{id}/jikkyo.json、設計書 7.4)。
// 描画に使う項目だけを、録画の始まりからの秒とともに返す。ユーザー ID は返さない。
type CommentsHandler struct {
	db     sqlc.DBTX
	table  *Table
	client *Client
	clock  clock.Clock
	logger *slog.Logger
}

var _ http.Handler = new(CommentsHandler)

// NewCommentsHandler は CommentsHandler を返す。
func NewCommentsHandler(db sqlc.DBTX, table *Table, client *Client, clk clock.Clock, logger *slog.Logger) *CommentsHandler {
	return &CommentsHandler{db: db, table: table, client: client, clock: clk, logger: logger}
}

type commentsResponse struct {
	// Channel は、録画したサービスの実況のチャンネル。対応するチャンネルがなければ null。
	Channel  *channelJSON   `json:"channel"`
	Comments []*commentJSON `json:"comments"`
}

type channelJSON struct {
	ID string `json:"id"`
}

type commentJSON struct {
	// Time は、録画の始まりからの秒。
	Time    float64 `json:"time"`
	Mail    string  `json:"mail"`
	Content string  `json:"content"`
}

func (h *CommentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	source, err := sqlc.New(h.db).GetRecordingJikkyoSource(r.Context(), recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "the recording is not found", http.StatusNotFound)

		return
	}
	if err != nil {
		h.internalError(w, r, err)

		return
	}

	resp := &commentsResponse{Channel: nil, Comments: []*commentJSON{}}
	id, ok := h.table.Lookup(source.NetworkID, source.Sid)
	if !ok {
		h.write(w, r, resp, source.RecordedUntil != nil)

		return
	}
	resp.Channel = &channelJSON{ID: id}
	from := source.RecordedFrom
	// 過去ログ API は、終わりが未来の時刻だと取得できないので、録画中なら今までにする。
	to := h.clock.Now()
	if source.RecordedUntil != nil && source.RecordedUntil.Before(to) {
		to = *source.RecordedUntil
	}
	// 録画を始めた直後は、時計のずれで終わりが始まりより前になることがある。過去ログ API はこれを拒むので、問い合わせない。
	if !to.After(from) {
		h.write(w, r, resp, source.RecordedUntil != nil)

		return
	}
	comments, err := h.client.Comments(r.Context(), id, from, to)
	if err != nil {
		h.logger.WarnContext(r.Context(), "failed to fetch the jikkyo comments",
			slog.String("recording_id", recordingID.String()), slog.String("channel", id), slog.Any("error", err))
		http.Error(w, "failed to fetch the jikkyo comments", http.StatusBadGateway)

		return
	}
	for _, comment := range comments {
		// 過去ログ API は秒の単位で絞るので、録画を始める前のコメントが混じる。
		if comment.At.Before(from) || comment.At.After(to) {
			continue
		}
		resp.Comments = append(resp.Comments, &commentJSON{
			Time: comment.At.Sub(from).Seconds(), Mail: comment.Mail, Content: comment.Content,
		})
	}
	h.write(w, r, resp, source.RecordedUntil != nil)
}

func (h *CommentsHandler) write(w http.ResponseWriter, r *http.Request, resp *commentsResponse, finished bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if finished {
		w.Header().Set("Cache-Control", finishedCacheControl)
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		// 書き始めた後なので、状態は変えられない。接続が切れたときなどに起きる。
		h.logger.WarnContext(r.Context(), "failed to write the jikkyo comments", slog.Any("error", err))
	}
}

func (h *CommentsHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.ErrorContext(r.Context(), "failed to serve the jikkyo comments", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
