package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/generated"
	"github.com/SlashNephy/kotatsu/graph/model"
)

// CreateRule is the resolver for the createRule field.
func (r *mutationResolver) CreateRule(ctx context.Context, input model.RuleInput) (*model.Rule, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	in, err := toRuleInput(&input)
	if err != nil {
		return nil, err
	}
	created, err := r.rules.Create(ctx, r.db, actor, in)
	if err != nil {
		return nil, err
	}

	return toRuleModel(ctx, created)
}

// UpdateRule is the resolver for the updateRule field.
func (r *mutationResolver) UpdateRule(ctx context.Context, id string, input model.RuleInput) (*model.Rule, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	ruleID, err := parseRuleID(id)
	if err != nil {
		return nil, err
	}
	in, err := toRuleInput(&input)
	if err != nil {
		return nil, err
	}
	updated, err := r.rules.Update(ctx, r.db, actor, ruleID, in)
	if err != nil {
		return nil, err
	}

	return toRuleModel(ctx, updated)
}

// SetRuleEnabled is the resolver for the setRuleEnabled field.
func (r *mutationResolver) SetRuleEnabled(ctx context.Context, id string, enabled bool) (*model.Rule, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return nil, err
	}
	ruleID, err := parseRuleID(id)
	if err != nil {
		return nil, err
	}
	updated, err := r.rules.SetEnabled(ctx, r.db, actor, ruleID, enabled)
	if err != nil {
		return nil, err
	}

	return toRuleModel(ctx, updated)
}

// DeleteRule is the resolver for the deleteRule field.
func (r *mutationResolver) DeleteRule(ctx context.Context, id string) (string, error) {
	actor, err := scheduler(ctx)
	if err != nil {
		return "", err
	}
	ruleID, err := parseRuleID(id)
	if err != nil {
		return "", err
	}
	if err := r.rules.Delete(ctx, r.db, actor, ruleID); err != nil {
		return "", err
	}

	return id, nil
}

// SetRulesEnabled is the resolver for the setRulesEnabled field.
func (r *mutationResolver) SetRulesEnabled(ctx context.Context, ids []string, enabled bool) (*model.BulkOperationPayload, error) {
	return r.setRulesEnabled(ctx, ids, enabled)
}

// DeleteRules is the resolver for the deleteRules field.
func (r *mutationResolver) DeleteRules(ctx context.Context, ids []string) (*model.BulkOperationPayload, error) {
	return r.deleteRules(ctx, ids)
}

// Services is the resolver for the services field.
func (r *programConditionsResolver) Services(ctx context.Context, obj *model.ProgramConditions) ([]*model.Service, error) {
	return r.loadServices(ctx, obj.ServiceKeys)
}

// ExcludeServices is the resolver for the excludeServices field.
func (r *programConditionsResolver) ExcludeServices(ctx context.Context, obj *model.ProgramConditions) ([]*model.Service, error) {
	return r.loadServices(ctx, obj.ExcludeServiceKeys)
}

// Rules is the resolver for the rules field.
func (r *queryResolver) Rules(ctx context.Context) ([]*model.Rule, error) {
	rules, err := r.rules.List(ctx, r.db)
	if err != nil {
		return nil, err
	}

	return toRules(ctx, rules...)
}

// PreviewRule is the resolver for the previewRule field.
func (r *queryResolver) PreviewRule(ctx context.Context, input model.RuleInput, first int, after *string) (*model.ProgramConnection, error) {
	return r.previewRule(ctx, &input, first, after)
}

// UpcomingReservationCount is the resolver for the upcomingReservationCount field.
func (r *ruleResolver) UpcomingReservationCount(ctx context.Context, obj *model.Rule) (int, error) {
	return loadersFrom(ctx).UpcomingReservationCounts.Load(ctx, obj.Key)
}

// ProgramConditions returns generated.ProgramConditionsResolver implementation.
func (r *Resolver) ProgramConditions() generated.ProgramConditionsResolver {
	return &programConditionsResolver{r}
}

// Rule returns generated.RuleResolver implementation.
func (r *Resolver) Rule() generated.RuleResolver { return &ruleResolver{r} }

type (
	programConditionsResolver struct{ *Resolver }
	ruleResolver              struct{ *Resolver }
)
