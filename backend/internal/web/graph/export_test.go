package graph

import (
	"github.com/SlashNephy/kotatsu/internal/audit"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/reserve"
	"github.com/SlashNephy/kotatsu/internal/rule"
)

// NewLoaders は newLoaders をテストから呼ぶ。
func NewLoaders(db sqlc.DBTX) *Loaders {
	clk := new(clock.Real)

	return newLoaders(db, rule.NewRules(clk, audit.NewRecorder(clk)), reserve.NewReservations(clk, audit.NewRecorder(clk)))
}
