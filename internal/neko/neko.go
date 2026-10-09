// Package neko は neko (scheduler) のロールを実装する。
//
// neko は全体を見て結果を一つに決める処理を担うので、アクティブにするのは 1 台だけにする (設計書 1.3)。
// 複数のインスタンスを起動しても、名前 neko のリースを取ったインスタンスだけがリーダーとして動く。
package neko

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/SlashNephy/kotatsu/internal/clock"
	"github.com/SlashNephy/kotatsu/internal/database/sqlc"
	"github.com/SlashNephy/kotatsu/internal/lease"
)

const (
	// LeaseName は neko のリーダー選出に使うリースの名前。
	LeaseName = "neko"
	// LeaseTTL と RenewInterval は、設計書の付録 A の初期値。
	LeaseTTL      = 15 * time.Second
	RenewInterval = 5 * time.Second
)

// Run は、ctx が終わるまでリースを取り合い、リーダーになっている間だけ neko の処理を動かす。
// 処理の本体はまだなく、リーダーになったことをログに出して待つだけである。
func Run(ctx context.Context, logger *slog.Logger, db sqlc.DBTX, clk clock.Clock, instanceID uuid.UUID) error {
	elector, err := lease.NewElector(lease.NewManager(db, clk, LeaseName, instanceID, LeaseTTL), logger, RenewInterval)
	if err != nil {
		return err
	}

	return elector.Run(ctx, func(ctx context.Context, _ int64) error {
		<-ctx.Done()

		return nil
	})
}
