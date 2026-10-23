package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/library"
)

// maxRenameRecordings は、一度に付け直す録画の数の上限。
const maxRenameRecordings = 100

// recordingRenames は、今のテンプレートで付け直すと名前が変わる録画を返す。
func (r *Resolver) recordingRenames(ctx context.Context) ([]*model.RecordingRename, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	plans, err := r.library.PlanRenames(ctx, r.db, nil, r.location)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(plans))
	for _, plan := range plans {
		ids = append(ids, plan.RecordingID)
	}
	rows, err := sqlc.New(r.db).ListRecordingsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get the recordings: %w", err)
	}
	recordings, err := r.recordingModels(ctx, rows)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*model.Recording, len(recordings))
	for _, recording := range recordings {
		byID[recording.ID] = recording
	}
	result := make([]*model.RecordingRename, 0, len(plans))
	for _, plan := range plans {
		recording, ok := byID[GlobalID(recordingType, plan.RecordingID.String())]
		if !ok {
			continue
		}
		files := make([]*model.RecordingFileRename, 0, len(plan.Files))
		for _, file := range plan.Files {
			files = append(files, &model.RecordingFileRename{From: file.From, To: file.To})
		}
		result = append(result, &model.RecordingRename{Recording: recording, Files: files})
	}

	return result, nil
}

// renameRecordingFiles は、録画 ids のファイルを、今のテンプレートの名前に付け直す。
// ファイルを動かす操作なので、直近 10 分以内の再認証を求める (設計書 9.5)。
func (r *Resolver) renameRecordingFiles(ctx context.Context, globalIDs []string) (*model.RenameRecordingFilesPayload, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can rename recording files")
	if err != nil {
		return nil, err
	}
	if len(globalIDs) > maxRenameRecordings {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("ids must be at most %d", maxRenameRecordings))
	}
	ids := make([]uuid.UUID, 0, len(globalIDs))
	for _, globalID := range globalIDs {
		recordingID, err := parseRecordingID(globalID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, recordingID)
	}
	// 計画は、付け直す直前に、今のテンプレートとファイルで立て直す。
	plans, err := r.library.PlanRenames(ctx, r.db, ids, r.location)
	if err != nil {
		return nil, err
	}
	payload := &model.RenameRecordingFilesPayload{Renamed: 0, Skipped: len(ids) - len(plans), Failed: 0}
	for _, plan := range plans {
		err := r.library.Rename(ctx, r.db, actor, plan)
		switch {
		case err == nil:
			payload.Renamed++
		case errors.Is(err, library.ErrRecording), errors.Is(err, library.ErrBusy), errors.Is(err, library.ErrNotFound):
			payload.Skipped++
		default:
			r.logger.WarnContext(ctx, "failed to rename recording files", "recording_id", plan.RecordingID.String(), "error", err)
			payload.Failed++
		}
	}

	return payload, nil
}
