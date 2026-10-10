package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// AuditEvents is the resolver for the auditEvents field.
func (r *queryResolver) AuditEvents(ctx context.Context, first int, after *string, filter *model.AuditEventFilter) (*model.AuditEventConnection, error) {
	return r.listAuditEvents(ctx, first, after, filter)
}

// SystemStatus is the resolver for the systemStatus field.
func (r *queryResolver) SystemStatus(ctx context.Context) (*model.SystemStatus, error) {
	return r.systemStatus(ctx)
}
