package mirakurun

import (
	"errors"
	"io"
	"sync/atomic"
	"time"
)

// DefaultStreamIdleTimeout は、ストリームのデータが届かないまま待つ時間の上限 (#537)。
// 放送のストリームには、PAT などが 1 秒に何度も入るので、これだけ届かなければ止まったとみなす。
const DefaultStreamIdleTimeout = 10 * time.Second

// ErrStreamStalled は、ストリームが、接続を保ったまま、データを送らなくなったこと。
var ErrStreamStalled = errors.New("the tuner server stopped sending the stream")

// idleReader は、データが timeout の間届かなければ、ストリームを閉じる。
// 閉じると、止まっている Read も抜け、ErrStreamStalled を返す。録画は、ほかの切断と同じく開き直す (設計書 5.2)。
type idleReader struct {
	body    io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	stalled atomic.Bool
}

var _ io.ReadCloser = new(idleReader)

func newIdleReader(body io.ReadCloser, timeout time.Duration) *idleReader {
	r := &idleReader{body: body, timeout: timeout, timer: nil, stalled: atomic.Bool{}}
	r.timer = time.AfterFunc(timeout, func() {
		r.stalled.Store(true)
		_ = body.Close()
	})

	return r
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 && !r.stalled.Load() {
		r.timer.Reset(r.timeout)
	}
	if err != nil && r.stalled.Load() {
		return n, ErrStreamStalled
	}

	return n, err
}

func (r *idleReader) Close() error {
	r.timer.Stop()

	return r.body.Close()
}
