package graph

import (
	"context"

	pgx "github.com/jackc/pgx/v5"

	"github.com/SlashNephy/kotatsu/graph/model"
)

// RenamePasskey is the resolver for the renamePasskey field.
func (r *mutationResolver) RenamePasskey(ctx context.Context, id string, name string) (*model.Passkey, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.passkeys == nil {
		return nil, errPasskeysUnavailable
	}
	passkeyID, err := parsePasskeyID(id)
	if err != nil {
		return nil, err
	}
	// 名前は、第二要素の強さを変えないので、再認証を求めない。
	if err := r.inTx(ctx, func(tx pgx.Tx) error {
		return r.passkeys.Rename(ctx, tx, actorOf(ctx, principal), principal.UserID, passkeyID, name)
	}); err != nil {
		return nil, err
	}
	passkeys, err := r.passkeys.List(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	for _, passkey := range passkeys {
		if passkey.ID == passkeyID {
			return passkeyOf(passkey), nil
		}
	}

	return nil, NewError(CodeNotFound, "passkey not found")
}

// DeletePasskey is the resolver for the deletePasskey field.
func (r *mutationResolver) DeletePasskey(ctx context.Context, id string) (string, error) {
	principal, actor, err := r.recentlyAuthenticated(ctx)
	if err != nil {
		return "", err
	}
	if r.passkeys == nil {
		return "", errPasskeysUnavailable
	}
	passkeyID, err := parsePasskeyID(id)
	if err != nil {
		return "", err
	}
	if err := r.inTx(ctx, func(tx pgx.Tx) error {
		return r.passkeys.Delete(ctx, tx, actor, principal.UserID, passkeyID)
	}); err != nil {
		return "", err
	}

	return id, nil
}

// Passkeys is the resolver for the passkeys field.
func (r *queryResolver) Passkeys(ctx context.Context) ([]*model.Passkey, error) {
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	if r.passkeys == nil {
		return nil, errPasskeysUnavailable
	}
	passkeys, err := r.passkeys.List(ctx, r.db, principal.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]*model.Passkey, 0, len(passkeys))
	for _, passkey := range passkeys {
		result = append(result, passkeyOf(passkey))
	}

	return result, nil
}
