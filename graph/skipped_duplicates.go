package graph

import (
	"context"
	"errors"
	"fmt"

	pgx "github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/rule"
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
	q := sqlc.New(r.db)
	result := make([]*model.SkippedDuplicate, 0, len(skipped))
	for _, s := range skipped {
		row, err := q.GetGuideProgram(ctx, sqlc.GetGuideProgramParams{ServiceID: s.ServiceID, EventID: s.EventID})
		// 計算した後に番組が消えていれば、飛ばす。
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get the program: %w", err)
		}
		program, err := toProgram((*programRow)(&row))
		if err != nil {
			return nil, err
		}
		rules := make([]*model.SkippedRule, 0, len(s.Rules))
		for _, skippedRule := range s.Rules {
			rules = append(rules, &model.SkippedRule{ID: GlobalID(ruleType, skippedRule.ID.String()), Name: skippedRule.Name})
		}
		result = append(result, &model.SkippedDuplicate{Program: program, Rules: rules, LastRecordedAt: s.LastRecordedAt})
	}

	return result, nil
}
