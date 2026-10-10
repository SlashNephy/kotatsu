package graph

import (
	"errors"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/auth"
)

// passkeyType は、パスキーの ID に付ける型の名前。
const passkeyType = "Passkey"

// errPasskeysUnavailable は、パスキーを扱う設定がないことを表す。
var errPasskeysUnavailable = errors.New("passkeys are not configured")

func passkeyOf(passkey *auth.Passkey) *model.Passkey {
	return &model.Passkey{
		ID:         GlobalID(passkeyType, passkey.ID.String()),
		Name:       passkey.Name,
		RpID:       passkey.RPID,
		CreatedAt:  passkey.CreatedAt,
		LastUsedAt: passkey.LastUsedAt,
	}
}

func parsePasskeyID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != passkeyType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid passkey ID")
	}
	passkeyID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid passkey ID")
	}

	return passkeyID, nil
}
