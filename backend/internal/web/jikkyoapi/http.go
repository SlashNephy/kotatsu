package jikkyoapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/infra/jikkyoclient"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/jikkyo"
)

// segmentKind は、録画のファイルのうち、TS のセグメントの種類。
const segmentKind = "segment"

// CommentsFileName は、過去ログの URL の末尾のファイル名。
const CommentsFileName = "jikkyo.json"

// finishedCacheControl は、録画を終えた後の過去ログの Cache-Control。過去ログは後から増えることがまれにあるので、
// 少しの間だけ使い回させる。ログイン中のユーザーにだけ見せるので、共有のキャッシュには置かせない。
const finishedCacheControl = "private, max-age=600"

// CommentsHandler は、録画の時間帯の実況の過去ログを、過去ログ API から取って中継する
// (GET /media/recordings/{id}/jikkyo.json、設計書 7.4)。
// 描画とミュートに使う項目だけを、録画の始まり (最初のセグメントを書き始めた時刻) からの秒とともに返す。
// ユーザー ID は、過去ログ API が公開している匿名の ID で、ID でのミュートに使う (#488)。
type CommentsHandler struct {
	queries *query.Queries
	table   *jikkyo.Table
	client  *jikkyoclient.Client
	clock   clock.Clock
	logger  *slog.Logger
}

var _ http.Handler = new(CommentsHandler)

// NewCommentsHandler は CommentsHandler を返す。
func NewCommentsHandler(pool *pgxpool.Pool, table *jikkyo.Table, client *jikkyoclient.Client, clk clock.Clock, logger *slog.Logger) *CommentsHandler {
	return &CommentsHandler{queries: query.New(pool), table: table, client: client, clock: clk, logger: logger}
}

type commentsResponse struct {
	// Channel は、録画したサービスの実況のチャンネル。対応するチャンネルがなければ null。
	Channel *channelJSON `json:"channel"`
	// Origin は、コメントの時刻 (Time) の基準の時刻 (最初のセグメントを書き始めた時刻)。チャンネルがなければ null。
	// 録画中の追っかけ再生で、リアルタイムのコメントを同じ基準の位置に流すのに使う (#667)。
	Origin   *time.Time     `json:"origin"`
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
	UserID  string  `json:"userId"`
}

func (h *CommentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	source, err := h.queries.GetRecordingJikkyoSource(r.Context(), recordingID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "the recording is not found", http.StatusNotFound)

		return
	}
	if err != nil {
		h.internalError(w, r, err)

		return
	}

	resp := &commentsResponse{Channel: nil, Origin: nil, Comments: []*commentJSON{}}
	id, ok := h.table.Lookup(source.NetworkID, source.Sid)
	if !ok {
		h.write(w, r, resp, source.RecordedUntil != nil)

		return
	}
	resp.Channel = &channelJSON{ID: id}
	from, err := h.origin(r.Context(), recordingID, source.RecordedFrom)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	resp.Origin = &from
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
			Time: comment.At.Sub(from).Seconds(), Mail: comment.Mail, Content: comment.Content, UserID: comment.UserID,
		})
	}
	h.write(w, r, resp, source.RecordedUntil != nil)
}

// origin は、コメントの時刻の基準を返す。録画の配信は、最初のセグメントを書き始めた時刻を 0 秒にするので、それに合わせる。
// 録画の行を作ってから、チューナーを開いて書き始めるまでには時間がかかるので、recorded_from とはずれる。
// セグメントがまだなければ、recorded_from を使う。
func (h *CommentsHandler) origin(ctx context.Context, recordingID uuid.UUID, recordedFrom time.Time) (time.Time, error) {
	files, err := h.queries.ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to list the recording files: %w", err)
	}
	origin := time.Time{}
	for _, file := range files {
		if file.Kind == segmentKind && (origin.IsZero() || file.StartedAt.Before(origin)) {
			origin = file.StartedAt
		}
	}
	if origin.IsZero() {
		return recordedFrom, nil
	}

	return origin, nil
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
