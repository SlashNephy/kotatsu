package settings

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// serviceAuditTarget は、サービスの監査ログの対象の種類。
const serviceAuditTarget = "service"

// ErrServiceNotFound は、サービスが見つからないことを表す。
var ErrServiceNotFound = errors.New("service not found")

// ServiceChanges は、サービスの表示の変える項目。nil の項目は変えない。
type ServiceChanges struct {
	// Hidden は、番組表に出さない印。
	Hidden *bool
	// SortOrder は、並び順の上書き。ResetSortOrder を立てると、上書きを消して既定の並び (リモコンのキーの順) に戻す。
	SortOrder      *int
	ResetSortOrder bool
}

// UpdateService は、サービス serviceID の、番組表に出さない印と並び順の上書きを変える (設計書 8.2、10.3)。
func (s *Store) UpdateService(ctx context.Context, db Database, actor *audit.Actor, serviceID int64, changes *ServiceChanges) error {
	if changes == nil {
		return fmt.Errorf("%w: changes is nil", ErrInvalidSettings)
	}
	if changes.ResetSortOrder && changes.SortOrder != nil {
		return fmt.Errorf("%w: sortOrder and resetSortOrder cannot be used together", ErrInvalidSettings)
	}
	var order *int32
	if changes.SortOrder != nil {
		stored, err := sortOrder(*changes.SortOrder)
		if err != nil {
			return err
		}
		order = &stored
	}

	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		current, err := q.LockService(ctx, serviceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrServiceNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to lock the service: %w", err)
		}
		params := sqlc.UpdateServiceDisplayParams{Hidden: current.Hidden, SortOrder: current.SortOrder, Now: s.clock.Now(), ID: serviceID}
		if changes.Hidden != nil {
			params.Hidden = *changes.Hidden
		}
		switch {
		case changes.ResetSortOrder:
			params.SortOrder = nil
		case order != nil:
			params.SortOrder = order
		}
		if err := q.UpdateServiceDisplay(ctx, params); err != nil {
			return fmt.Errorf("failed to update the service: %w", err)
		}

		return s.recorder.Record(ctx, tx, &audit.Event{
			Actor:      actor,
			Action:     "service.updated",
			TargetType: serviceAuditTarget,
			TargetID:   strconv.FormatInt(serviceID, 10),
			Summary: map[string]any{
				"hidden":    change(current.Hidden, params.Hidden),
				"sortOrder": change(current.SortOrder, params.SortOrder),
			},
			// 送信元は、操作者 (actor) に持たせる。
			SourceIP:  netip.Addr{},
			UserAgent: "",
		})
	})
}
