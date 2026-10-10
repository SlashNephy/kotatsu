package graph

import (
	"context"
	"fmt"

	"github.com/SlashNephy/kotatsu/graph/generated"
	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// Services is the resolver for the services field.
func (r *guideViewResolver) Services(ctx context.Context, obj *model.GuideView) ([]*model.Service, error) {
	return r.loadServices(ctx, obj.ServiceKeys)
}

// CreateGuideView is the resolver for the createGuideView field.
func (r *mutationResolver) CreateGuideView(ctx context.Context, input model.GuideViewInput) (*model.GuideView, error) {
	return r.saveGuideView(ctx, nil, &input)
}

// UpdateGuideView is the resolver for the updateGuideView field.
func (r *mutationResolver) UpdateGuideView(ctx context.Context, id string, input model.GuideViewInput) (*model.GuideView, error) {
	viewID, err := parseGuideViewID(id)
	if err != nil {
		return nil, err
	}

	return r.saveGuideView(ctx, &viewID, &input)
}

// DeleteGuideView is the resolver for the deleteGuideView field.
func (r *mutationResolver) DeleteGuideView(ctx context.Context, id string) (string, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	viewID, err := parseGuideViewID(id)
	if err != nil {
		return "", err
	}
	if err := r.deleteGuideView(ctx, principal.UserID, viewID); err != nil {
		return "", err
	}

	return id, nil
}

// GuideViews is the resolver for the guideViews field.
func (r *queryResolver) GuideViews(ctx context.Context) ([]*model.GuideView, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := sqlc.New(r.db).ListGuideViews(ctx, principal.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to list guide views: %w", err)
	}
	views := make([]*model.GuideView, 0, len(rows))
	for i := range rows {
		view, err := toGuideView(&rows[i])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}

	return views, nil
}

// GuideView returns generated.GuideViewResolver implementation.
func (r *Resolver) GuideView() generated.GuideViewResolver { return &guideViewResolver{r} }

// Mutation returns generated.MutationResolver implementation.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

type (
	guideViewResolver struct{ *Resolver }
	mutationResolver  struct{ *Resolver }
)
