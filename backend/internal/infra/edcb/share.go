package edcb

import (
	"context"
	"errors"
	"io"
	"sync"
)

const (
	// DefaultSubscriberBufferSize は、分け合うストリームの受け手ごとに溜められるバイト数の既定 (#1535)。
	// BS4K (毎秒 4 MB ほど) を受け取る録画が、ディスクの書き込みで数秒詰まっても切られないよう、多めにする。
	DefaultSubscriberBufferSize = 32 << 20
	// tsPacketSize は、TS のパケットの大きさ。受け手に配るときは、途中から加わった受け手もパケットの区切りから読めるよう、
	// この大きさの倍数で区切る。
	tsPacketSize = 188
	// shareReadSize は、NetworkTV の TS を 1 回に読む大きさ。
	shareReadSize = tsPacketSize * 348
)

// ErrSubscriberTooSlow は、分け合うストリームの受け手が読むのが遅く、溜められる上限を超えたので切ったことを表す (#1535)。
// ほかの受け手は待たせずに受け取り続ける。切られた受け手は、つなぎ直せる。
var ErrSubscriberTooSlow = errors.New("the stream reader was too slow and was cut off")

// errSubscriberClosed は、閉じた受け手を読んだことを表す。
var errSubscriberClosed = errors.New("the shared stream is closed")

// sharedStreams は、サービスごとに 1 本の NetworkTV を開き、TS をそのサービスの受け手に配る (#1535)。
// EDCB は、同じ局でも NetworkTV ごとにチューナーを使い、1 本の TS を受け取れるのも 2 つまでなので、Kotatsu の中で分け合う。
//   - 最初の受け手が来たら開き、最後の受け手が去ったら閉じる (NWTV_ID_CLOSE と ID の返却)。
//   - 開いている途中に来た受け手は、その開きを待つ。閉じている途中に来た受け手は、閉じ終えるのを待ってから開き直す。
//     同じサービスに 2 つの NetworkTV を開かない。
//   - NetworkTV の TS は、受け手ではなく、ここが持つ context で開く。受け手の context が終わっても、その受け手が去るだけになる。
//   - 局が変わったときの切断 (#1534) は、1 本の TS で 1 度だけ判定し、すべての受け手に同じエラーで知らせる。
type sharedStreams struct {
	server     *TunerServer
	bufferSize int

	mu      sync.Mutex
	streams map[ServiceKey]*broadcast
}

func newSharedStreams(server *TunerServer, bufferSize int) *sharedStreams {
	return &sharedStreams{server: server, bufferSize: bufferSize, mu: sync.Mutex{}, streams: map[ServiceKey]*broadcast{}}
}

// broadcast は、1 つのサービスの NetworkTV の TS と、その受け手。フィールドは sharedStreams の mu で守る。
type broadcast struct {
	key ServiceKey
	// ready は、開き終えたら (失敗しても) 閉じる。openErr は、開けなかった理由。
	ready   chan struct{}
	openErr error
	// upstream は、開いた NetworkTV の TS。
	upstream io.ReadCloser
	// cancel は、upstream を開いた context を止める。
	cancel context.CancelFunc
	subs   map[*subscriber]struct{}
	// closing は、閉じ始めたかどうか。閉じ始めたら、新しい受け手を加えない。
	closing bool
	// done は、NetworkTV を閉じて ID を返し終えたら閉じる。
	done chan struct{}
}

// open は、service の TS を受け取る受け手を返す。開いていなければ開く。
func (s *sharedStreams) open(ctx context.Context, service ServiceKey) (io.ReadCloser, error) {
	sub := &subscriber{
		shared: s, b: nil, limit: s.bufferSize,
		mu: sync.Mutex{}, signal: make(chan struct{}, 1), queue: [][]byte{}, head: nil, queued: 0, err: nil, closed: false,
	}
	s.mu.Lock()
	for {
		b := s.streams[service]
		if b == nil {
			b = &broadcast{
				key: service, ready: make(chan struct{}), openErr: nil, upstream: nil, cancel: func() {},
				subs: map[*subscriber]struct{}{sub: {}}, closing: false, done: make(chan struct{}),
			}
			sub.b = b
			s.streams[service] = b
			s.mu.Unlock()
			// 受け手の context の値 (トレースなど) は引き継ぎ、止まることは引き継がない。
			go s.start(context.WithoutCancel(ctx), b)

			break
		}
		if !b.closing {
			b.subs[sub] = struct{}{}
			sub.b = b
			s.mu.Unlock()

			break
		}
		// 閉じている途中なら、閉じ終えて ID を返すのを待ってから開き直す。
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-b.done:
		}
		s.mu.Lock()
	}

	select {
	case <-ctx.Done():
		_ = sub.Close()

		return nil, ctx.Err()
	case <-sub.b.ready:
	}
	if err := sub.b.openErr; err != nil {
		return nil, err
	}

	return sub, nil
}

// start は、b の NetworkTV を開き、TS を受け手に配り始める。開いている間に受け手がいなくなったら、閉じる。
// 開くのは受け手の context ではなく、b が持つ context で行う。最初の受け手が待ちきれずに去っても、ほかの受け手の開きを止めない。
// 開く処理の時間は、CtrlCmd と中継のやり直しの上限 (RequestTimeout、relayRetryTimeout) で限られる。
func (s *sharedStreams) start(base context.Context, b *broadcast) {
	ctx, cancel := context.WithCancel(base)
	upstream, err := s.server.openNetworkTV(ctx, b.key)

	s.mu.Lock()
	if err != nil {
		cancel()
		b.openErr, b.closing = err, true
		delete(s.streams, b.key)
		close(b.ready)
		s.mu.Unlock()
		close(b.done)

		return
	}
	b.upstream, b.cancel = upstream, cancel
	close(b.ready)
	if len(b.subs) == 0 {
		// 開いている間に、受け手が全員去った。
		b.closing = true
		delete(s.streams, b.key)
		s.mu.Unlock()
		_ = s.shutdown(b)

		return
	}
	s.mu.Unlock()
	go s.pump(b)
}

// pump は、NetworkTV の TS を読み、パケットの区切りで受け手に配る。TS が終わるか切れたら、NetworkTV を閉じ、
// 受け手に、受け取った分を読み終えた後で同じエラーを返させる。
func (s *sharedStreams) pump(b *broadcast) {
	buf := make([]byte, shareReadSize)
	rest := 0
	for {
		n, err := b.upstream.Read(buf[rest:])
		total := rest + n
		if aligned := total - total%tsPacketSize; aligned > 0 {
			chunk := make([]byte, aligned)
			copy(chunk, buf[:aligned])
			s.publish(b, chunk)
			rest = copy(buf, buf[aligned:total])
		} else {
			rest = total
		}
		if err == nil {
			continue
		}

		s.mu.Lock()
		if b.closing {
			// 最後の受け手が去って閉じたので、読めなくなった。
			s.mu.Unlock()

			return
		}
		b.closing = true
		delete(s.streams, b.key)
		subs := make([]*subscriber, 0, len(b.subs))
		for sub := range b.subs {
			subs = append(subs, sub)
		}
		b.subs = map[*subscriber]struct{}{}
		s.mu.Unlock()
		// 受け手がつなぎ直したときに、閉じていない NetworkTV と ID が残っていないよう、閉じてから知らせる。
		_ = s.shutdown(b)
		for _, sub := range subs {
			sub.end(err)
		}

		return
	}
}

// publish は、chunk を b の受け手に配る。溜めた量が上限を超える受け手は、ほかの受け手を待たせないよう切る。
func (s *sharedStreams) publish(b *broadcast, chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range b.subs {
		if !sub.push(chunk) {
			delete(b.subs, sub)
		}
	}
	if len(b.subs) == 0 && !b.closing {
		b.closing = true
		delete(s.streams, b.key)
		go func() { _ = s.shutdown(b) }()
	}
}

// leave は、sub を b の受け手から外す。最後の受け手なら、NetworkTV を閉じて ID を返す。
func (s *sharedStreams) leave(b *broadcast, sub *subscriber) error {
	s.mu.Lock()
	delete(b.subs, sub)
	opened := b.upstream != nil
	last := len(b.subs) == 0 && opened && !b.closing
	if last {
		b.closing = true
		delete(s.streams, b.key)
	}
	s.mu.Unlock()
	if !last {
		return nil
	}

	return s.shutdown(b)
}

// shutdown は、NetworkTV を閉じて ID を返し、閉じ終えたことを知らせる。
func (s *sharedStreams) shutdown(b *broadcast) error {
	err := b.upstream.Close()
	b.cancel()
	close(b.done)

	return err
}

// subscriber は、分け合うストリームの 1 つの受け手。受け取った TS を、上限まで溜めて、Read で返す。
type subscriber struct {
	shared *sharedStreams
	b      *broadcast
	limit  int

	mu sync.Mutex
	// signal は、データが届いたか、終わったことを、待っている Read に知らせる。
	signal chan struct{}
	queue  [][]byte
	// head は、読みかけのチャンクの残り。
	head   []byte
	queued int
	// err は、溜めた分を読み終えた後に返すエラー。
	err    error
	closed bool
}

var _ io.ReadCloser = new(subscriber)

// Read は、溜めた TS を読む。溜めた分がなければ、届くか終わるまで待つ。
func (r *subscriber) Read(p []byte) (int, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()

			return 0, errSubscriberClosed
		}
		if len(r.head) == 0 && len(r.queue) > 0 {
			r.head, r.queue = r.queue[0], r.queue[1:]
		}
		if len(r.head) > 0 {
			n := copy(p, r.head)
			r.head = r.head[n:]
			r.queued -= n
			r.mu.Unlock()

			return n, nil
		}
		if r.err != nil {
			err := r.err
			r.mu.Unlock()

			return 0, err
		}
		r.mu.Unlock()
		<-r.signal
	}
}

// Close は、受け手から外れる。最後の受け手なら、NetworkTV を閉じて ID を返し、閉じられなかったらそのエラーを返す。何度呼んでもよい。
func (r *subscriber) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()

		return nil
	}
	r.closed = true
	r.queue, r.head, r.queued = nil, nil, 0
	r.notify()
	r.mu.Unlock()

	return r.shared.leave(r.b, r)
}

// push は、chunk を溜める。上限を超えるなら、溜めた分を捨てて ErrSubscriberTooSlow で終わらせ、false を返す。
func (r *subscriber) push(chunk []byte) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.err != nil {
		return false
	}
	if r.queued+len(chunk) > r.limit {
		r.queue, r.head, r.queued, r.err = nil, nil, 0, ErrSubscriberTooSlow
		r.notify()

		return false
	}
	r.queue = append(r.queue, chunk)
	r.queued += len(chunk)
	r.notify()

	return true
}

// end は、溜めた分を読み終えた後に err を返すようにする。
func (r *subscriber) end(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		r.err = err
	}
	r.notify()
}

func (r *subscriber) notify() {
	select {
	case r.signal <- struct{}{}:
	default:
	}
}
