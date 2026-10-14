package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/kago"
)

// MissingRecording は、セグメントのファイルが見つからない録画 (設計書 5.6)。
type MissingRecording struct {
	RecordingID uuid.UUID
	// Segments は、録画のセグメントの数。MissingSegments は、そのうち見つからないものの数。
	Segments        int
	MissingSegments int
}

// AllMissing は、セグメントのファイルが 1 つも見つからないかどうか。
func (m *MissingRecording) AllMissing() bool {
	return m.MissingSegments == m.Segments
}

// FindMissing は、録画中でない録画のうち、セグメントのファイルが見つからないものを、録画を始めた日時の新しい順に返す。
// このインスタンスにマウントしていないプールのセグメントは、見つからないとはみなさない。
func (l *Library) FindMissing(ctx context.Context, db sqlc.DBTX) ([]*MissingRecording, error) {
	rows, err := sqlc.New(db).ListFinishedRecordingSegments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording segments: %w", err)
	}
	result := []*MissingRecording{}
	var current *MissingRecording
	for _, row := range rows {
		if current == nil || current.RecordingID != row.RecordingID {
			if current != nil && current.MissingSegments > 0 {
				result = append(result, current)
			}
			current = &MissingRecording{RecordingID: row.RecordingID, Segments: 0, MissingSegments: 0}
		}
		current.Segments++
		missing, err := l.missing(row.PoolID, row.Path)
		if err != nil {
			return nil, err
		}
		if missing {
			current.MissingSegments++
		}
	}
	if current != nil && current.MissingSegments > 0 {
		result = append(result, current)
	}

	return result, nil
}

// missing は、セグメントのファイルが見つからないかを返す。マウントしていないプールのものは、見つからないとはみなさない。
func (l *Library) missing(poolID, path string) (bool, error) {
	exists, err := l.store.Exists(poolID, path)
	if errors.Is(err, kago.ErrPoolNotMounted) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return !exists, nil
}

// CleanUpResult は、CleanUpMissing の結果。
type CleanUpResult struct {
	// Deleted は、セグメントのファイルが 1 つも見つからず、DB から消したかどうか。
	Deleted bool
	// PrunedSegments は、一部のセグメントだけが見つからない録画で、記録を消したセグメントの数。
	PrunedSegments int
}

// CleanUpMissing は、録画のセグメントのファイルを確かめ直し、見つからないものの記録を DB から消す。ファイルは触らない。
// セグメントが 1 つも見つからなければ録画を消し、一部だけなら、録画は残して、見つからないセグメントと、その索引の記録だけを消す。
// 録画中と保護した録画は消さない。監査ログに残す。
func (l *Library) CleanUpMissing(ctx context.Context, db Database, actor *audit.Actor, recordingID uuid.UUID) (*CleanUpResult, error) {
	result := &CleanUpResult{Deleted: false, PrunedSegments: 0}
	err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		q := sqlc.New(tx)
		recording, err := lock(ctx, q, recordingID)
		if err != nil {
			return err
		}
		switch {
		case recording.Protected:
			return ErrProtected
		case recording.State == "recording":
			return ErrRecording
		}
		files, err := q.ListRecordingFiles(ctx, recordingID)
		if err != nil {
			return fmt.Errorf("failed to list the recording files: %w", err)
		}
		segments, missingSegments, missing, err := l.missingFiles(files)
		if err != nil {
			return err
		}
		switch missingSegments {
		case 0:
			return nil
		case segments:
			if _, err := q.DeleteRecording(ctx, recordingID); err != nil {
				return fmt.Errorf("failed to delete the recording: %w", err)
			}
			result.Deleted = true

			return l.record(ctx, tx, actor, "recording.deleted", &recording, map[string]any{"reason": "missing_files"})
		default:
			if err := q.DeleteRecordingFiles(ctx, sqlc.DeleteRecordingFilesParams{RecordingID: recordingID, Ids: missing}); err != nil {
				return fmt.Errorf("failed to delete the missing recording files: %w", err)
			}
			result.PrunedSegments = missingSegments

			return l.record(ctx, tx, actor, "recording.missing_segments_removed", &recording, map[string]any{"segments": result.PrunedSegments})
		}
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// missingFiles は、セグメントの数と、見つからないセグメントの数と、見つからないセグメントとその索引の記録の ID を返す。
func (l *Library) missingFiles(files []sqlc.RecordingFile) (int, int, []uuid.UUID, error) {
	segments, missingSegments := 0, 0
	missing := []uuid.UUID{}
	for i := range files {
		if files[i].Kind != segmentKind {
			continue
		}
		segments++
		gone, err := l.missing(files[i].PoolID, files[i].Path)
		if err != nil {
			return 0, 0, nil, err
		}
		if !gone {
			continue
		}
		missingSegments++
		missing = append(missing, files[i].ID)
		// 索引は、セグメントの付属ファイルのディレクトリにあるので、セグメントと一緒に記録を消す。
		for j := range files {
			if files[j].Kind == "index" && files[j].PoolID == files[i].PoolID && files[j].Path == kago.IndexPath(files[i].Path) {
				missing = append(missing, files[j].ID)
			}
		}
	}

	return segments, missingSegments, missing, nil
}
