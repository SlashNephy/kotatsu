// Package videoformat は、録画のセグメントの映像から読んだ映像の形式を記録する (#1310)。
package videoformat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

// Save は、セグメント fileID (始まりが startedAt) の映像の形式 format を記録し、録画の映像の解像度を、映像から読んだ値にする。
// 録画の解像度は、EPG から写した値より、映像から読んだ値を優先する。録画の最初の、形式を読めたセグメントの値を使う。
// format が nil (読めなかった) なら、何もしない。
func Save(ctx context.Context, q *sqlc.Queries, recordingID, fileID uuid.UUID, startedAt time.Time, format *ts.VideoFormat) error {
	if format == nil {
		return nil
	}
	encoded, err := json.Marshal(format)
	if err != nil {
		return fmt.Errorf("failed to encode the video format: %w", err)
	}
	if err := q.SetRecordingFileVideo(ctx, sqlc.SetRecordingFileVideoParams{Video: encoded, ID: fileID}); err != nil {
		return fmt.Errorf("failed to save the video format: %w", err)
	}
	resolution := format.Resolution()
	err = q.SetRecordingVideoResolutionFromFile(ctx, sqlc.SetRecordingVideoResolutionFromFileParams{
		VideoResolution: &resolution, RecordingID: recordingID, StartedAt: startedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to save the video resolution: %w", err)
	}

	return nil
}
