package dramalibrary

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/usecase/usersettings"
)

// DefaultThresholdPercent は、番組の長さのうち、見たら見た話にする割合の既定値 (付録 A)。見たアニメを記録する割合の既定値と同じ。
const DefaultThresholdPercent = 20

// ErrInvalidSettings は、見たドラマの記録の設定の値が正しくないことを表す。
var ErrInvalidSettings = errors.New("invalid drama record settings")

// ThresholdPercent は、ユーザー userID の見た話にする割合を返す (#1649)。見たアニメを記録する割合 (設計書 3.8) とは別に持つ。
// 変えていなければ、既定の 20% を返す。
func ThresholdPercent(ctx context.Context, db sqlc.DBTX, userID uuid.UUID) (int, error) {
	row, err := sqlc.New(db).GetDramaRecordSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultThresholdPercent, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get the drama record settings: %w", err)
	}

	return int(row.ThresholdPercent), nil
}

// SaveThresholdPercent は、ユーザー userID の見た話にする割合を percent (1〜100) に変え、変えた後の値を返す。
// ほかの端末の設定の画面が読み直すよう、ユーザーの設定が変わったことを通知する。
func SaveThresholdPercent(ctx context.Context, db Database, clk clock.Clock, userID uuid.UUID, percent int) (int, error) {
	if percent < 1 || percent > 100 {
		return 0, fmt.Errorf("%w: the threshold must be between 1 and 100", ErrInvalidSettings)
	}
	threshold := int16(percent)
	var saved int
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).UpsertDramaRecordSettings(ctx, sqlc.UpsertDramaRecordSettingsParams{
			UserID: userID, ThresholdPercent: threshold, Now: clk.Now(),
		})
		if err != nil {
			return fmt.Errorf("failed to save the drama record settings: %w", err)
		}
		saved = int(row.ThresholdPercent)

		return usersettings.Touch(ctx, tx, clk, userID)
	})
	if err != nil {
		return 0, err
	}

	return saved, nil
}
