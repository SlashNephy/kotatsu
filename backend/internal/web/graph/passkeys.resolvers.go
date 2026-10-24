package graph

import (
	"context"

	"github.com/SlashNephy/kotatsu/internal/usecase/auth"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// RenamePasskey is the resolver for the renamePasskey field.
func (r *mutationResolver) RenamePasskey(ctx context.Context, id string, name string) (*model.Passkey, error) {
	if r.passkeys == nil {
		return nil, errPasskeysUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
	}
	passkeyID, err := parsePasskeyID(id)
	if err != nil {
		return nil, err
	}
	// 名前は、第二要素の強さを変えないので、再認証を求めない。
	if err := secondFactorError(r.passkeys.Rename(ctx, r.db, actorOf(ctx, principal), principal.UserID, passkeyID, name)); err != nil {
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
	if r.passkeys == nil {
		return "", errPasskeysUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return "", err
	}
	// パスキーを消すことは、第二要素を変える操作なので、直前の再認証を求める (設計書 9.5)。
	if err := auth.RequireRecentAuthentication(ctx, r.clock); err != nil {
		return "", err
	}
	actor := actorOf(ctx, principal)
	passkeyID, err := parsePasskeyID(id)
	if err != nil {
		return "", err
	}
	if err := secondFactorError(r.passkeys.Delete(ctx, r.db, actor, principal.UserID, passkeyID)); err != nil {
		return "", err
	}

	return id, nil
}

// Passkeys is the resolver for the passkeys field.
func (r *queryResolver) Passkeys(ctx context.Context) ([]*model.Passkey, error) {
	if r.passkeys == nil {
		return nil, errPasskeysUnavailable
	}
	principal, err := viewer(ctx)
	if err != nil {
		return nil, err
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
