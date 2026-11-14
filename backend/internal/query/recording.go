package query

import (
	"context"
	"errors"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// SearchRecordingsParams は、SearchRecordings の条件と、続きから読む位置。
type SearchRecordingsParams struct {
	States     []string
	ServiceIDs []int64
	TagIDs     []uuid.UUID
	Keyword    *string
	RuleID     *uuid.UUID
	Genres     []int32
	Since      *time.Time
	Until      *time.Time
	BeforeAt   *time.Time
	BeforeID   *uuid.UUID
	MaxCount   int32
}

// SearchRecordingsOldestParams は、SearchRecordingsOldest の条件と、続きから読む位置。
type SearchRecordingsOldestParams struct {
	States     []string
	ServiceIDs []int64
	TagIDs     []uuid.UUID
	Keyword    *string
	RuleID     *uuid.UUID
	Genres     []int32
	Since      *time.Time
	Until      *time.Time
	AfterAt    *time.Time
	AfterID    *uuid.UUID
	MaxCount   int32
}

// ListAPIRecordingsParams は、ListAPIRecordings の条件。
type ListAPIRecordingsParams struct {
	Keyword  string
	Skip     int32
	MaxCount int32
}

// GetRecording は、録画を 1 つ返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetRecording(ctx context.Context, id uuid.UUID) (domain.Recording, error) {
	result, err := sqlc.New(q.pool).GetRecording(ctx, id)
	if err != nil {
		return domain.Recording{}, err
	}

	return domain.Recording(result), nil
}

// ListRecordingsByIDs は、ids の録画を返す。ない録画は返さない。
func (q *Queries) ListRecordingsByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Recording, error) {
	result, err := sqlc.New(q.pool).ListRecordingsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recording, len(result))
	for i := range result {
		out[i] = domain.Recording(result[i])
	}

	return out, nil
}

// SearchRecordings は、条件に合う録画を、新しい順に返す。
func (q *Queries) SearchRecordings(ctx context.Context, arg SearchRecordingsParams) ([]domain.Recording, error) {
	result, err := sqlc.New(q.pool).SearchRecordings(ctx, sqlc.SearchRecordingsParams{States: arg.States, ServiceIds: arg.ServiceIDs, TagIds: arg.TagIDs, Keyword: arg.Keyword, RuleID: arg.RuleID, Genres: arg.Genres, Since: arg.Since, Until: arg.Until, BeforeAt: arg.BeforeAt, BeforeID: arg.BeforeID, MaxCount: arg.MaxCount})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recording, len(result))
	for i := range result {
		out[i] = domain.Recording(result[i])
	}

	return out, nil
}

// SearchRecordingsOldest は、条件に合う録画を、古い順に返す。
func (q *Queries) SearchRecordingsOldest(ctx context.Context, arg SearchRecordingsOldestParams) ([]domain.Recording, error) {
	result, err := sqlc.New(q.pool).SearchRecordingsOldest(ctx, sqlc.SearchRecordingsOldestParams{States: arg.States, ServiceIds: arg.ServiceIDs, TagIds: arg.TagIDs, Keyword: arg.Keyword, RuleID: arg.RuleID, Genres: arg.Genres, Since: arg.Since, Until: arg.Until, AfterAt: arg.AfterAt, AfterID: arg.AfterID, MaxCount: arg.MaxCount})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Recording, len(result))
	for i := range result {
		out[i] = domain.Recording(result[i])
	}

	return out, nil
}

// ListRecordingFiles は、録画のファイルを返す。
func (q *Queries) ListRecordingFiles(ctx context.Context, recordingID uuid.UUID) ([]domain.RecordingFile, error) {
	result, err := sqlc.New(q.pool).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecordingFile, len(result))
	for i := range result {
		out[i] = domain.RecordingFile(result[i])
	}

	return out, nil
}

// ListRecordingFilesByRecordings は、録画ごとのファイルをまとめて返す。
func (q *Queries) ListRecordingFilesByRecordings(ctx context.Context, recordingIDs []uuid.UUID) ([]domain.RecordingFile, error) {
	result, err := sqlc.New(q.pool).ListRecordingFilesByRecordings(ctx, recordingIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecordingFile, len(result))
	for i := range result {
		out[i] = domain.RecordingFile(result[i])
	}

	return out, nil
}

// ListRecordingTags は、録画ごとのタグをまとめて返す。
func (q *Queries) ListRecordingTags(ctx context.Context, recordingIDs []uuid.UUID) ([]domain.RecordingTag, error) {
	result, err := sqlc.New(q.pool).ListRecordingTags(ctx, recordingIDs)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecordingTag, len(result))
	for i := range result {
		out[i] = domain.RecordingTag(result[i])
	}

	return out, nil
}

// ListTags は、タグを返す。
func (q *Queries) ListTags(ctx context.Context) ([]domain.Tag, error) {
	result, err := sqlc.New(q.pool).ListTags(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Tag, len(result))
	for i := range result {
		out[i] = domain.Tag(result[i])
	}

	return out, nil
}

// IsReservationRecording は、予約の録画が録画中かを返す。
func (q *Queries) IsReservationRecording(ctx context.Context, reservationID *uuid.UUID) (bool, error) {
	result, err := sqlc.New(q.pool).IsReservationRecording(ctx, reservationID)
	if err != nil {
		return false, err
	}

	return result, nil
}

// GetPlayerSettings は、ユーザーのプレイヤーの設定を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetPlayerSettings(ctx context.Context, userID uuid.UUID) (domain.PlayerSettings, error) {
	result, err := sqlc.New(q.pool).GetPlayerSettings(ctx, userID)
	if err != nil {
		return domain.PlayerSettings{}, err
	}

	return domain.PlayerSettings(result), nil
}

// GetAPIRecording は、EPGStation 互換 API の ID の録画を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetAPIRecording(ctx context.Context, apiID int64) (domain.APIRecording, error) {
	result, err := sqlc.New(q.pool).GetAPIRecording(ctx, apiID)
	if err != nil {
		return domain.APIRecording{}, err
	}

	return domain.APIRecording(result), nil
}

// GetRecordingIDByAPIID は、EPGStation 互換 API の ID から、録画の ID を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetRecordingIDByAPIID(ctx context.Context, apiID int64) (uuid.UUID, error) {
	result, err := sqlc.New(q.pool).GetRecordingIDByAPIID(ctx, apiID)
	if err != nil {
		return uuid.UUID{}, err
	}

	return result, nil
}

// CountAPIRecordings は、EPGStation 互換 API の録画の数を返す。
// keyword は、ListAPIRecordings と同じく、正規化してから探す。
func (q *Queries) CountAPIRecordings(ctx context.Context, keyword string) (int64, error) {
	result, err := sqlc.New(q.pool).CountAPIRecordings(ctx, apiKeyword(keyword))
	if err != nil {
		return 0, err
	}

	return result, nil
}

// ListAPIRecordings は、EPGStation 互換 API の録画を返す。
// キーワードは、画面の録画の検索と同じく、正規化して、番組名の正規化した列から探す。% と _ は、文字そのものとして探す (#1726)。
func (q *Queries) ListAPIRecordings(ctx context.Context, arg ListAPIRecordingsParams) ([]domain.APIRecording, error) {
	params := sqlc.ListAPIRecordingsParams(arg)
	params.Keyword = apiKeyword(arg.Keyword)
	result, err := sqlc.New(q.pool).ListAPIRecordings(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]domain.APIRecording, len(result))
	for i := range result {
		out[i] = domain.APIRecording(result[i])
	}

	return out, nil
}

// ListContinueWatchingParams は、ListContinueWatching の条件。
type ListContinueWatchingParams struct {
	UserID        uuid.UUID
	MinPositionMs int64
	EndMarginMs   int64
	FinishedRatio float64
	MaxCount      int32
}

// ListContinueWatching は、ユーザーが途中まで見た録画を、見た時刻の新しい順に返す。
func (q *Queries) ListContinueWatching(ctx context.Context, arg ListContinueWatchingParams) ([]domain.ContinueWatching, error) {
	result, err := sqlc.New(q.pool).ListContinueWatching(ctx, sqlc.ListContinueWatchingParams(arg))
	if err != nil {
		return nil, err
	}
	out := make([]domain.ContinueWatching, len(result))
	for i := range result {
		out[i] = domain.ContinueWatching{Recording: domain.Recording(result[i].Recording)}
	}

	return out, nil
}

// ListWatchProgressParams は、ListWatchProgress の条件。
type ListWatchProgressParams struct {
	UserID       uuid.UUID
	RecordingIDs []uuid.UUID
}

// ListWatchProgress は、ユーザーの、録画ごとの見た位置を返す。
func (q *Queries) ListWatchProgress(ctx context.Context, arg ListWatchProgressParams) ([]domain.WatchProgress, error) {
	result, err := sqlc.New(q.pool).ListWatchProgress(ctx, sqlc.ListWatchProgressParams{UserID: arg.UserID, RecordingIds: arg.RecordingIDs})
	if err != nil {
		return nil, err
	}
	out := make([]domain.WatchProgress, len(result))
	for i := range result {
		out[i] = domain.WatchProgress(result[i])
	}

	return out, nil
}

// GetRecordingJikkyoSource は、録画の実況のコメントを引くための、録画したサービスと時刻を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetRecordingJikkyoSource(ctx context.Context, id uuid.UUID) (domain.RecordingJikkyoSource, error) {
	result, err := sqlc.New(q.pool).GetRecordingJikkyoSource(ctx, id)
	if err != nil {
		return domain.RecordingJikkyoSource{}, err
	}

	return domain.RecordingJikkyoSource(result), nil
}

// GetRecordingLiveStats は、録画中の録画の、書いているセグメントの様子を返す (#829)。録画中でないか、since より後に
// 書いていなければ pgx.ErrNoRows を返す。
func (q *Queries) GetRecordingLiveStats(ctx context.Context, recordingID uuid.UUID, since time.Time) (domain.RecordingLiveStats, error) {
	result, err := sqlc.New(q.pool).GetRecordingLiveStats(ctx, sqlc.GetRecordingLiveStatsParams{RecordingID: recordingID, Since: since})
	if err != nil {
		return domain.RecordingLiveStats{}, err
	}

	return domain.RecordingLiveStats(result), nil
}

// GetCommentSettingsJSON は、ユーザーの実況のコメントの設定の JSON を返す。設定を変えていなければ nil を返す。
func (q *Queries) GetCommentSettingsJSON(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	raw, err := sqlc.New(q.pool).GetCommentSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return raw, err
}

// ListOtherAnimeBroadcasts は、録画と同じ話の、now までに終わった別の放送を返す (#1361)。放送の古い順に並べる。
// 1 話だけに結びついた録画だけを対象にする。ほかの録画では空を返す。
func (q *Queries) ListOtherAnimeBroadcasts(ctx context.Context, recordingID uuid.UUID, now time.Time) ([]*domain.OtherAnimeBroadcast, error) {
	queries := sqlc.New(q.pool)
	episodes, err := queries.ListRecordingEpisodesForBroadcasts(ctx, recordingID)
	if err != nil {
		return nil, err
	}
	broadcasts := []*domain.OtherAnimeBroadcast{}
	if len(episodes) != 1 || episodes[0].SyobocalTid == nil || episodes[0].CountStart == nil || episodes[0].CountEnd == nil ||
		*episodes[0].CountStart != *episodes[0].CountEnd {
		return broadcasts, nil
	}
	rows, err := queries.ListOtherAnimeBroadcasts(ctx, sqlc.ListOtherAnimeBroadcastsParams{
		Tid: *episodes[0].SyobocalTid, EpisodeCount: *episodes[0].CountStart, Now: now,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		broadcasts = append(broadcasts, &domain.OtherAnimeBroadcast{
			PID: row.Pid, StartAt: row.StartAt, EndAt: row.EndAt, OffsetSeconds: row.OffsetSeconds, JikkyoChannelID: row.JikkyoChannelID,
		})
	}

	return broadcasts, nil
}

// GetRecordingNicologPath は、録画に対応する NICOLOG のファイルのパスを返す (#1363)。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetRecordingNicologPath(ctx context.Context, recordingID uuid.UUID) (string, error) {
	path, err := sqlc.New(q.pool).GetRecordingNicologPath(ctx, recordingID)
	if err != nil {
		return "", err
	}
	if path == nil {
		return "", pgx.ErrNoRows
	}

	return *path, nil
}

// apiKeyword は、EPGStation 互換 API の録画のキーワードを、正規化した列に当てる LIKE のパターンにする。空なら空を返す。
func apiKeyword(keyword string) string {
	return escapeLike(domain.Normalize(strings.TrimSpace(keyword)))
}
