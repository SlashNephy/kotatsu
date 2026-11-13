package edcbimport

import (
	"github.com/SlashNephy/kotatsu/internal/domain/match"
	"github.com/SlashNephy/kotatsu/internal/infra/edcb"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
)

// RuleInput は、テストから ruleInput を呼ぶ。
func RuleInput(a *edcb.AutoAdd, services map[int64]bool) (*rule.Input, error) {
	return ruleInput(a, services)
}

// TimeRangeOf は、テストから timeRangeOf を呼ぶ。
func TimeRangeOf(d edcb.SearchDate) *match.TimeRange {
	return timeRangeOf(d)
}

// ComplementTimeRanges は、テストから complementTimeRanges を呼ぶ。
func ComplementTimeRanges(dates []edcb.SearchDate) []*match.TimeRange {
	return complementTimeRanges(dates)
}
