package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/library"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// maxCleanUpRecordings は、一度に確かめ直して消す録画の数の上限。
const maxCleanUpRecordings = 500

// missingRecordings は、セグメントのファイルが見つからない録画を返す。
func (r *Resolver) missingRecordings(ctx context.Context) ([]*model.MissingRecording, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	missing, err := r.library.FindMissing(ctx, r.db)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(missing))
	for _, m := range missing {
		ids = append(ids, m.RecordingID)
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
	result := make([]*model.MissingRecording, 0, len(missing))
	for _, m := range missing {
		// 探している間に消えた録画は飛ばす。
		if recording, ok := byID[GlobalID(recordingType, m.RecordingID.String())]; ok {
			result = append(result, &model.MissingRecording{Recording: recording, Segments: m.Segments, MissingSegments: m.MissingSegments})
		}
	}

	return result, nil
}

// cleanUpMissingRecordings は、録画 ids のセグメントのファイルを確かめ直し、見つからないものの記録を DB から消す。
// 録画を消す取り返しのつかない操作なので、直近 10 分以内の再認証を求める (設計書 9.5)。
func (r *Resolver) cleanUpMissingRecordings(ctx context.Context, ids []string) (*model.CleanUpMissingRecordingsPayload, error) {
	_, actor, err := r.recentAdministrator(ctx, "only administrators can clean up missing recordings")
	if err != nil {
		return nil, err
	}
	if len(ids) > maxCleanUpRecordings {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("ids must be at most %d", maxCleanUpRecordings))
	}
	payload := &model.CleanUpMissingRecordingsPayload{DeletedRecordings: 0, PrunedSegments: 0, Skipped: 0}
	for _, globalID := range ids {
		recordingID, err := parseRecordingID(globalID)
		if err != nil {
			return nil, err
		}
		result, err := r.library.CleanUpMissing(ctx, r.db, actor, recordingID)
		switch {
		case errors.Is(err, library.ErrProtected), errors.Is(err, library.ErrRecording), errors.Is(err, library.ErrNotFound):
			payload.Skipped++

			continue
		case err != nil:
			return nil, err
		}
		if result.Deleted {
			payload.DeletedRecordings++
		}
		payload.PrunedSegments += result.PrunedSegments
	}

	return payload, nil
}
