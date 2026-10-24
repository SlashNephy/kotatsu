package query

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// WatchReservedPrograms は、keys のうち、ユーザー userID が視聴予約した番組を返す。
func (q *Queries) WatchReservedPrograms(ctx context.Context, userID uuid.UUID, keys []domain.ProgramKey) (map[domain.ProgramKey]bool, error) {
	serviceIDs := make([]int64, 0, len(keys))
	eventIDs := make([]int64, 0, len(keys))
	for _, key := range keys {
		serviceIDs = append(serviceIDs, key.ServiceID)
		eventIDs = append(eventIDs, key.EventID)
	}
	rows, err := sqlc.New(q.pool).ListWatchReservedPrograms(ctx, sqlc.ListWatchReservedProgramsParams{
		UserID: userID, ServiceIds: serviceIDs, EventIds: eventIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list the watch reservations: %w", err)
	}
	reserved := make(map[domain.ProgramKey]bool, len(rows))
	for _, row := range rows {
		reserved[domain.ProgramKey{ServiceID: row.ServiceID, EventID: row.EventID}] = true
	}

	return reserved, nil
}

// WatchReminderMinutes は、ユーザー userID の、視聴予約の通知を放送の何分前に送るかの設定を返す。
func (q *Queries) WatchReminderMinutes(ctx context.Context, userID uuid.UUID) (int, error) {
	minutes, err := sqlc.New(q.pool).GetWatchReminderMinutes(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get the watch reminder minutes: %w", err)
	}

	return int(minutes), nil
}

// UserSettingsVersion は、ユーザー userID の設定の版数を返す。一度も変えていなければ 0 を返す。
func (q *Queries) UserSettingsVersion(ctx context.Context, userID uuid.UUID) (int64, error) {
	version, err := sqlc.New(q.pool).GetUserSettingsVersion(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get the user settings version: %w", err)
	}

	return version, nil
}
