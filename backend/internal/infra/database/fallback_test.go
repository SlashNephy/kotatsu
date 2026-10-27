package database_test

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/infra/database"
	"github.com/SlashNephy/kotatsu/internal/testutil/clocktest"
)

func TestFallback(t *testing.T) {
	t.Parallel()

	refused := &net.OpError{Op: "dial", Net: "tcp", Source: nil, Addr: nil, Err: syscall.ECONNREFUSED}
	invalid := errors.New("session is invalid")

	// step は、DB から読める値かエラーを決め、時計を進めてから読んだ結果を確かめる。
	type step struct {
		value     string
		err       error
		advance   time.Duration
		want      string
		wantStale bool
		wantErr   error
		// wantLoaded は、DB から読もうとしたかどうか。
		wantLoaded bool
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{
			name: "serves the cached value while unreachable",
			steps: []step{
				{value: "v1", want: "v1", wantLoaded: true},
				{err: refused, advance: time.Minute, want: "v1", wantStale: true, wantLoaded: true},
			},
		},
		{
			name: "stops serving the cached value after the TTL",
			steps: []step{
				{value: "v1", want: "v1", wantLoaded: true},
				{err: refused, advance: database.FallbackTTL, wantErr: refused, wantLoaded: true},
			},
		},
		{
			// DB から使えないと分かったものは、DB が止まった後に覚えた結果で通さない。
			name: "forgets the value that the database rejected",
			steps: []step{
				{value: "v1", want: "v1", wantLoaded: true},
				{err: invalid, wantErr: invalid, wantLoaded: true},
				{err: refused, wantErr: refused, wantLoaded: true},
			},
		},
		{
			name: "skips the database for a while after it is found unreachable",
			steps: []step{
				{value: "v1", want: "v1", wantLoaded: true},
				{err: refused, want: "v1", wantStale: true, wantLoaded: true},
				{err: refused, advance: 4 * time.Second, want: "v1", wantStale: true, wantLoaded: false},
				{value: "v2", advance: time.Second, want: "v2", wantLoaded: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clk := clocktest.NewFake(time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
			fallback := database.NewFallback[string, string]("test", database.FallbackTTL, clk, slog.New(slog.DiscardHandler))
			for i, step := range tt.steps {
				clk.Advance(step.advance)
				loaded := false
				got, stale, err := fallback.Load(t.Context(), "key", func(context.Context) (string, error) {
					loaded = true

					return step.value, step.err
				})
				assert.Equal(t, step.wantLoaded, loaded, "step %d", i)
				if step.wantErr != nil {
					require.ErrorIs(t, err, step.wantErr, "step %d", i)

					continue
				}
				require.NoError(t, err, "step %d", i)
				assert.Equal(t, step.want, got, "step %d", i)
				assert.Equal(t, step.wantStale, stale, "step %d", i)
			}
		})
	}
}
