package cli_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/SlashNephy/kotatsu/internal/cli"
)

var (
	errSideFailed = errors.New("the side worker failed")
	errMainFailed = errors.New("the main worker failed")
)

// TestRunAlongside は、mikan で録画の Worker の横で動かすタイムシフトの Worker が、エラーで止まっても、録画の Worker を止めないことを確かめる。
// 録画の Worker が返るか、親の ctx が終われば、タイムシフトの Worker も止まる。
func TestRunAlongside(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// sideFails なら、横の Worker はすぐにエラーで返る。そうでなければ、ctx が終わるまで動く。
		sideFails bool
		// mainFails なら、主の Worker はすぐにエラーで返る。そうでなければ、ctx が終わるまで動く。
		mainFails bool
		wantErr   error
	}{
		{name: "横の Worker がエラーで止まっても、主の Worker は動き続ける", sideFails: true, mainFails: false, wantErr: context.Canceled},
		{name: "主の Worker が返ったら、横の Worker も止める", sideFails: false, mainFails: true, wantErr: errMainFailed},
		{name: "親の ctx が終わったら、両方とも止まる", sideFails: false, mainFails: false, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			sideReturned := make(chan struct{})
			side := func(ctx context.Context) error {
				defer close(sideReturned)
				if tt.sideFails {
					return errSideFailed
				}
				<-ctx.Done()

				return ctx.Err()
			}
			mainStarted := make(chan struct{})
			main := func(ctx context.Context) error {
				close(mainStarted)
				if tt.mainFails {
					return errMainFailed
				}
				<-ctx.Done()

				return ctx.Err()
			}
			done := make(chan error, 1)
			go func() { done <- cli.RunAlongside(ctx, slog.New(slog.DiscardHandler), main, side) }()
			<-mainStarted

			if tt.sideFails {
				<-sideReturned
				// 横の Worker が止まった後も、主の Worker は止まらない。
				select {
				case err := <-done:
					require.FailNow(t, "the main worker stopped with the side worker", "%v", err)
				case <-time.After(50 * time.Millisecond):
				}
			}
			if !tt.mainFails {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, tt.wantErr)
			case <-time.After(5 * time.Second):
				require.FailNow(t, "runAlongside did not return")
			}
			// 返ったときには、横の Worker も返っている。
			select {
			case <-sideReturned:
			default:
				assert.Fail(t, "the side worker was still running")
			}
		})
	}
}
