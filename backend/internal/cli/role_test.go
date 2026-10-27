package cli_test

import (
	"bytes"
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

// fakeWorker は、fails ならすぐに err で返り、panics ならすぐに panic し、どちらでもなければ ctx が終わるまで動く Worker を返す。
// started は始めたとき、returned は返るときに閉じる。nil なら閉じない。
func fakeWorker(fails bool, err error, panics bool, started, returned chan struct{}) func(context.Context) error {
	return func(ctx context.Context) error {
		if returned != nil {
			defer close(returned)
		}
		if started != nil {
			close(started)
		}
		if fails {
			return err
		}
		if panics {
			panic("the side worker exploded")
		}
		<-ctx.Done()

		return ctx.Err()
	}
}

// TestRunAlongside は、mikan で録画の Worker の横で動かすタイムシフトの Worker が、エラーで止まるか panic しても、録画の Worker を止めないことを確かめる。
// 録画の Worker が返るか、親の ctx が終われば、タイムシフトの Worker も止まる。
func TestRunAlongside(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// sideFails なら、横の Worker はすぐにエラーで返る。sidePanics なら、すぐに panic する。どちらでもなければ、ctx が終わるまで動く。
		sideFails  bool
		sidePanics bool
		// mainFails なら、主の Worker はすぐにエラーで返る。そうでなければ、ctx が終わるまで動く。
		mainFails bool
		wantErr   error
	}{
		{name: "横の Worker がエラーで止まっても、主の Worker は動き続ける", sideFails: true, sidePanics: false, mainFails: false, wantErr: context.Canceled},
		{name: "横の Worker が panic しても、主の Worker は動き続ける", sideFails: false, sidePanics: true, mainFails: false, wantErr: context.Canceled},
		{name: "主の Worker が返ったら、横の Worker も止める", sideFails: false, sidePanics: false, mainFails: true, wantErr: errMainFailed},
		{name: "親の ctx が終わったら、両方とも止まる", sideFails: false, sidePanics: false, mainFails: false, wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			sideReturned := make(chan struct{})
			side := fakeWorker(tt.sideFails, errSideFailed, tt.sidePanics, nil, sideReturned)
			mainStarted := make(chan struct{})
			main := fakeWorker(tt.mainFails, errMainFailed, false, mainStarted, nil)
			done := make(chan error, 1)
			// ログは、runAlongside が返る前に、横の Worker の goroutine で書き終える。
			var logs bytes.Buffer
			go func() { done <- cli.RunAlongside(ctx, slog.New(slog.NewTextHandler(&logs, nil)), main, side) }()
			<-mainStarted

			if tt.sideFails || tt.sidePanics {
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
			if tt.sidePanics {
				assert.Contains(t, logs.String(), "the side worker exploded")
				assert.Contains(t, logs.String(), "goroutine", "the stack is logged")
			}
		})
	}
}
