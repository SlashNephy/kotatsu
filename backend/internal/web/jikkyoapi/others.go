package jikkyoapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
)

// OtherCommentsFileName は、別の放送の過去ログの URL の末尾のファイル名。
const OtherCommentsFileName = "jikkyo-others.json"

// maxOtherBroadcasts は、過去ログを取る別の放送の数の上限。古い順に使う。
const maxOtherBroadcasts = 5

type otherCommentsResponse struct {
	Broadcasts []*otherBroadcastJSON `json:"broadcasts"`
}

type otherBroadcastJSON struct {
	Channel *channelJSON `json:"channel"`
	// Comments の Time は、その放送の始まりからの秒。
	Comments []*commentJSON `json:"comments"`
}

// OtherCommentsHandler は、録画と同じ話の、別の放送の実況の過去ログを中継する (GET /media/recordings/{id}/jikkyo-others.json、#1361)。
// 全体の設定かユーザーの設定で実況を切っているか、別の放送を混ぜる設定を切っていれば、過去ログ API へ問い合わせずに 404 を返す。
type OtherCommentsHandler struct {
	*CommentsHandler
}

var _ http.Handler = new(OtherCommentsHandler)

// NewOtherCommentsHandler は OtherCommentsHandler を返す。
func NewOtherCommentsHandler(comments *CommentsHandler) *OtherCommentsHandler {
	return &OtherCommentsHandler{CommentsHandler: comments}
}

func (h *OtherCommentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	recordingID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid recording ID", http.StatusBadRequest)

		return
	}
	allowed, err := h.mixAllowed(r.Context())
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	if !allowed {
		http.NotFound(w, r)

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
	// 録画中の録画には混ぜない。区切りが揃わず、過去ログ API に問い合わせても使えない。
	if source.RecordedUntil == nil {
		h.writeOthers(w, r, &otherCommentsResponse{Broadcasts: []*otherBroadcastJSON{}}, false)

		return
	}
	broadcasts, err := h.otherBroadcasts(r.Context(), recordingID, &source)
	if err != nil {
		h.internalError(w, r, err)

		return
	}
	h.writeOthers(w, r, &otherCommentsResponse{Broadcasts: h.fetchOthers(r.Context(), recordingID, broadcasts)}, source.RecordedUntil != nil)
}

// mixAllowed は、リクエストのユーザーが、別の放送の過去ログを混ぜられるかどうかを返す。
func (h *OtherCommentsHandler) mixAllowed(ctx context.Context) (bool, error) {
	allowed, err := h.allowed(ctx)
	if err != nil || !allowed {
		return false, err
	}
	raw, err := h.queries.GetCommentSettingsJSON(ctx, auth.PrincipalFrom(ctx).UserID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && len(raw) == 0) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read the comment settings: %w", err)
	}
	var settings struct {
		MixOtherBroadcasts bool `json:"mixOtherBroadcasts"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return false, fmt.Errorf("failed to decode the comment settings: %w", err)
	}

	return settings.MixOtherBroadcasts, nil
}

// otherBroadcasts は、録画と同じ話の、別の放送を返す。1 話だけに結びついた録画だけを対象にする。
// 録画と同じチャンネルで時間帯が重なる放送は、録画自身の過去ログと同じなので除く。
func (h *OtherCommentsHandler) otherBroadcasts(ctx context.Context, recordingID uuid.UUID, source *domain.RecordingJikkyoSource) ([]*domain.OtherAnimeBroadcast, error) {
	rows, err := h.queries.ListOtherAnimeBroadcasts(ctx, recordingID, h.clock.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to list the other broadcasts: %w", err)
	}
	own, hasOwn := h.table.Lookup(source.NetworkID, source.Sid)
	until := *source.RecordedUntil
	seen := map[int32]bool{}
	broadcasts := make([]*domain.OtherAnimeBroadcast, 0, maxOtherBroadcasts)
	for _, row := range rows {
		start, end := broadcastSpan(row)
		if seen[row.PID] || (hasOwn && own == channelName(row.JikkyoChannelID) && start.Before(until) && source.RecordedFrom.Before(end)) {
			continue
		}
		seen[row.PID] = true
		broadcasts = append(broadcasts, row)
		if len(broadcasts) == maxOtherBroadcasts {
			break
		}
	}

	return broadcasts, nil
}

// fetchOthers は、放送ごとの過去ログを取る。取れない放送は除き、残りを返す。
func (h *OtherCommentsHandler) fetchOthers(ctx context.Context, recordingID uuid.UUID, broadcasts []*domain.OtherAnimeBroadcast) []*otherBroadcastJSON {
	result := make([]*otherBroadcastJSON, 0, len(broadcasts))
	for _, broadcast := range broadcasts {
		start, end := broadcastSpan(broadcast)
		channel := channelName(broadcast.JikkyoChannelID)
		comments, err := h.client.Comments(ctx, channel, start, end)
		if err != nil {
			h.logger.WarnContext(ctx, "failed to fetch the jikkyo comments of another broadcast",
				slog.String("recording_id", recordingID.String()), slog.String("channel", channel), slog.Any("error", err))

			continue
		}
		converted := make([]*commentJSON, 0, len(comments))
		for _, comment := range comments {
			if comment.At.Before(start) || comment.At.After(end) {
				continue
			}
			converted = append(converted, &commentJSON{
				Time: comment.At.Sub(start).Seconds(), Mail: comment.Mail, Content: comment.Content, UserID: comment.UserID,
			})
		}
		result = append(result, &otherBroadcastJSON{Channel: &channelJSON{ID: channel}, Comments: converted})
	}

	return result
}

// broadcastSpan は、放送の実際の始まりと終わりを返す。
func broadcastSpan(broadcast *domain.OtherAnimeBroadcast) (time.Time, time.Time) {
	offset := time.Duration(broadcast.OffsetSeconds) * time.Second

	return broadcast.StartAt.Add(offset), broadcast.EndAt.Add(offset)
}

// channelName は、実況のチャンネルの番号を、過去ログ API のチャンネルの ID にする。
func channelName(id int32) string {
	return "jk" + strconv.FormatInt(int64(id), 10)
}

func (h *OtherCommentsHandler) writeOthers(w http.ResponseWriter, r *http.Request, resp *otherCommentsResponse, finished bool) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if finished {
		w.Header().Set("Cache-Control", finishedCacheControl)
	} else {
		w.Header().Set("Cache-Control", noStoreCacheControl)
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.WarnContext(r.Context(), "failed to write the jikkyo comments of the other broadcasts", slog.Any("error", err))
	}
}
