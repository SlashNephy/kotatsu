package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/infra/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

// ErrDatabaseUnavailable は、DB に届かず、覚えた設定もないので、URL を受け付けるかを確かめられないことを表す (#1084)。
var ErrDatabaseUnavailable = errors.New("the database is unavailable")

// URLGate は、外の道具に渡す URL (外部プレイヤー用の署名付き URL と、IPTV のクライアント用の URL) を受け付けるかを、
// 要求のたびに全体の設定から読む (#1755、設計書 7.6、9.8)。管理者が無効にしたら、どのレプリカでも、発行済みの URL をすぐに断るためである。
// DB に届かない間は、database.FallbackTTL の中で読めた設定を使う (#1084)。読めた設定がなければ、ErrDatabaseUnavailable を包んで返す。
type URLGate struct {
	db       sqlc.DBTX
	fallback *database.Fallback[struct{}, *System]
}

// NewURLGate は URLGate を返す。logger には、DB に届かずに覚えた設定を使ったことを出す。
func NewURLGate(db sqlc.DBTX, clk clock.Clock, logger *slog.Logger) *URLGate {
	return &URLGate{db: db, fallback: database.NewFallback[struct{}, *System]("url_gate", database.FallbackTTL, clk, logger)}
}

// ExternalPlayerURLsEnabled は、外部プレイヤー用の署名付き URL を受け付けるかどうかを返す。
// DB に届かず、覚えた設定もなければ、ErrDatabaseUnavailable を包んで返す。
func (g *URLGate) ExternalPlayerURLsEnabled(ctx context.Context) (bool, error) {
	system, err := g.load(ctx)
	if err != nil {
		return false, err
	}

	return system.ExternalPlayerURLsEnabled, nil
}

// IPTVFeedsEnabled は、IPTV のクライアント用の URL を受け付けるかどうかを返す。
// DB に届かず、覚えた設定もなければ、ErrDatabaseUnavailable を包んで返す。
func (g *URLGate) IPTVFeedsEnabled(ctx context.Context) (bool, error) {
	system, err := g.load(ctx)
	if err != nil {
		return false, err
	}

	return system.IPTVFeedsEnabled, nil
}

// FallbackUsed は、DB に届かずに、覚えた設定を使った回数を返す。
func (g *URLGate) FallbackUsed() int64 {
	return g.fallback.Used()
}

func (g *URLGate) load(ctx context.Context) (*System, error) {
	system, _, err := g.fallback.Load(ctx, struct{}{}, func(ctx context.Context) (*System, error) {
		return Load(ctx, g.db)
	})
	if database.IsUnavailable(err) {
		return nil, fmt.Errorf("%w: %w", ErrDatabaseUnavailable, err)
	}

	return system, err
}
