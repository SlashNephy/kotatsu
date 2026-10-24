package graph

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// NewLoaders は newLoaders をテストから呼ぶ。
func NewLoaders(pool *pgxpool.Pool) *Loaders {
	clk := new(clock.Real)

	return newLoaders(pool, query.New(pool), clk, rule.NewRules(clk, audit.NewRecorder(clk)))
}
