package query

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// SearchAuditEventsParams は、SearchAuditEvents の条件と、続きから読む位置。
type SearchAuditEventsParams struct {
	ActionPrefix *string
	ActorUserID  *uuid.UUID
	TargetType   *string
	TargetID     *string
	Since        *time.Time
	Until        *time.Time
	BeforeAt     *time.Time
	BeforeID     *uuid.UUID
	MaxCount     int32
}

// GetUser は、ユーザーを 1 つ返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	result, err := sqlc.New(q.pool).GetUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}

	return domain.User(result), nil
}

// ListUsersByIDs は、ids のユーザーを返す。ないユーザーは返さない。
func (q *Queries) ListUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.User, error) {
	result, err := sqlc.New(q.pool).ListUsersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, len(result))
	for i := range result {
		out[i] = domain.User(result[i])
	}

	return out, nil
}

// GetUserPicture は、ユーザーのアイコンの URL を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetUserPicture(ctx context.Context, userID uuid.UUID) (string, error) {
	result, err := sqlc.New(q.pool).GetUserPicture(ctx, userID)
	if err != nil {
		return "", err
	}

	return result, nil
}

// SearchAuditEvents は、条件に合う監査ログを、新しい順に返す。
func (q *Queries) SearchAuditEvents(ctx context.Context, arg SearchAuditEventsParams) ([]domain.AuditEvent, error) {
	result, err := sqlc.New(q.pool).SearchAuditEvents(ctx, sqlc.SearchAuditEventsParams(arg))
	if err != nil {
		return nil, err
	}
	out := make([]domain.AuditEvent, len(result))
	for i := range result {
		out[i] = domain.AuditEvent(result[i])
	}

	return out, nil
}

// GetTunerServerStatus は、チューナーサーバーの状態を返す。なければ pgx.ErrNoRows を返す。
func (q *Queries) GetTunerServerStatus(ctx context.Context) (domain.TunerServerStatus, error) {
	result, err := sqlc.New(q.pool).GetTunerServerStatus(ctx)
	if err != nil {
		return domain.TunerServerStatus{}, err
	}

	return domain.TunerServerStatus(result), nil
}

// ListStoragePools は、保存先プールを返す。
func (q *Queries) ListStoragePools(ctx context.Context) ([]domain.StoragePool, error) {
	result, err := sqlc.New(q.pool).ListStoragePools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.StoragePool, len(result))
	for i := range result {
		out[i] = domain.StoragePool(result[i])
	}

	return out, nil
}

// ListRecordingFailureCounts は、理由ごとの、録画の失敗の累計を返す。
func (q *Queries) ListRecordingFailureCounts(ctx context.Context) ([]domain.RecordingFailureCount, error) {
	result, err := sqlc.New(q.pool).ListRecordingFailureCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.RecordingFailureCount, len(result))
	for i := range result {
		out[i] = domain.RecordingFailureCount(result[i])
	}

	return out, nil
}

// ListInstances は、動いているロールのインスタンスを返す。
func (q *Queries) ListInstances(ctx context.Context) ([]domain.Instance, error) {
	result, err := sqlc.New(q.pool).ListInstances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Instance, len(result))
	for i := range result {
		out[i] = domain.Instance(result[i])
	}

	return out, nil
}

// ListLeases は、リースを返す。
func (q *Queries) ListLeases(ctx context.Context) ([]domain.Lease, error) {
	result, err := sqlc.New(q.pool).ListLeases(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Lease, len(result))
	for i := range result {
		out[i] = domain.Lease(result[i])
	}

	return out, nil
}

// ListTuners は、チューナーを返す。
func (q *Queries) ListTuners(ctx context.Context) ([]domain.Tuner, error) {
	result, err := sqlc.New(q.pool).ListTuners(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Tuner, len(result))
	for i := range result {
		out[i] = domain.Tuner(result[i])
	}

	return out, nil
}
