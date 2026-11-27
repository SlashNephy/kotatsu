package aribcaption

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"uuid"

	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
)

// RecordingsCues は、録画 recordingIDs の ARIB 字幕の文を、録画ごとに始まりの順に返す。取り出していない録画は含めない。
// 字幕の文で探すとき (設計書 3.5、#2217) に、候補の録画の字幕をまとめて読む。時刻は録画を始めた日時からの時間。
func RecordingsCues(ctx context.Context, db sqlc.DBTX, recordingIDs []uuid.UUID) (map[uuid.UUID][]Cue, error) {
	rows, err := sqlc.New(db).ListRecordingARIBCaptionCues(ctx, recordingIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list the ARIB captions of the recordings: %w", err)
	}
	out := make(map[uuid.UUID][]Cue, len(rows))
	for _, row := range rows {
		var stored []storedCue
		if err := json.Unmarshal(row.Cues, &stored); err != nil {
			return nil, fmt.Errorf("failed to decode the ARIB captions of recording %s: %w", row.RecordingID, err)
		}
		cues := make([]Cue, 0, len(stored))
		for _, c := range stored {
			cues = append(cues, Cue{Start: time.Duration(c.StartMs) * time.Millisecond, End: time.Duration(c.EndMs) * time.Millisecond, Text: c.Text})
		}
		out[row.RecordingID] = cues
	}

	return out, nil
}
