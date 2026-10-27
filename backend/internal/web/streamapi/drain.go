package streamapi

import (
	"context"
	"sync"
	"time"
)

const (
	// DrainTimeout は、tenban を止める前に、視聴者がつなぎ直すのを待つ時間の上限 (設計書 11.4、#1085)。
	// 過ぎたら、つなぎ直していない視聴者のストリームも、今までどおり打ち切る。
	DrainTimeout = 20 * time.Second
	// drainPollInterval は、つなぎ直しを待つ間に、流している最中の応答の数を確かめる間隔。
	drainPollInterval = 200 * time.Millisecond
)

// Drainer は、tenban を止める前に、流している視聴者に、新しい接続を張ってから切り替えるよう知らせる (#1085)。
// 知らせは、GraphQL の viewerEvents で、この tenban に購読している視聴者に届く。
type Drainer struct {
	streams  *StreamCounter
	draining chan struct{}
	start    func()
}

// NewDrainer は、streams の応答を待つ Drainer を返す。
func NewDrainer(streams *StreamCounter) *Drainer {
	draining := make(chan struct{})

	return &Drainer{streams: streams, draining: draining, start: sync.OnceFunc(func() { close(draining) })}
}

// Draining は、止める前の知らせを始めたら閉じるチャネルを返す。
func (d *Drainer) Draining() <-chan struct{} {
	return d.draining
}

// Drain は、知らせを始め、ライブ視聴と録画のストリームがなくなるか、timeout が過ぎるか、ctx が終わるまで待つ。
// データ放送のストリームは、ライブ視聴をつなぎ直しても張り直さないので、待たない。
func (d *Drainer) Drain(ctx context.Context, timeout time.Duration) {
	d.start()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(drainPollInterval)
	defer ticker.Stop()
	for d.streams.Streams(StreamLive)+d.streams.Streams(StreamRecording) > 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
