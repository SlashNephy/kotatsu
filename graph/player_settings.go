package graph

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
)

// playerSettings は、ログイン中のユーザーのプレイヤーの設定を返す (設計書 9.1)。
func (r *Resolver) playerSettings(ctx context.Context) (*model.PlayerSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	row, err := sqlc.New(r.db).GetPlayerSettings(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}

	return &model.PlayerSettings{JikkyoEnabled: row.JikkyoEnabled, CaptionsEnabled: row.CaptionsEnabled}, nil
}

// updatePlayerSettings は、プレイヤーの設定を変え、同じユーザーのほかの端末に知らせる。
func (r *Resolver) updatePlayerSettings(ctx context.Context, input *model.PlayerSettingsInput) (*model.PlayerSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	var settings *model.PlayerSettings
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		row, err := sqlc.New(tx).UpsertPlayerSettings(ctx, sqlc.UpsertPlayerSettingsParams{
			UserID: principal.UserID, JikkyoEnabled: optionalBool(input.JikkyoEnabled), CaptionsEnabled: optionalBool(input.CaptionsEnabled),
			Now: r.clock.Now(),
		})
		if err != nil {
			return err
		}
		settings = &model.PlayerSettings{JikkyoEnabled: row.JikkyoEnabled, CaptionsEnabled: row.CaptionsEnabled}

		return usersettings.Touch(ctx, tx, r.clock, principal.UserID)
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
