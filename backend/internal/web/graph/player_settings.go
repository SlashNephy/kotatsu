package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/usersettings"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// コメントの設定の範囲 (#488)。
const (
	minCommentOpacity     = 0.1
	maxCommentOpacity     = 1.0
	minCommentSpeed       = 0.5
	maxCommentSpeed       = 2.0
	minCommentScale       = 0.5
	maxCommentScale       = 1.5
	maxMutedKeywords      = 100
	maxMutedKeywordLength = 100
	maxMutedUsers         = 500
	maxMutedUserLength    = 64
)

// defaultCaptionSettings は、字幕の設定の既定値 (設計書 付録 A)。文字スーパーは、速報を見落とさないよう、ライブでも録画でも出す。
func defaultCaptionSettings() *model.CaptionSettings {
	return &model.CaptionSettings{SuperimposeLive: true, SuperimposeRecorded: true, Stroke: false, BackgroundOpacity: nil}
}

// defaultCommentSettings は、コメントの設定の既定値 (設計書 付録 A)。
func defaultCommentSettings() *model.CommentSettings {
	return &model.CommentSettings{Opacity: 1, Speed: 1, Scale: 1, ShowAll: true, MutedKeywords: []*model.MutedKeyword{}, MutedUsers: []string{}}
}

// playerSettings は、ログイン中のユーザーのプレイヤーの設定を返す。
func (r *Resolver) playerSettings(ctx context.Context) (*model.PlayerSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	row, err := sqlc.New(r.db).GetPlayerSettings(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	comments, err := decodeCommentSettings(row.CommentSettings)
	if err != nil {
		return nil, err
	}
	captions, err := decodeCaptionSettings(row.CaptionSettings)
	if err != nil {
		return nil, err
	}

	return &model.PlayerSettings{JikkyoEnabled: row.JikkyoEnabled, CaptionsEnabled: row.CaptionsEnabled, Comments: comments, Captions: captions}, nil
}

// updatePlayerSettings は、プレイヤーの設定を変え、同じユーザーのほかの端末に知らせる。
func (r *Resolver) updatePlayerSettings(ctx context.Context, input *model.PlayerSettingsInput) (*model.PlayerSettings, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	var encoded, encodedCaptions []byte
	if input.Comments != nil {
		if encoded, err = encodeCommentSettings(input.Comments); err != nil {
			return nil, err
		}
	}
	if input.Captions != nil {
		if encodedCaptions, err = encodeCaptionSettings(input.Captions); err != nil {
			return nil, err
		}
	}
	var settings *model.PlayerSettings
	err = pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		if encoded != nil {
			err := q.SetCommentSettings(ctx, sqlc.SetCommentSettingsParams{UserID: principal.UserID, CommentSettings: encoded, Now: r.clock.Now()})
			if err != nil {
				return fmt.Errorf("failed to save the comment settings: %w", err)
			}
		}
		if encodedCaptions != nil {
			err := q.SetCaptionSettings(ctx, sqlc.SetCaptionSettingsParams{UserID: principal.UserID, CaptionSettings: encodedCaptions, Now: r.clock.Now()})
			if err != nil {
				return fmt.Errorf("failed to save the caption settings: %w", err)
			}
		}
		row, err := q.UpsertPlayerSettings(ctx, sqlc.UpsertPlayerSettingsParams{
			UserID: principal.UserID, JikkyoEnabled: optionalBool(input.JikkyoEnabled), CaptionsEnabled: optionalBool(input.CaptionsEnabled),
			Now: r.clock.Now(),
		})
		if err != nil {
			return err
		}
		comments, err := decodeCommentSettings(row.CommentSettings)
		if err != nil {
			return err
		}
		captions, err := decodeCaptionSettings(row.CaptionSettings)
		if err != nil {
			return err
		}
		settings = &model.PlayerSettings{JikkyoEnabled: row.JikkyoEnabled, CaptionsEnabled: row.CaptionsEnabled, Comments: comments, Captions: captions}

		return usersettings.Touch(ctx, tx, r.clock, principal.UserID)
	})
	if err != nil {
		return nil, err
	}

	return settings, nil
}

// encodeCommentSettings は、コメントの設定を確かめて、DB に書く JSON にする。
// 正規表現は、画面 (JavaScript の RegExp) で使うので、ここでは長さだけを確かめる。
func encodeCommentSettings(input *model.CommentSettingsInput) ([]byte, error) {
	switch {
	case input.Opacity < minCommentOpacity || input.Opacity > maxCommentOpacity:
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("opacity must be between %g and %g", minCommentOpacity, maxCommentOpacity))
	case input.Speed < minCommentSpeed || input.Speed > maxCommentSpeed:
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("speed must be between %g and %g", minCommentSpeed, maxCommentSpeed))
	case input.Scale < minCommentScale || input.Scale > maxCommentScale:
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("scale must be between %g and %g", minCommentScale, maxCommentScale))
	case len(input.MutedKeywords) > maxMutedKeywords:
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("mutedKeywords must be at most %d", maxMutedKeywords))
	case len(input.MutedUsers) > maxMutedUsers:
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("mutedUsers must be at most %d", maxMutedUsers))
	}
	settings := &model.CommentSettings{
		Opacity: input.Opacity, Speed: input.Speed, Scale: input.Scale, ShowAll: input.ShowAll,
		MutedKeywords: make([]*model.MutedKeyword, 0, len(input.MutedKeywords)), MutedUsers: make([]string, 0, len(input.MutedUsers)),
	}
	for _, keyword := range input.MutedKeywords {
		if keyword.Text == "" || utf8.RuneCountInString(keyword.Text) > maxMutedKeywordLength || !keyword.Match.IsValid() {
			return nil, NewError(CodeBadUserInput, fmt.Sprintf("each muted keyword must be 1 to %d characters", maxMutedKeywordLength))
		}
		settings.MutedKeywords = append(settings.MutedKeywords, &model.MutedKeyword{Text: keyword.Text, Match: keyword.Match})
	}
	for _, user := range input.MutedUsers {
		if user == "" || len(user) > maxMutedUserLength {
			return nil, NewError(CodeBadUserInput, fmt.Sprintf("each muted user must be 1 to %d bytes", maxMutedUserLength))
		}
		settings.MutedUsers = append(settings.MutedUsers, user)
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the comment settings: %w", err)
	}

	return encoded, nil
}

// decodeCommentSettings は、DB の JSON をコメントの設定にする。NULL なら既定値を返す。
func decodeCommentSettings(raw []byte) (*model.CommentSettings, error) {
	settings := defaultCommentSettings()
	if raw == nil {
		return settings, nil
	}
	if err := json.Unmarshal(raw, settings); err != nil {
		return nil, fmt.Errorf("failed to decode the comment settings: %w", err)
	}

	return settings, nil
}

// encodeCaptionSettings は、字幕の設定を確かめて、DB に書く JSON にする (#491)。
func encodeCaptionSettings(input *model.CaptionSettingsInput) ([]byte, error) {
	if opacity := input.BackgroundOpacity; opacity != nil && (*opacity < 0 || *opacity > 1) {
		return nil, NewError(CodeBadUserInput, "backgroundOpacity must be between 0 and 1")
	}
	encoded, err := json.Marshal(&model.CaptionSettings{
		SuperimposeLive: input.SuperimposeLive, SuperimposeRecorded: input.SuperimposeRecorded,
		Stroke: input.Stroke, BackgroundOpacity: input.BackgroundOpacity,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode the caption settings: %w", err)
	}

	return encoded, nil
}

// decodeCaptionSettings は、DB の JSON を字幕の設定にする。NULL なら既定値を返す。
func decodeCaptionSettings(raw []byte) (*model.CaptionSettings, error) {
	settings := defaultCaptionSettings()
	if raw == nil {
		return settings, nil
	}
	if err := json.Unmarshal(raw, settings); err != nil {
		return nil, fmt.Errorf("failed to decode the caption settings: %w", err)
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
