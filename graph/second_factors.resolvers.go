package graph

import (
	"context"

	pgx "github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
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
	var codes []string
	err = r.inTx(ctx, func(tx pgx.Tx) error {
		codes, err = r.factors.ConfirmTOTP(ctx, tx, actor, principal.UserID, code)

		return err
	})
	if err != nil {
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
	if err := r.inTx(ctx, func(tx pgx.Tx) error {
		return r.factors.DeleteTOTP(ctx, tx, actor, principal.UserID)
	}); err != nil {
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
	var codes []string
	err = r.inTx(ctx, func(tx pgx.Tx) error {
		codes, err = r.factors.RegenerateRecoveryCodes(ctx, tx, actor, principal.UserID)

		return err
	})
	if err != nil {
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
