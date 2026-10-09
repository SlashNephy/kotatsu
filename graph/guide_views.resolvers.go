package graph

import (
	"context"
	"errors"
	"fmt"

	"github.com/vikstrous/dataloadgen"

	"github.com/SlashNephy/kotatsu/graph/generated"
	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// Services is the resolver for the services field.
func (r *guideViewResolver) Services(ctx context.Context, obj *model.GuideView) ([]*model.Service, error) {
	rows, err := loadersFrom(ctx).Services.LoadAll(ctx, obj.ServiceKeys)
	var errs dataloadgen.ErrorSlice
	if err != nil && !errors.As(err, &errs) {
		return nil, err
	}
	services := make([]*model.Service, 0, len(rows))
	for i, row := range rows {
		// 並べた後に消えたサービスは飛ばす。
		if row == nil || (i < len(errs) && errs[i] != nil) {
			continue
		}
		services = append(services, toService(row))
	}

	return services, nil
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
	n, err := sqlc.New(r.db).DeleteGuideView(ctx, sqlc.DeleteGuideViewParams{ID: viewID, UserID: principal.UserID})
	if err != nil {
		return "", fmt.Errorf("failed to delete the guide view: %w", err)
	}
	if n == 0 {
		return "", NewError(CodeNotFound, "guide view not found")
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
