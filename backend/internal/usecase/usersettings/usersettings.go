// Package usersettings は、ユーザーが自分の設定を変えたことを、同じユーザーのほかの端末に知らせる (設計書 9.1)。
//
// 設定を書くトランザクションの中で Touch を呼ぶと、そのユーザーの設定の版数を上げ、変更を通知する。
// tenban は、通知を受けて、購読中の端末のうち、版数が上がったユーザーの端末にだけ知らせる。
package usersettings

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/infra/notify"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// NotifyChannel は、いずれかのユーザーが設定を変えたときに、変更を通知するチャンネル (設計書 1.4)。
const NotifyChannel = "user_settings"

// Touch は、tx の中で、ユーザー userID の設定の版数を上げ、変更を通知する。
func Touch(ctx context.Context, tx pgx.Tx, clk clock.Clock, userID uuid.UUID) error {
	if _, err := sqlc.New(tx).BumpUserSettingsVersion(ctx, sqlc.BumpUserSettingsVersionParams{UserID: userID, Now: clk.Now()}); err != nil {
		return fmt.Errorf("failed to bump the user settings version: %w", err)
	}
	if _, err := notify.Publish(ctx, tx, clk, NotifyChannel); err != nil {
		return fmt.Errorf("failed to publish the user settings change: %w", err)
	}

	return nil
}
