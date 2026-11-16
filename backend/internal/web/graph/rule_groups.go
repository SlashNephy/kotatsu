package graph

import (
	"context"
	"fmt"
	"uuid"

	"github.com/graph-gophers/dataloader/v7"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const ruleGroupType = "RuleGroup"

// parseRuleGroupID は、RuleGroup の ID から、グループの ID を取り出す。
func parseRuleGroupID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != ruleGroupType {
		return uuid.Nil(), NewError(CodeBadUserInput, "invalid rule group ID")
	}
	groupID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil(), NewError(CodeBadUserInput, "invalid rule group ID")
	}

	return groupID, nil
}

func toRuleGroup(g *rule.Group) *model.RuleGroup {
	return &model.RuleGroup{
		ID:        GlobalID(ruleGroupType, g.ID.String()),
		Name:      g.Name,
		Position:  g.Position,
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}
}

func toRuleGroups(groups []*rule.Group) []*model.RuleGroup {
	models := make([]*model.RuleGroup, 0, len(groups))
	for _, g := range groups {
		models = append(models, toRuleGroup(g))
	}

	return models
}

// ruleGroup は、グループを引く。ルールと同じく、ログインしたユーザーなら誰でも読める。
func (r *Resolver) ruleGroup(ctx context.Context, globalID string) (*model.RuleGroup, error) {
	groupID, err := parseRuleGroupID(globalID)
	if err != nil {
		return nil, err
	}
	found, err := r.rules.GetGroup(ctx, r.db, groupID)
	if err != nil {
		return nil, err
	}

	return toRuleGroup(found), nil
}

// ruleGroupOf は、ルールが入るグループを引く。どのグループにも入らなければ nil を返す。
func ruleGroupOf(ctx context.Context, obj *model.Rule) (*model.RuleGroup, error) {
	var found *rule.Group
	var err error
	if obj.GroupKey != nil {
		found, err = loadersFrom(ctx).RuleGroups.Load(ctx, *obj.GroupKey)()
	}
	// グループのないルールと、読む間に消えたグループは、null で返す。
	if err != nil || found == nil {
		return nil, err
	}

	return toRuleGroup(found), nil
}

// setRulesGroup は、ルール ids をまとめてグループに移す (#1493)。1 つのトランザクションで移し、監査ログはルールごとに残す。
func (r *Resolver) setRulesGroup(ctx context.Context, ids []string, groupID *string) (*model.BulkOperationPayload, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) > maxBulkOperations {
		return nil, NewError(CodeBadUserInput, fmt.Sprintf("ids must be at most %d", maxBulkOperations))
	}
	// group は、移す先のグループ。null なら、どのグループにも入れない。
	var group *uuid.UUID
	if groupID != nil {
		parsed, err := parseRuleGroupID(*groupID)
		if err != nil {
			return nil, err
		}
		group = &parsed
	}
	ruleIDs := make([]uuid.UUID, 0, len(ids))
	for _, globalID := range ids {
		ruleID, err := parseRuleID(globalID)
		if err != nil {
			return nil, err
		}
		ruleIDs = append(ruleIDs, ruleID)
	}
	result, err := r.rules.SetGroup(ctx, r.db, actor, ruleIDs, group)
	if err != nil {
		return nil, err
	}

	return &model.BulkOperationPayload{Processed: result.Processed, Skipped: result.Skipped}, nil
}

// reorderRuleGroups は、グループを ids の順に並べ直す。
func (r *Resolver) reorderRuleGroups(ctx context.Context, ids []string) ([]*model.RuleGroup, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]uuid.UUID, 0, len(ids))
	for _, globalID := range ids {
		groupID, err := parseRuleGroupID(globalID)
		if err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	groups, err := r.rules.ReorderGroups(ctx, r.db, actor, groupIDs)
	if err != nil {
		return nil, err
	}

	return toRuleGroups(groups), nil
}

// newRuleGroupsLoader は、ID のルールのグループをまとめて引く dataloader を作る。見つからないグループでは nil にする。
func newRuleGroupsLoader(db *pgxpool.Pool, rules *rule.Rules) *dataloader.Loader[uuid.UUID, *rule.Group] {
	return newLoader(func(ctx context.Context, ids []uuid.UUID) ([]*rule.Group, []error) {
		byID, err := rules.ListGroupsByIDs(ctx, db, ids)
		if err != nil {
			return failAll[*rule.Group](len(ids), err)
		}
		found := make([]*rule.Group, 0, len(ids))
		for _, groupID := range ids {
			found = append(found, byID[groupID])
		}

		return found, make([]error, len(ids))
	})
}
