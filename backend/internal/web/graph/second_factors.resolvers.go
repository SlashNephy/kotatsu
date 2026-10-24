package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// BeginTotpEnrollment is the resolver for the beginTotpEnrollment field.
func (r *mutationResolver) BeginTotpEnrollment(ctx context.Context) (*model.TOTPEnrollment, error) {
	principal, _, err := r.recentlyAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	user, err := loadersFrom(ctx).Users.Load(ctx, principal.UserID)
	if err != nil {
		return nil, err
	}
	enrollment, err := r.factors.BeginTOTP(ctx, r.db, principal.UserID, user.Username)
	if err != nil {
		return nil, err
	}

	return &model.TOTPEnrollment{Secret: enrollment.Secret, URI: enrollment.URI, ExpiresAt: enrollment.ExpiresAt}, nil
}

// ConfirmTotpEnrollment is the resolver for the confirmTotpEnrollment field.
func (r *mutationResolver) ConfirmTotpEnrollment(ctx context.Context, code string) (*model.RecoveryCodes, error) {
	principal, actor, err := r.recentlyAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	codes, err := r.factors.ConfirmTOTP(ctx, r.db, actor, principal.UserID, code)
	if err := secondFactorError(err); err != nil {
		return nil, err
	}

	return &model.RecoveryCodes{Codes: codes}, nil
}

// DeleteTotp is the resolver for the deleteTotp field.
func (r *mutationResolver) DeleteTotp(ctx context.Context) (*model.SecondFactorStatus, error) {
	principal, actor, err := r.recentlyAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	if err := secondFactorError(r.factors.DeleteTOTP(ctx, r.db, actor, principal.UserID)); err != nil {
		return nil, err
	}

	return r.secondFactorStatus(ctx, principal)
}

// RegenerateRecoveryCodes is the resolver for the regenerateRecoveryCodes field.
func (r *mutationResolver) RegenerateRecoveryCodes(ctx context.Context) (*model.RecoveryCodes, error) {
	principal, actor, err := r.recentlyAuthenticated(ctx)
	if err != nil {
		return nil, err
	}
	codes, err := r.factors.RegenerateRecoveryCodes(ctx, r.db, actor, principal.UserID)
	if err := secondFactorError(err); err != nil {
		return nil, err
	}

	return &model.RecoveryCodes{Codes: codes}, nil
}

// SecondFactors is the resolver for the secondFactors field.
func (r *queryResolver) SecondFactors(ctx context.Context) (*model.SecondFactorStatus, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}

	return r.secondFactorStatus(ctx, principal)
}
