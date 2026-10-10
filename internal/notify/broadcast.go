package notify

import (
	"context"
	"sync"
)

// Broadcaster は、1 つの Watcher が知った変更を、複数の購読者に知らせる。
// GraphQL の subscription のように、購読者が出入りするものに使う。購読者ごとに LISTEN の接続を持たずに済む。
//
// 知らせるのは「どれかが変わったかもしれない」ことだけで、購読者は Version で版数を読み、自分が最後に見た版数と比べる。
// 版数は Watcher が持つので、購読者が遅れて、知らせが 1 回にまとまっても、変更を取りこぼさない。
type Broadcaster struct {
	watcher *Watcher

	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

// NewBroadcaster は、watcher の変更を知らせる Broadcaster を返す。
func NewBroadcaster(watcher *Watcher) *Broadcaster {
	return &Broadcaster{watcher: watcher, mu: sync.Mutex{}, subscribers: map[chan struct{}]struct{}{}}
}

// Run は、ctx が終わるまで、Watcher を動かし、変更を購読者に知らせる。
func (b *Broadcaster) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- b.watcher.Run(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			return <-watchErr
		case err := <-watchErr:
			return err
		case <-b.watcher.Changes():
			b.wake()
		}
	}
}

// Subscribe は、変更があると値を受け取れるチャンネルと、購読をやめる関数を返す。
// 受け取る前に何度変更があっても、受け取れるのは 1 回にまとまる。受け取ったら Version で版数を確かめる。
func (b *Broadcaster) Subscribe() (<-chan struct{}, func()) {
	wake := make(chan struct{}, 1)
	b.mu.Lock()
	b.subscribers[wake] = struct{}{}
	b.mu.Unlock()

	return wake, func() {
		b.mu.Lock()
		delete(b.subscribers, wake)
		b.mu.Unlock()
	}
}

// Version は、channel について最後に確かめた版数を返す。一度も変更のないチャンネルは 0 を返す。
func (b *Broadcaster) Version(channel string) int64 {
	return b.watcher.Version(channel)
}

// wake は、購読者に知らせる。遅い購読者を待たないよう、まだ受け取られていない知らせがあれば、それにまとめる。
func (b *Broadcaster) wake() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for wake := range b.subscribers {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}
