package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// streamQuality は、録画のセグメントの PID ごとの品質の集計を、PID ごとに足し合わせて返す (#473)。
func (r *Resolver) streamQuality(ctx context.Context, obj *model.Recording) ([]*model.StreamQuality, error) {
	recordingID, err := parseRecordingID(obj.ID)
	if err != nil {
		return nil, err
	}
	files, err := sqlc.New(r.db).ListRecordingFiles(ctx, recordingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the recording files: %w", err)
	}
	byPID := map[int]*model.StreamQuality{}
	for i := range files {
		if files[i].PIDStats == nil {
			continue
		}
		var stats []ts.PIDQuality
		if err := json.Unmarshal(files[i].PIDStats, &stats); err != nil {
			return nil, fmt.Errorf("failed to decode the PID stats: %w", err)
		}
		for _, stat := range stats {
			kind := model.StreamKind(strings.ToUpper(stat.Kind))
			if !kind.IsValid() {
				kind = model.StreamKindUnknown
			}
			sum, ok := byPID[int(stat.PID)]
			if !ok {
				sum = &model.StreamQuality{Pid: int(stat.PID), Kind: kind, Packets: 0, Drops: 0, Errors: 0, Scrambled: 0}
				byPID[int(stat.PID)] = sum
			}
			// セグメントによって PMT を読めていないことがあるので、種類の分かったものを使う。
			if sum.Kind == model.StreamKindUnknown {
				sum.Kind = kind
			}
			sum.Packets += stat.Packets
			sum.Drops += stat.Drops
			sum.Errors += stat.Errors
			sum.Scrambled += stat.Scrambled
		}
	}
	result := make([]*model.StreamQuality, 0, len(byPID))
	for _, pid := range slices.Sorted(maps.Keys(byPID)) {
		result = append(result, byPID[pid])
	}

	return result, nil
}
