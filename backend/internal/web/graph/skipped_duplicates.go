package graph

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// skippedDuplicates は、重複の回避、同じ回の束ね、前の回を録らない設定で予約しなかった番組を返す。
func (r *Resolver) skippedDuplicates(ctx context.Context) ([]*model.SkippedDuplicate, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	skipped, err := rule.FindSkippedDuplicates(ctx, r.db, r.clock, r.location)
	if err != nil {
		return nil, err
	}
	type key struct{ serviceID, eventID int64 }
	// 見送った番組と、代わりに録る放送を、まとめて引く。
	keys := map[key]bool{}
	recordedServices := map[int64]bool{}
	for _, s := range skipped {
		keys[key{serviceID: s.ServiceID, eventID: s.EventID}] = true
		if s.Instead != nil {
			keys[key{serviceID: s.Instead.ServiceID, eventID: s.Instead.EventID}] = true
		}
		if s.Recorded != nil {
			recordedServices[s.Recorded.ServiceID] = true
		}
	}
	serviceIDs := make([]int64, 0, len(keys))
	eventIDs := make([]int64, 0, len(keys))
	for k := range keys {
		serviceIDs = append(serviceIDs, k.serviceID)
		eventIDs = append(eventIDs, k.eventID)
	}
	rows, err := r.queries.ListGuideProgramsByKeys(ctx, query.ListGuideProgramsByKeysParams{ServiceIDs: serviceIDs, EventIDs: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs: %w", err)
	}
	programs := make(map[key]*model.Program, len(rows))
	for i := range rows {
		program, err := toProgram(&rows[i])
		if err != nil {
			return nil, err
		}
		programs[key{serviceID: rows[i].ServiceID, eventID: rows[i].EventID}] = program
	}
	loaded, err := r.loadServices(ctx, slices.Sorted(maps.Keys(recordedServices)))
	if err != nil {
		return nil, err
	}
	services := make(map[int64]*model.Service, len(loaded))
	for _, s := range loaded {
		services[s.MirakurunID()] = s
	}

	result := make([]*model.SkippedDuplicate, 0, len(skipped))
	for _, s := range skipped {
		program, ok := programs[key{serviceID: s.ServiceID, eventID: s.EventID}]
		// 計算した後に番組が消えていれば、飛ばす。
		if !ok {
			continue
		}
		rules := make([]*model.SkippedRule, 0, len(s.Rules))
		for _, skippedRule := range s.Rules {
			rules = append(rules, &model.SkippedRule{ID: GlobalID(ruleType, skippedRule.ID.String()), Name: skippedRule.Name})
		}
		item := &model.SkippedDuplicate{
			Program: program, Rules: rules, Reason: s.Reason, LastRecordedAt: s.LastRecordedAt, Recorded: nil, Instead: nil,
			LaterEpisode: s.LaterEpisode,
		}
		if s.Recorded != nil {
			item.Recorded = &model.SkippedRecording{Service: services[s.Recorded.ServiceID], Name: s.Recorded.Name, EndAt: s.Recorded.EndAt}
		}
		if s.Instead != nil {
			item.Instead = programs[key{serviceID: s.Instead.ServiceID, eventID: s.Instead.EventID}]
		}
		result = append(result, item)
	}

	return result, nil
}
