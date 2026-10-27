package databasetest

import (
	"context"
	"net"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// errUnreachable は、Switch で DB に届かないことにしている間の、接続のエラー。DB が止まって接続を断られたときと同じエラーにする。
var errUnreachable = &net.OpError{Op: "dial", Net: "tcp", Source: nil, Addr: nil, Err: syscall.ECONNREFUSED}

// Switch は、DB に届くかどうかを切り替えられる接続プール。DB が止まった状態を作るのに使う。
type Switch struct {
	// Pool は、Down の間は問い合わせが接続のエラーで失敗するプール。
	Pool *pgxpool.Pool

	down atomic.Bool
}

// NewSwitch は、pool と同じ DB につなぐ、届くかどうかを切り替えられるプールを作る。プールはテストの終了時に閉じる。
func NewSwitch(tb testing.TB, pool *pgxpool.Pool) *Switch {
	tb.Helper()

	s := &Switch{Pool: nil, down: atomic.Bool{}}
	cfg := pool.Config().Copy()
	cfg.BeforeConnect = func(context.Context, *pgx.ConnConfig) error {
		if s.down.Load() {
			return errUnreachable
		}

		return nil
	}
	// 届かない間は、プールに残った接続も捨て、問い合わせを接続のエラーで失敗させる。
	cfg.PrepareConn = func(context.Context, *pgx.Conn) (bool, error) {
		if s.down.Load() {
			return false, errUnreachable
		}

		return true, nil
	}
	p, err := pgxpool.NewWithConfig(tb.Context(), cfg)
	require.NoError(tb, err)
	tb.Cleanup(p.Close)
	s.Pool = p

	return s
}

// Down は、DB に届かないことにする。
func (s *Switch) Down() {
	s.down.Store(true)
}

// Up は、DB に届くことにする。
func (s *Switch) Up() {
	s.down.Store(false)
}
