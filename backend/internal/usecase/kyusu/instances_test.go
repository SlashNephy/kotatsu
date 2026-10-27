package kyusu_test

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/registry"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
	"github.com/SlashNephy/kotatsu/internal/usecase/kyusu"
)

// switchingInstances は、result が返すものを返す。DB に届かない状態や、DB が戻った直後の状態を作るのに使う。
type switchingInstances struct {
	instances []*registry.Instance
	err       error
	calls     int
}

func (s *switchingInstances) ListAlive(context.Context, string) ([]*registry.Instance, error) {
	s.calls++

	return s.instances, s.err
}

var _ kyusu.Instances = new(switchingInstances)

func TestLastKnownInstances(t *testing.T) {
	t.Parallel()

	alive := []*registry.Instance{{ID: uuid.New(), Role: kyusuRole, Address: "10.0.0.1:8080", StartedAt: time.Time{}, HeartbeatAt: time.Time{}}}
	refused := &net.OpError{Op: "dial", Net: "tcp", Source: nil, Addr: nil, Err: syscall.ECONNREFUSED}
	broken := errors.New("syntax error")

	// step は、登録簿が返すものを変え、時計を進めてから、引いた結果を確かめる。
	type step struct {
		instances []*registry.Instance
		err       error
		advance   time.Duration
		want      []*registry.Instance
		wantErr   error
		// wantQueried は、登録簿を引いたかどうか。
		wantQueried bool
	}
	tests := []struct {
		name     string
		steps    []step
		wantUsed int64
	}{
		{
			name: "unreachable after a successful lookup",
			steps: []step{
				{instances: alive, want: alive, wantQueried: true},
				{err: refused, advance: time.Minute, want: alive, wantQueried: true},
			},
			wantUsed: 1,
		},
		{
			name: "unreachable before any successful lookup",
			steps: []step{
				{err: refused, wantErr: refused, wantQueried: true},
			},
		},
		{
			name: "other errors are returned",
			steps: []step{
				{instances: alive, want: alive, wantQueried: true},
				{err: broken, wantErr: broken, wantQueried: true},
			},
		},
		{
			// 詰まった DB を、リクエストのたびに待たない。
			name: "skips the database for a while after it is found unreachable",
			steps: []step{
				{instances: alive, want: alive, wantQueried: true},
				{err: refused, want: alive, wantQueried: true},
				{err: refused, advance: 4 * time.Second, want: alive, wantQueried: false},
				{err: refused, advance: time.Second, want: alive, wantQueried: true},
			},
			wantUsed: 3,
		},
		{
			// DB が戻った直後は、ハートビートが書かれるまで、kyusu が止まったように見える。
			name: "keeps the last known instances right after the database recovers",
			steps: []step{
				{instances: alive, want: alive, wantQueried: true},
				{err: refused, want: alive, wantQueried: true},
				{instances: []*registry.Instance{}, advance: 10 * time.Second, want: alive, wantQueried: true},
				{instances: alive, want: alive, wantQueried: true},
			},
			wantUsed: 2,
		},
		{
			name: "trusts an empty list long after the database recovers",
			steps: []step{
				{instances: alive, want: alive, wantQueried: true},
				{err: refused, want: alive, wantQueried: true},
				{instances: []*registry.Instance{}, advance: registry.AliveTTL, want: []*registry.Instance{}, wantQueried: true},
			},
			wantUsed: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
			source := &switchingInstances{instances: nil, err: nil, calls: 0}
			instances := kyusu.NewLastKnownInstances(source, clk, slog.New(slog.DiscardHandler))
			for i, step := range tt.steps {
				source.instances, source.err = step.instances, step.err
				clk.Advance(step.advance)
				calls := source.calls
				got, err := instances.ListAlive(t.Context(), kyusuRole)
				assert.Equal(t, step.wantQueried, source.calls > calls, "step %d", i)
				if step.wantErr != nil {
					require.ErrorIs(t, err, step.wantErr, "step %d", i)

					continue
				}
				require.NoError(t, err, "step %d", i)
				assert.Equal(t, step.want, got, "step %d", i)
			}
			assert.Equal(t, tt.wantUsed, instances.FallbackUsed())
		})
	}
}
