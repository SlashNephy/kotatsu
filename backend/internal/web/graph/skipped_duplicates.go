package graph

import (
	"context"
	"fmt"

	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// skippedDuplicates は、重複の回避で予約しなかった番組を返す。
func (r *Resolver) skippedDuplicates(ctx context.Context) ([]*model.SkippedDuplicate, error) {
	if _, err := viewer(ctx); err != nil {
		return nil, err
	}
	skipped, err := rule.FindSkippedDuplicates(ctx, r.db, r.clock, r.location)
	if err != nil {
		return nil, err
	}
	serviceIDs := make([]int64, 0, len(skipped))
	eventIDs := make([]int64, 0, len(skipped))
	for _, s := range skipped {
		serviceIDs = append(serviceIDs, s.ServiceID)
		eventIDs = append(eventIDs, s.EventID)
	}
	rows, err := r.queries.ListGuideProgramsByKeys(ctx, query.ListGuideProgramsByKeysParams{ServiceIDs: serviceIDs, EventIDs: eventIDs})
	if err != nil {
		return nil, fmt.Errorf("failed to list the programs: %w", err)
	}
	type key struct{ serviceID, eventID int64 }
	programs := make(map[key]*model.Program, len(rows))
	for i := range rows {
		program, err := toProgram(&rows[i])
		if err != nil {
			return nil, err
		}
		programs[key{serviceID: rows[i].ServiceID, eventID: rows[i].EventID}] = program
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
		result = append(result, &model.SkippedDuplicate{Program: program, Rules: rules, LastRecordedAt: s.LastRecordedAt})
	}

	return result, nil
}
