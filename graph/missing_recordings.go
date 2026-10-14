package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/auth"
	"github.com/SlashNephy/kotatsu/internal/library"
)

// maxCleanUpRecordings は、一度に確かめ直して消す録画の数の上限。
const maxCleanUpRecordings = 500

// administrator は、管理者である操作者を返す。
func administrator(ctx context.Context) (*audit.Actor, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if !principal.Role.Can(auth.PermissionAdminister) {
		return nil, NewError(CodeForbidden, "only administrators can do this")
	}

	return actorOf(ctx, principal), nil
}

// missingRecordings は、セグメントのファイルが見つからない録画を返す。
func (r *Resolver) missingRecordings(ctx context.Context) ([]*model.MissingRecording, error) {
	if err := administer(ctx); err != nil {
		return nil, err
	}
	missing, err := r.library.FindMissing(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]*model.MissingRecording, 0, len(missing))
	for _, m := range missing {
		recording, err := r.recording(ctx, m.RecordingID)
		if err != nil {
			return nil, err
		}
		if recording == nil {
			continue
		}
		result = append(result, &model.MissingRecording{Recording: recording, Segments: m.Segments, MissingSegments: m.MissingSegments})
	}

	return result, nil
}

// cleanUpMissingRecordings は、録画 ids のセグメントのファイルを確かめ直し、見つからないものの記録を DB から消す。
func (r *Resolver) cleanUpMissingRecordings(ctx context.Context, ids []string) (*model.CleanUpMissingRecordingsPayload, error) {
	actor, err := administrator(ctx)
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
