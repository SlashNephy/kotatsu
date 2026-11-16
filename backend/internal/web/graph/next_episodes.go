package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/nextepisode"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// nextEpisodes は、録画のシリーズの続きの放送を、番組表と同じ形の番組にして、放送の早い順に返す (#1564)。
// 探してから引くまでの間に番組表から消えた番組は、飛ばす。
func (r *Resolver) nextEpisodes(ctx context.Context, obj *model.Recording) ([]*model.Program, error) {
	recordingID, err := parseRecordingID(obj.ID)
	if err != nil {
		return nil, err
	}
	broadcasts, err := nextepisode.Find(ctx, r.db, recordingID, r.clock.Now())
	if err != nil {
		return nil, err
	}
	serviceIDs, eventIDs := make([]int64, 0, len(broadcasts)), make([]int64, 0, len(broadcasts))
	for _, b := range broadcasts {
		serviceIDs, eventIDs = append(serviceIDs, b.ServiceID), append(eventIDs, b.EventID)
	}
	programs, err := r.programsByKeys(ctx, serviceIDs, eventIDs)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Program, 0, len(broadcasts))
	for _, b := range broadcasts {
		if program, ok := programs[programKeyOf(b.ServiceID, b.EventID)]; ok {
			result = append(result, program)
		}
	}

	return result, nil
}
