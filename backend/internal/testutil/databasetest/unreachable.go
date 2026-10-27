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
	// Pool は、Down の間は問い合わせが接続のエラーで失敗し、Hang の間は問い合わせが ctx が終わるまで返らないプール。
	Pool *pgxpool.Pool

	down atomic.Bool
	hang atomic.Bool
	// reached は、DB に届いた問い合わせの数。
	reached atomic.Int64
}

// NewSwitch は、pool と同じ DB につなぐ、届くかどうかを切り替えられるプールを作る。プールはテストの終了時に閉じる。
func NewSwitch(tb testing.TB, pool *pgxpool.Pool) *Switch {
	tb.Helper()

	s := &Switch{Pool: nil, down: atomic.Bool{}, hang: atomic.Bool{}, reached: atomic.Int64{}}
	cfg := pool.Config().Copy()
	cfg.BeforeConnect = func(ctx context.Context, _ *pgx.ConnConfig) error {
		return s.check(ctx)
	}
	// 届かない間は、プールに残った接続も捨て、問い合わせを接続のエラーで失敗させる。
	cfg.PrepareConn = func(ctx context.Context, _ *pgx.Conn) (bool, error) {
		if err := s.check(ctx); err != nil {
			return false, err
		}
		s.reached.Add(1)

		return true, nil
	}
	p, err := pgxpool.NewWithConfig(tb.Context(), cfg)
	require.NoError(tb, err)
	tb.Cleanup(p.Close)
	s.Pool = p

	return s
}

// Down は、DB に届かない (接続を断られる) ことにする。
func (s *Switch) Down() {
	s.down.Store(true)
}

// Hang は、DB への問い合わせが、応答のないまま詰まることにする。
func (s *Switch) Hang() {
	s.hang.Store(true)
}

// Reached は、これまでに DB に届いた問い合わせの数を返す。DB を止める前に、問い合わせが進んだことを待つのに使う。
func (s *Switch) Reached() int64 {
	return s.reached.Load()
}

// Up は、DB に届くことにする。
func (s *Switch) Up() {
	s.down.Store(false)
	s.hang.Store(false)
}

// check は、届かないことにしていればエラーを返す。詰まったことにしていれば、ctx が終わるまで返らない。
func (s *Switch) check(ctx context.Context) error {
	if s.hang.Load() {
		<-ctx.Done()

		return ctx.Err()
	}
	if s.down.Load() {
		return errUnreachable
	}

	return nil
}
