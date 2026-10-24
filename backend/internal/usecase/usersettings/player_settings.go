package usersettings

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/internal/domain"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// PlayerSettingsChange は、プレイヤーの設定の変更。nil の項目は変えない。
// CommentSettings と CaptionSettings は、コメントと字幕の設定を JSON にしたもの。
type PlayerSettingsChange struct {
	JikkyoEnabled   *bool
	CaptionsEnabled *bool
	CommentSettings []byte
	CaptionSettings []byte
}

// UpdatePlayerSettings は、ユーザー userID のプレイヤーの設定を変え、同じユーザーのほかの端末に知らせる。変えた後の設定を返す。
func UpdatePlayerSettings(ctx context.Context, db Beginner, clk clock.Clock, userID uuid.UUID, change *PlayerSettingsChange) (*domain.PlayerSettings, error) {
	var settings *domain.PlayerSettings
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if change.CommentSettings != nil {
			err := q.SetCommentSettings(ctx, sqlc.SetCommentSettingsParams{UserID: userID, CommentSettings: change.CommentSettings, Now: clk.Now()})
			if err != nil {
				return fmt.Errorf("failed to save the comment settings: %w", err)
			}
		}
		if change.CaptionSettings != nil {
			err := q.SetCaptionSettings(ctx, sqlc.SetCaptionSettingsParams{UserID: userID, CaptionSettings: change.CaptionSettings, Now: clk.Now()})
			if err != nil {
				return fmt.Errorf("failed to save the caption settings: %w", err)
			}
		}
		row, err := q.UpsertPlayerSettings(ctx, sqlc.UpsertPlayerSettingsParams{
			UserID: userID, JikkyoEnabled: optionalBool(change.JikkyoEnabled), CaptionsEnabled: optionalBool(change.CaptionsEnabled),
			Now: clk.Now(),
		})
		if err != nil {
			return err
		}
		settings = new(domain.PlayerSettings(row))

		return Touch(ctx, tx, clk, userID)
	})
	if err != nil {
		return nil, err
	}

	return settings, nil
}

// optionalBool は、省いた項目 (nil) を、DB の NULL にする。
func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{Bool: false, Valid: false}
	}

	return pgtype.Bool{Bool: *v, Valid: true}
}
