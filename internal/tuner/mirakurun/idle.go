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

// DefaultStreamOpenTimeout は、ストリームを開くときに、応答の頭を待つ時間の上限 (#412)。
// チューナーサーバーは、チューナーの空きを待つ間、応答を返さないことがある。待ち続けると、予定の終わりを過ぎても録画を抜けない。
const DefaultStreamOpenTimeout = 30 * time.Second

// ErrStreamStalled は、ストリームが、接続を保ったまま、データを送らなくなったこと。
var ErrStreamStalled = errors.New("the tuner server stopped sending the stream")

// ErrStreamOpenTimeout は、ストリームを開くときに、チューナーサーバーが上限の時間までに応答しなかったこと。
var ErrStreamOpenTimeout = errors.New("the tuner server did not respond to the stream request in time")

// idleReader は、データが timeout の間届かなければ、ストリームを閉じる。
// 閉じると、止まっている Read も抜け、ErrStreamStalled を返す。録画は、ほかの切断と同じく開き直す (設計書 5.2)。
type idleReader struct {
	body    io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	stalled atomic.Bool
	// release は、閉じたときに、ストリームの要求の context を手放す。
	release func()
}

var _ io.ReadCloser = new(idleReader)

func newIdleReader(body io.ReadCloser, timeout time.Duration, release func()) *idleReader {
	r := &idleReader{body: body, timeout: timeout, timer: nil, stalled: atomic.Bool{}, release: release}
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
	if err != nil {
		if r.stalled.Load() {
			return n, ErrStreamStalled
		}
		// 読み終えたか、ほかの理由で切れたら、見張るのをやめる。閉じるのが遅れても、止まったことにしない。
		r.timer.Stop()
	}

	return n, err
}

func (r *idleReader) Close() error {
	r.timer.Stop()
	defer r.release()

	return r.body.Close()
}
