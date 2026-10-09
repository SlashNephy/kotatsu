package graph

import "github.com/SlashNephy/kotatsu/internal/database/sqlc"

// NewLoaders は newLoaders をテストから呼ぶ。
func NewLoaders(db sqlc.DBTX) *Loaders {
	return newLoaders(db)
}
