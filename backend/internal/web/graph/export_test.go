package graph

import (
	"context"

	"github.com/graph-gophers/dataloader/v7"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/query"
	"github.com/SlashNephy/kotatsu/internal/usecase/audit"
	"github.com/SlashNephy/kotatsu/internal/usecase/rule"
	"github.com/SlashNephy/kotatsu/internal/web/graph/model"
)

// NewLoaders は newLoaders をテストから呼ぶ。
func NewLoaders(pool *pgxpool.Pool) *Loaders {
	clk := new(clock.Real)

	return newLoaders(pool, query.New(pool), clk, rule.NewRules(clk, audit.NewRecorder(clk)))
}

// LoadAll は loadAll をテストから呼ぶ。
func LoadAll[K comparable, V any](ctx context.Context, loader *dataloader.Loader[K, V], keys []K) ([]V, error) {
	return loadAll(ctx, loader, keys)
}

// LoadServices は、pool から引く Loaders を context に入れて、loadServices をテストから呼ぶ。
func LoadServices(ctx context.Context, pool *pgxpool.Pool, keys []int64) ([]*model.Service, error) {
	ctx = context.WithValue(ctx, loadersKey{}, NewLoaders(pool))

	return new(Resolver).loadServices(ctx, keys)
}
