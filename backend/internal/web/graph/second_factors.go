package graph

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// errSecondFactorsUnavailable は、第二要素を扱う設定 (アプリの秘密キー) がないことを表す。
var errSecondFactorsUnavailable = errors.New("second factors are not configured")

// recentlyAuthenticated は、ログイン中のユーザーが直近 10 分以内に認証したことを確かめ、操作者を返す。
// 第二要素を変える前に、もう一度認証してもらう (設計書 9.5)。
func (r *Resolver) recentlyAuthenticated(ctx context.Context) (*auth.Principal, *audit.Actor, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	if r.factors == nil {
		return nil, nil, errSecondFactorsUnavailable
	}
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return nil, nil, err
	}

	return principal, actorOf(ctx, principal), nil
}

// secondFactorStatus は、ユーザーの第二要素の状態を返す。
func (r *Resolver) secondFactorStatus(ctx context.Context, principal *auth.Principal) (*model.SecondFactorStatus, error) {
	if r.factors == nil {
		return nil, errSecondFactorsUnavailable
	}
	totp, remaining, err := r.factors.Status(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}

	return &model.SecondFactorStatus{Totp: totp, RemainingRecoveryCodes: remaining}, nil
}

// secondFactorError は、第二要素の操作のエラーを、エラーコードにする。
func secondFactorError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidSecondFactor):
		return NewError(CodeBadUserInput, "invalid code")
	case errors.Is(err, auth.ErrNoTOTPEnrollment):
		return NewError(CodeBadUserInput, "start the TOTP enrollment first")
	case errors.Is(err, auth.ErrNoTOTP):
		return NewError(CodeNotFound, "TOTP is not enabled")
	case errors.Is(err, auth.ErrPasskeyNotFound):
		return NewError(CodeNotFound, "passkey not found")
	case errors.Is(err, auth.ErrInvalidPasskeyName):
		return NewError(CodeBadUserInput, "the name must be 1 to 64 characters")
	case errors.Is(err, auth.ErrLastPasskey):
		return NewError(CodeBadUserInput, "the last passkey of a user without a password cannot be deleted")
	default:
		return err
	}
}

// inTx は、第二要素の操作を、1 つのトランザクションで行う。
func (r *Resolver) inTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return secondFactorError(pgx.BeginFunc(ctx, r.db, fn))
}
