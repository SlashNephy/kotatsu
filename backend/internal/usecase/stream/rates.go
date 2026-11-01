package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/SlashNephy/kotatsu/internal/infra/mirakurun"
	"github.com/SlashNephy/kotatsu/internal/lib/clock"
	"github.com/SlashNephy/kotatsu/internal/lib/ts"
)

const (
	// ratesPriority は、内訳を数えるためにサービスのストリームを開くときの優先度 (#812)。
	// 負なので、ほかの利用者からチューナーを奪わない。見ている人がいれば、同じチューナーに相乗りする。
	ratesPriority = -1
	// defaultRatesInterval は、内訳を送る間隔。
	defaultRatesInterval = time.Second
)

// ServiceOpener は、サービスのストリームを開く。*mirakurun.Client が満たす。
type ServiceOpener interface {
	OpenServiceStream(ctx context.Context, serviceID int64, opts *mirakurun.StreamOptions) (io.ReadCloser, error)
}

var _ ServiceOpener = new(mirakurun.Client)

// KindBytes は、PID の種類ごとのバイト数。
type KindBytes struct {
	Video int64
	Audio int64
	// Other は、字幕、文字スーパー、データ放送など、映像と音声のほかの ES。
	Other int64
	// PSI は、PAT、PMT と、決まった PID で送る SI。
	PSI int64
	// Unknown は、PMT に載っていない PID (ECM、EMM など)。
	Unknown int64
}

// LiveRates は、ライブのサービスの、種類ごとのビットレートと、ドロップ、エラー、スクランブルの数。
type LiveRates struct {
	// BPS は、直前の間隔の、種類ごとのビットレート (bps)。
	BPS KindBytes
	// TotalBPS は、直前の間隔の、全体のビットレート (bps)。
	TotalBPS int64
	// Drops、Errors、Scrambled は、数え始めてからの合計。
	Drops     int64
	Errors    int64
	Scrambled int64
}

// kindBytes は、解析の統計を、PID の種類ごとのバイト数にまとめる。
func kindBytes(stats *ts.Stats) KindBytes {
	var sum KindBytes
	for _, q := range ts.PIDQualities(stats, stats.PIDs) {
		bytes := q.Packets * ts.PacketSize
		switch q.Kind {
		case ts.PIDKindVideo:
			sum.Video += bytes
		case ts.PIDKindAudio:
			sum.Audio += bytes
		case ts.PIDKindOther:
			sum.Other += bytes
		case ts.PIDKindPSI:
			sum.PSI += bytes
		default:
			sum.Unknown += bytes
		}
	}

	return sum
}

// liveRates は、前の統計 previous から今の統計 current までの elapsed の間の、種類ごとのビットレートを返す。
func liveRates(previous, current *ts.Stats, elapsed time.Duration) *LiveRates {
	before, after := kindBytes(previous), kindBytes(current)
	bps := func(from, to int64) int64 {
		if elapsed <= 0 {
			return 0
		}

		return (to - from) * 8 * int64(time.Second) / int64(elapsed)
	}
	rates := &LiveRates{
		BPS: KindBytes{
			Video:   bps(before.Video, after.Video),
			Audio:   bps(before.Audio, after.Audio),
			Other:   bps(before.Other, after.Other),
			PSI:     bps(before.PSI, after.PSI),
			Unknown: bps(before.Unknown, after.Unknown),
		},
		TotalBPS:  bps(previous.Packets*ts.PacketSize, current.Packets*ts.PacketSize),
		Drops:     0,
		Errors:    0,
		Scrambled: 0,
	}
	for _, pid := range current.PIDs {
		rates.Drops += pid.Drops
		rates.Errors += pid.Errors
		rates.Scrambled += pid.Scrambled
	}

	return rates
}

// LiveRatesWatcher は、ライブのサービスのストリームを tenban の中で開いて数え、種類ごとのビットレートを送る (#812)。
// ブラウザに届く TS を数えるのではないので、ブラウザ内での変換でも kyusu の経路でも、放送の内訳を出せる。
type LiveRatesWatcher struct {
	opener   ServiceOpener
	clock    clock.Clock
	interval time.Duration
}

// NewLiveRatesWatcher は LiveRatesWatcher を返す。opener が nil なら、Watch は失敗する。interval が 0 なら 1 秒にする。
func NewLiveRatesWatcher(opener ServiceOpener, clk clock.Clock, interval time.Duration) *LiveRatesWatcher {
	if interval <= 0 {
		interval = defaultRatesInterval
	}

	return &LiveRatesWatcher{opener: opener, clock: clk, interval: interval}
}

// ErrNoTuner は、チューナーサーバーを設定していないときのエラー。
var ErrNoTuner = errors.New("tuner server is not configured")

// Watch は、サービス serviceID のストリームを開き、interval ごとに内訳を送る。ctx が終わるか、ストリームが切れたら、チャネルを閉じる。
func (w *LiveRatesWatcher) Watch(ctx context.Context, serviceID int64) (<-chan *LiveRates, error) {
	if w.opener == nil {
		return nil, ErrNoTuner
	}
	upstream, err := w.opener.OpenServiceStream(ctx, serviceID, &mirakurun.StreamOptions{Decode: true, Priority: ratesPriority})
	if err != nil {
		return nil, fmt.Errorf("failed to open the service stream: %w", err)
	}
	analyzer := ts.NewAnalyzer(nil)
	// 最初の間隔の起点は、読み始める前に取る。読む goroutine が先に TS を読み切ると、間隔ごとの差がすべて 0 になる (#1187)。
	previous := analyzer.Stats()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(analyzer, upstream)
	}()
	out := make(chan *LiveRates, 1)
	go func() {
		defer close(out)
		defer func() {
			_ = upstream.Close()
		}()
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		stopwatch := w.clock.StartStopwatch()
		var last time.Duration
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
			}
			current := analyzer.Stats()
			now := stopwatch.Elapsed()
			rates := liveRates(previous, current, now-last)
			previous, last = current, now
			// 受け取る側が遅れていれば、古い値を捨て、今の値だけを残す。
			select {
			case out <- rates:
			default:
				select {
				case <-out:
				default:
				}
				out <- rates
			}
		}
	}()

	return out, nil
}
