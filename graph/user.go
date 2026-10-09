package graph

import (
	"strings"

	"github.com/SlashNephy/kotatsu/graph/model"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
)

// userType は、User の Node の ID に使う型の名前。
const userType = "User"

func toUser(user *sqlc.User) *model.User {
	return &model.User{
		ID:          GlobalID(userType, user.ID.String()),
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        model.Role(strings.ToUpper(user.Role)),
	}
}
