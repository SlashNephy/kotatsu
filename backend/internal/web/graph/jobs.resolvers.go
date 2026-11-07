package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// RetryJob is the resolver for the retryJob field.
func (r *mutationResolver) RetryJob(ctx context.Context, id string) (*model.Job, error) {
	return r.changeJob(ctx, id, actionJobRetry)
}

// CancelJob is the resolver for the cancelJob field.
func (r *mutationResolver) CancelJob(ctx context.Context, id string) (*model.Job, error) {
	return r.changeJob(ctx, id, actionJobCancel)
}

// Jobs is the resolver for the jobs field.
func (r *queryResolver) Jobs(ctx context.Context, state model.JobState, first int, after *string) (*model.JobPage, error) {
	return r.listJobs(ctx, state, first, after)
}

// JobCounts is the resolver for the jobCounts field.
func (r *queryResolver) JobCounts(ctx context.Context) ([]*model.JobStateCount, error) {
	return r.jobCounts(ctx)
}
