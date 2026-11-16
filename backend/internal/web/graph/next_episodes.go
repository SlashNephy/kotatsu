package graph

import (
	"context"
	"fmt"
	"uuid"

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

// viewerNextEpisodes は、ログイン中のユーザーが見たシリーズの続きの放送を、ホームの「続きの放送」の形で返す (#1564)。
// 探してから引くまでの間に消えた録画と番組は、飛ばす。
func (r *Resolver) viewerNextEpisodes(ctx context.Context) ([]*model.SeriesNextEpisode, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	suggestions, err := nextepisode.FindForUser(ctx, r.db, principal.UserID, r.clock.Now())
	if err != nil {
		return nil, err
	}
	result := make([]*model.SeriesNextEpisode, 0, len(suggestions))
	if len(suggestions) == 0 {
		return result, nil
	}
	recordingIDs := make([]uuid.UUID, 0, len(suggestions))
	serviceIDs, eventIDs := make([]int64, 0, len(suggestions)), make([]int64, 0, len(suggestions))
	for _, s := range suggestions {
		recordingIDs = append(recordingIDs, s.RecordingID)
		serviceIDs, eventIDs = append(serviceIDs, s.Broadcast.ServiceID), append(eventIDs, s.Broadcast.EventID)
	}
	rows, err := r.queries.ListRecordingsByIDs(ctx, recordingIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watched recordings: %w", err)
	}
	models, err := r.recordingModels(ctx, rows)
	if err != nil {
		return nil, err
	}
	recordings := make(map[uuid.UUID]*model.Recording, len(rows))
	for i := range rows {
		recordings[rows[i].ID] = models[i]
	}
	programs, err := r.programsByKeys(ctx, serviceIDs, eventIDs)
	if err != nil {
		return nil, err
	}
	for _, s := range suggestions {
		recording, ok := recordings[s.RecordingID]
		if !ok {
			continue
		}
		program, ok := programs[programKeyOf(s.Broadcast.ServiceID, s.Broadcast.EventID)]
		if !ok {
			continue
		}
		result = append(result, &model.SeriesNextEpisode{Watched: recording, Program: program})
	}

	return result, nil
}
