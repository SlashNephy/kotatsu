package graph

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/registry"
)

const (
	// auditEventType は、AuditEvent の ID に付ける型の名前。
	auditEventType = "AuditEvent"
	// maxAuditEvents は、監査ログを 1 回に返す件数の上限。
	maxAuditEvents = 200
)

// auditCursor は、監査ログのページを送るカーソル。最後に返した記録の日時と ID。
type auditCursor struct {
	OccurredAt time.Time `json:"occurredAt"`
	ID         uuid.UUID `json:"id"`
}

// administer は、ログイン中のユーザーが管理者であることを確かめる。読むだけなので、再認証は求めない。
func administer(ctx context.Context) error {
	principal, err := viewer(ctx)
	if err != nil {
		return err
	}
	if !principal.Role.Can(auth.PermissionAdminister) {
		return NewError(CodeForbidden, "only administrators can see this")
	}

	return nil
}

// listAuditEvents は、監査ログを、新しい順に first 件まで返す。
func (r *Resolver) listAuditEvents(ctx context.Context, first int, after *string, filter *model.AuditEventFilter) (*model.AuditEventConnection, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	if first < 1 || first > maxAuditEvents {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("first must be between 1 and %d", maxAuditEvents))
	}
	// 次のページがあるかを知るため、1 件多く読む。絞り込まない項目は NULL のままにする。
	params := sqlc.SearchAuditEventsParams{
		ActionPrefix: nil, ActorUserID: nil, TargetType: nil, TargetID: nil, Since: nil, Until: nil, BeforeAt: nil, BeforeID: nil,
		MaxCount: int32(first + 1),
	}
	if filter != nil {
		params.ActionPrefix, params.TargetType, params.TargetID = filter.ActionPrefix, filter.TargetType, filter.TargetID
		params.Since, params.Until = filter.Since, filter.Until
		if filter.ActorUserID != nil {
			actorID, err := parseUserID(*filter.ActorUserID)
			if err != nil {
				return nil, err
			}
			params.ActorUserID = &actorID
		}
	}
	if after != nil {
		var cursor auditCursor
		if err := DecodeCursor(*after, &cursor); err != nil {
			return nil, NewError(CodeBadUserInput, "invalid cursor")
		}
		params.BeforeAt, params.BeforeID = &cursor.OccurredAt, &cursor.ID
	}

	rows, err := sqlc.New(r.db).SearchAuditEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to search audit events: %w", err)
	}
	hasNext := len(rows) > first
	rows = rows[:min(len(rows), first)]
	actorIDs := make([]*uuid.UUID, 0, len(rows))
	for i := range rows {
		actorIDs = append(actorIDs, rows[i].ActorUserID)
	}
	byID, err := r.auditActors(ctx, actorIDs)
	if err != nil {
		return nil, err
	}

	edges := make([]*model.AuditEventEdge, 0, len(rows))
	for i := range rows {
		cursor, err := EncodeCursor(auditCursor{OccurredAt: rows[i].OccurredAt, ID: rows[i].ID})
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.AuditEventEdge{Cursor: cursor, Node: auditEventOf(&rows[i], byID)})
	}
	info := &model.PageInfo{HasNextPage: hasNext, HasPreviousPage: after != nil, StartCursor: nil, EndCursor: nil}
	if len(edges) > 0 {
		info.StartCursor, info.EndCursor = &edges[0].Cursor, &edges[len(edges)-1].Cursor
	}

	return &model.AuditEventConnection{Edges: edges, PageInfo: info}, nil
}

// auditActors は、監査ログの操作者のユーザーを引く。監査ログは、ユーザーを消しても残すので (設計書 8.5)、
// 見つからないユーザーは表に入れず、操作者のユーザーを null にする。
func (r *Resolver) auditActors(ctx context.Context, ids []*uuid.UUID) (map[uuid.UUID]*model.User, error) {
	keys := make([]uuid.UUID, 0, len(ids))
	for _, userID := range ids {
		if userID != nil && !slices.Contains(keys, *userID) {
			keys = append(keys, *userID)
		}
	}
	byID := make(map[uuid.UUID]*model.User, len(keys))
	if len(keys) == 0 {
		return byID, nil
	}
	rows, err := sqlc.New(r.db).ListUsersByIDs(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	for i := range rows {
		byID[rows[i].ID] = toUser(&rows[i])
	}

	return byID, nil
}

func auditEventOf(row *sqlc.AuditEvent, byID map[uuid.UUID]*model.User) *model.AuditEvent {
	event := &model.AuditEvent{
		ID:         GlobalID(auditEventType, row.ID.String()),
		OccurredAt: row.OccurredAt,
		Actor:      &model.AuditActor{Type: row.ActorType, User: userOf(byID, row.ActorUserID), Role: row.ActorRole},
		Action:     row.Action,
		TargetType: row.TargetType,
		TargetID:   row.TargetID,
		Summary:    string(row.Summary),
		SourceIP:   nil,
		UserAgent:  row.UserAgent,
	}
	if row.SourceIP != nil {
		event.SourceIP = new(row.SourceIP.String())
	}

	return event
}

// systemStatus は、インスタンス、リース、チューナー、保存先プールの状態を返す。
func (r *Resolver) systemStatus(ctx context.Context) (*model.SystemStatus, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	q := sqlc.New(r.db)
	now := r.clock.Now()
	status := &model.SystemStatus{
		Instances: []*model.Instance{}, Leases: []*model.Lease{}, TunerServer: nil, Tuners: []*model.Tuner{}, Pools: []*model.StoragePool{},
	}

	instances, err := q.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances: %w", err)
	}
	for _, row := range instances {
		status.Instances = append(status.Instances, &model.Instance{
			InstanceID: row.ID.String(), Role: row.Role, Address: row.Address, StartedAt: row.StartedAt, HeartbeatAt: row.HeartbeatAt,
			Alive: row.HeartbeatAt.After(now.Add(-registry.AliveTTL)),
		})
	}
	leases, err := q.ListLeases(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list leases: %w", err)
	}
	for _, row := range leases {
		status.Leases = append(status.Leases, &model.Lease{
			Name: row.Name, HolderInstanceID: row.Holder.String(), Generation: int(row.Generation),
			AcquiredAt: row.AcquiredAt, ExpiresAt: row.ExpiresAt, Valid: row.ExpiresAt.After(now),
		})
	}

	server, err := q.GetTunerServerStatus(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("failed to get the tuner server status: %w", err)
	default:
		status.TunerServer = &model.TunerServerStatus{
			Degraded: server.DegradedSince != nil, DegradedSince: server.DegradedSince, LastError: server.LastError,
			ClockSkewMs: nil, CheckedAt: server.CheckedAt,
		}
		if server.ClockSkewMs != nil {
			status.TunerServer.ClockSkewMs = new(int(*server.ClockSkewMs))
		}
	}
	tuners, err := q.ListTuners(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tuners: %w", err)
	}
	for _, row := range tuners {
		status.Tuners = append(status.Tuners, &model.Tuner{
			Index: int(row.TunerIndex), Name: row.Name, Types: row.Types, Available: row.IsAvailable, Remote: row.IsRemote, Fault: row.IsFault,
		})
	}
	pools, err := r.storagePools(ctx)
	if err != nil {
		return nil, err
	}
	status.Pools = pools

	return status, nil
}

// storagePools は、このインスタンスにマウントされた保存先プールを、設定と空き容量と一緒に返す。
func (r *Resolver) storagePools(ctx context.Context) ([]*model.StoragePool, error) {
	rows, err := sqlc.New(r.db).ListStoragePools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list storage pools: %w", err)
	}
	settings := make(map[string]*sqlc.StoragePool, len(rows))
	for i := range rows {
		settings[rows[i].ID] = &rows[i]
	}
	pools := make([]*model.StoragePool, 0, len(r.pools))
	for _, id := range slices.Sorted(maps.Keys(r.pools)) {
		pools = append(pools, r.storagePoolOf(id, settings[id]))
	}

	return pools, nil
}

// storagePoolOf は、保存先プールを GraphQL の形にする。設定がなければ、優先順位 0、容量の閾値 0 とする。
func (r *Resolver) storagePoolOf(id string, setting *sqlc.StoragePool) *model.StoragePool {
	pool := &model.StoragePool{ID: id, Path: r.pools[id], Priority: 0, MinFreeBytes: 0, FreeBytes: nil}
	if setting != nil {
		pool.Priority, pool.MinFreeBytes = int(setting.Priority), setting.MinFreeBytes
	}
	if free, err := r.freeSpace(r.pools[id]); err == nil {
		pool.FreeBytes = new(int64(min(free, math.MaxInt64)))
	}

	return pool
}
