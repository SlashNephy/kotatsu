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

// TunerServerHealth is the resolver for the tunerServerHealth field.
func (r *queryResolver) TunerServerHealth(ctx context.Context) (*model.TunerServerHealth, error) {
	return r.tunerServerHealth(ctx)
}

// StoragePoolIds is the resolver for the storagePoolIds field.
func (r *queryResolver) StoragePoolIds(ctx context.Context) ([]string, error) {
	return r.storagePoolIDs(ctx)
}
