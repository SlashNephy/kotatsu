package graph

import (
	"errors"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/settings"
)

// systemSettingsOf は、全体の設定を GraphQL の形にする。
func systemSettingsOf(system *settings.System) *model.SystemSettings {
	recording := &model.RecordingSettings{
		PoolID:             nil,
		FileNameTemplate:   system.Recording.FileNameTemplate,
		StartMarginSeconds: system.Recording.StartMarginSeconds,
		EndMarginSeconds:   system.Recording.EndMarginSeconds,
		Priority:           system.Recording.Priority,
	}
	if system.Recording.PoolID != "" {
		recording.PoolID = new(system.Recording.PoolID)
	}

	return &model.SystemSettings{
		RecordingDefaults:    recording,
		ArchiveRetentionDays: system.ArchiveRetentionDays,
		UpdatedAt:            system.UpdatedAt,
	}
}

// settingsError は、設定のエラーを、エラーコードを持つエラーにする。
func settingsError(err error) error {
	if errors.Is(err, settings.ErrInvalidSettings) {
		return NewError(CodeBadUserInput, err.Error())
	}
	if errors.Is(err, settings.ErrChannelTypeNotFound) {
		return NewError(CodeNotFound, "channel type not found")
	}
	if errors.Is(err, settings.ErrServiceNotFound) {
		return NewError(CodeNotFound, "service not found")
	}

	return err
}
