// Package registry は、各ロールのインスタンスの登録簿を扱う (設計書 1.4)。
//
// 各インスタンスは、自分のアドレスとハートビートを DB に書く。tenban は、この登録簿を引いて
// kyusu のセッションへリクエストを中継する。
package registry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

const (
	// HeartbeatInterval は、ハートビートを書く間隔。
	HeartbeatInterval = 5 * time.Second
	// AliveTTL は、最後のハートビートからこの時間が過ぎたインスタンスを、止まったものとみなす時間。
	// ハートビートを 2 回取りこぼしても生きているとみなせるよう、間隔の 3 倍にする。
	AliveTTL = 3 * HeartbeatInterval
	// staleTTL は、止まったインスタンスの行を消すまでの時間。止まった直後の行は、調査のために残しておく。
	staleTTL = time.Hour
	// pruneEvery は、止まったインスタンスの行を消す頻度 (ハートビート何回に 1 回か)。
	// 消すのは staleTTL を過ぎた行だけなので、ハートビートのたびに消す必要はない。
	pruneEvery = 60
)

// Instance は、登録簿に載っているインスタンス。
type Instance struct {
	ID          uuid.UUID
	Role        string
	Address     string
	StartedAt   time.Time
	HeartbeatAt time.Time
}

// Registry は、このプロセスのインスタンスを登録簿に載せ、他のインスタンスを引く。
type Registry struct {
	db    sqlc.DBTX
	clock clock.Clock
	id    uuid.UUID
}

// New は、id のインスタンスとして登録簿を扱う Registry を作る。
func New(db sqlc.DBTX, clk clock.Clock, id uuid.UUID) *Registry {
	return &Registry{db: db, clock: clk, id: id}
}

// Register は、role のインスタンスとして address を登録簿に載せる。載っていればハートビートを書く。
func (r *Registry) Register(ctx context.Context, role, address string) error {
	if err := sqlc.New(r.db).UpsertInstance(ctx, sqlc.UpsertInstanceParams{
		ID:      r.id,
		Role:    role,
		Address: address,
		Now:     r.clock.Now(),
	}); err != nil {
		return fmt.Errorf("failed to register the %s instance: %w", role, err)
	}

	return nil
}

// Heartbeat は、role のインスタンスのハートビートを書く。
// 行が消されていた (止まったとみなされた) ときは、登録し直す。
func (r *Registry) Heartbeat(ctx context.Context, role, address string) error {
	updated, err := sqlc.New(r.db).HeartbeatInstance(ctx, sqlc.HeartbeatInstanceParams{
		Now:  r.clock.Now(),
		ID:   r.id,
		Role: role,
	})
	if err != nil {
		return fmt.Errorf("failed to write the heartbeat of the %s instance: %w", role, err)
	}
	if updated == 0 {
		return r.Register(ctx, role, address)
	}

	return nil
}

// Deregister は、role のインスタンスを登録簿から外す。
func (r *Registry) Deregister(ctx context.Context, role string) error {
	if err := sqlc.New(r.db).DeleteInstance(ctx, sqlc.DeleteInstanceParams{ID: r.id, Role: role}); err != nil {
		return fmt.Errorf("failed to deregister the %s instance: %w", role, err)
	}

	return nil
}

// ListAlive は、role のインスタンスのうち、AliveTTL 以内にハートビートを書いたものを、起動の早い順に返す。
func (r *Registry) ListAlive(ctx context.Context, role string) ([]*Instance, error) {
	rows, err := sqlc.New(r.db).ListAliveInstances(ctx, sqlc.ListAliveInstancesParams{
		Role:       role,
		AliveSince: r.clock.Now().Add(-AliveTTL),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the %s instances: %w", role, err)
	}

	instances := make([]*Instance, 0, len(rows))
	for _, row := range rows {
		instances = append(instances, &Instance{
			ID:          row.ID,
			Role:        row.Role,
			Address:     row.Address,
			StartedAt:   row.StartedAt,
			HeartbeatAt: row.HeartbeatAt,
		})
	}

	return instances, nil
}

// PruneStale は、長い間ハートビートのないインスタンスの行を消し、消した行の数を返す。
func (r *Registry) PruneStale(ctx context.Context) (int64, error) {
	deleted, err := sqlc.New(r.db).DeleteStaleInstances(ctx, r.clock.Now().Add(-staleTTL))
	if err != nil {
		return 0, fmt.Errorf("failed to prune stale instances: %w", err)
	}

	return deleted, nil
}

// Run は、roles のインスタンスとして address を登録し、ctx が終わるまでハートビートを書き続ける。
// 止まるときは登録簿から外す。ハートビートを書けなかったときは、ログに出して書き続ける。
func (r *Registry) Run(ctx context.Context, logger *slog.Logger, roles []string, address string) error {
	for _, role := range roles {
		if err := r.Register(ctx, role, address); err != nil {
			return err
		}
	}
	logger.InfoContext(ctx, "registered the instance", slog.String("instance_id", r.id.String()), slog.Any("roles", roles), slog.String("address", address))

	defer func() {
		// 終了のシグナルで ctx は終わっているので、登録簿から外すための context を別に作る。
		deregisterCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), HeartbeatInterval)
		defer cancel()
		for _, role := range roles {
			if err := r.Deregister(deregisterCtx, role); err != nil {
				logger.WarnContext(deregisterCtx, "failed to deregister the instance", slog.Any("error", err))
			}
		}
	}()

	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	for beats := 1; ; beats++ {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		errs := []error{}
		for _, role := range roles {
			if err := r.Heartbeat(ctx, role, address); err != nil {
				errs = append(errs, err)
			}
		}
		if beats%pruneEvery == 0 {
			if _, err := r.PruneStale(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if err := errors.Join(errs...); err != nil && ctx.Err() == nil {
			logger.WarnContext(ctx, "failed to update the instance registry", slog.Any("error", err))
		}
	}
}
