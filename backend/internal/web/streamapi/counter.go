package streamapi

import (
	"net/http"
	"sync/atomic"
)

// StreamKind は、tenban が変換せずに流す配信の種類 (#959)。
type StreamKind string

const (
	// StreamLive は、ライブ視聴の中継。
	StreamLive StreamKind = "live"
	// StreamDataBroadcast は、ライブ視聴の間に並べて開く、データ放送の描画に要るパケットだけのストリーム。
	// ライブ視聴 1 本ごとに開くので、live に入れると、1 つの視聴を 2 本と数えてしまう。
	StreamDataBroadcast StreamKind = "data_broadcast"
	// StreamRecording は、録画の再生と、取り込んだ録画の TS 以外のファイル。
	StreamRecording StreamKind = "recording"
)

// StreamKinds は、配信の種類をすべて返す。
func StreamKinds() []StreamKind {
	return []StreamKind{StreamLive, StreamDataBroadcast, StreamRecording}
}

// StreamCounter は、このインスタンスで流している最中の応答を、種類ごとに数える (設計書 11.3)。
// 見ている人の数ではない。録画の再生では、プレイヤーが Range で取り直すたびに、別の応答になる。
type StreamCounter struct {
	counts map[StreamKind]*atomic.Int64
}

// NewStreamCounter は StreamCounter を返す。
func NewStreamCounter() *StreamCounter {
	counts := make(map[StreamKind]*atomic.Int64, len(StreamKinds()))
	for _, kind := range StreamKinds() {
		counts[kind] = new(atomic.Int64)
	}

	return &StreamCounter{counts: counts}
}

// Count は、next の応答を、流している間だけ kind の配信として数えるハンドラーを返す。
// HEAD は、ストリームを開かずにヘッダーだけを返すので数えない。
func (c *StreamCounter) Count(kind StreamKind, next http.Handler) http.Handler {
	count := c.counts[kind]

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			count.Add(1)
			defer count.Add(-1)
		}
		next.ServeHTTP(w, r)
	})
}

// Streams は、kind の配信を流している最中の応答の数を返す。
func (c *StreamCounter) Streams(kind StreamKind) int {
	return int(c.counts[kind].Load())
}
