package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/usecase/watch"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

const watchRuleType = "WatchRule"

// parseWatchRuleID は、WatchRule の ID から、視聴ルールの ID を取り出す。
func parseWatchRuleID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != watchRuleType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid watch rule ID")
	}
	ruleID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid watch rule ID")
	}

	return ruleID, nil
}

// toWatchRuleInput は、GraphQL の視聴ルールの内容を、視聴ルールの入力にする。内容の検証は watch が行う。
func toWatchRuleInput(in *model.WatchRuleInput) (*watch.RuleInput, error) {
	conditions, err := toConditions(in.Conditions)
	if err != nil {
		return nil, err
	}

	return &watch.RuleInput{Name: in.Name, Enabled: in.Enabled, Conditions: conditions}, nil
}

func toWatchRule(r *watch.Rule) *model.WatchRule {
	return &model.WatchRule{
		ID:         GlobalID(watchRuleType, r.ID.String()),
		Name:       r.Name,
		Enabled:    r.Enabled,
		Conditions: toProgramConditions(r.Conditions),
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// watchRuleError は、視聴ルールの操作のエラーを、エラーコードを持つエラーにする。
func watchRuleError(err error) error {
	switch {
	case errors.Is(err, watch.ErrRuleNotFound):
		return NewError(CodeNotFound, "watch rule not found")
	case errors.Is(err, watch.ErrTooManyRules):
		return NewError(CodeConflict, fmt.Sprintf("at most %d watch rules can be created", watch.MaxRulesPerUser))
	case errors.Is(err, watch.ErrInvalidRule):
		return NewError(CodeBadUserInput, err.Error())
	default:
		return err
	}
}

// watchRule は、ログイン中のユーザーの視聴ルールを引く。ほかのユーザーのものは見つからないことにする。
func (r *Resolver) watchRule(ctx context.Context, globalID string) (*model.WatchRule, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	ruleID, err := parseWatchRuleID(globalID)
	if err != nil {
		return nil, err
	}
	found, err := watch.GetRule(ctx, r.db, principal.UserID, ruleID)
	if err != nil {
		return nil, watchRuleError(err)
	}

	return toWatchRule(found), nil
}

// listWatchRules は、ログイン中のユーザーの視聴ルールを、作った順に返す。
func (r *Resolver) listWatchRules(ctx context.Context) ([]*model.WatchRule, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	rules, err := watch.ListRules(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	models := make([]*model.WatchRule, 0, len(rules))
	for _, rule := range rules {
		models = append(models, toWatchRule(rule))
	}

	return models, nil
}

// saveWatchRule は、ログイン中のユーザーの視聴ルールを作るか (globalID が nil)、内容を input で置き換える。
// 視聴ルールは自分のものなので、閲覧だけのユーザーも使える (設計書 9.7)。
func (r *Resolver) saveWatchRule(ctx context.Context, globalID *string, input *model.WatchRuleInput) (*model.WatchRule, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	in, err := toWatchRuleInput(input)
	if err != nil {
		return nil, err
	}
	var saved *watch.Rule
	if globalID == nil {
		saved, err = watch.CreateRule(ctx, r.db, r.clock, principal.UserID, in)
	} else {
		ruleID, parseErr := parseWatchRuleID(*globalID)
		if parseErr != nil {
			return nil, parseErr
		}
		saved, err = watch.UpdateRule(ctx, r.db, r.clock, principal.UserID, ruleID, in)
	}
	if err != nil {
		return nil, watchRuleError(err)
	}

	return toWatchRule(saved), nil
}

// setWatchRuleEnabled は、ログイン中のユーザーの視聴ルールを有効または無効にする。
func (r *Resolver) setWatchRuleEnabled(ctx context.Context, globalID string, enabled bool) (*model.WatchRule, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	ruleID, err := parseWatchRuleID(globalID)
	if err != nil {
		return nil, err
	}
	updated, err := watch.SetRuleEnabled(ctx, r.db, r.clock, principal.UserID, ruleID, enabled)
	if err != nil {
		return nil, watchRuleError(err)
	}

	return toWatchRule(updated), nil
}

// deleteWatchRule は、ログイン中のユーザーの視聴ルールを消す。視聴ルールが付けた視聴予約は残す。
func (r *Resolver) deleteWatchRule(ctx context.Context, globalID string) (string, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	ruleID, err := parseWatchRuleID(globalID)
	if err != nil {
		return "", err
	}
	if err := watch.DeleteRule(ctx, r.db, r.clock, principal.UserID, ruleID); err != nil {
		return "", watchRuleError(err)
	}

	return globalID, nil
}

// ownNode は、ログイン中のユーザーだけのもの (番組表ビューと視聴ルール) を、グローバル ID で引く。
// ほかのユーザーのものは見つからないことにする。
func (r *Resolver) ownNode(ctx context.Context, typeName, globalID string) (model.Node, error) {
	if typeName == watchRuleType {
		return r.watchRule(ctx, globalID)
	}
	view, err := r.guideView(ctx, globalID)
	if err != nil {
		return nil, err
	}

	return toGuideView(view)
}
