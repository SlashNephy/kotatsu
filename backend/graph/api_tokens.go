package graph

import (
	"errors"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/auth"
)

// apiTokenType は、API トークンの ID に付ける型の名前。
const apiTokenType = "ApiToken"

// errAPITokensUnavailable は、API トークンを扱う設定がないことを表す。
var errAPITokensUnavailable = errors.New("API tokens are not configured")

func apiTokenOf(token *auth.APIToken) *model.APIToken {
	return &model.APIToken{
		ID:         GlobalID(apiTokenType, token.ID.String()),
		Name:       token.Name,
		CreatedAt:  token.CreatedAt,
		LastUsedAt: token.LastUsedAt,
	}
}

func parseAPITokenID(globalID string) (uuid.UUID, error) {
	typeName, raw, err := ParseGlobalID(globalID)
	if err != nil || typeName != apiTokenType {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid API token ID")
	}
	tokenID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, NewError(CodeBadUserInput, "invalid API token ID")
	}

	return tokenID, nil
}

// apiTokenError は、API トークンの操作のエラーを、エラーコードにする。
func apiTokenError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidAPITokenName):
		return NewError(CodeBadUserInput, "the name must be 1 to 64 characters")
	case errors.Is(err, auth.ErrAPITokenNotFound):
		return NewError(CodeNotFound, "API token not found")
	default:
		return err
	}
}
