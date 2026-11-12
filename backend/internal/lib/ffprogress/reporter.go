package ffprogress

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// WriteInterval は、進み具合を書き戻す間隔 (書き戻しの間隔)。ジョブの画面は 5 秒ごとに読み直すので、それに合わせる。
	// -progress のまとまり (0.5 秒ごと) のたびに書くと、6 時間の変換で 4 万回を超えるが、5 秒なら 4,320 回で済む。
	WriteInterval = 5 * time.Second
	// LogInterval は、進み具合をログに出す間隔。詰まったときに見分けられれば足りるので、書き戻しより間を空ける。
	LogInterval = time.Minute
	// saveTimeout は、進み具合を 1 回書き戻すのにかける時間の上限。書き戻しは、FFmpeg の出力を読むのと同じゴルーチンで行うので、
	// DB が詰まっても、FFmpeg を長く止めないようにする。
	saveTimeout = 5 * time.Second
	bytesPerMB  = 1 << 20
)

// SaveFunc は、進み具合を書き戻す。
type SaveFunc func(ctx context.Context, s *Snapshot) error

// Reporter は、まとまりを Tracker で進み具合にし、WriteInterval 以上空けて書き戻し、LogInterval ごとにログに出す。
//
// ctx が終わった (ジョブを止めた) 後に届いたまとまりは読まない。止めてから FFmpeg が終わるまでに届くまとまり
// (止められた FFmpeg が書き出す progress=end など) で、止めたジョブの表示が 100% になったり戻ったりしないようにするためである。
type Reporter struct {
	tracker *Tracker
	clock   clock.Clock
	est     Estimate
	read    func() int64
	save    SaveFunc
	logger  *slog.Logger

	mu sync.Mutex
	// last は、最後に求めた進み具合。pending なら、まだ書き戻していない。
	last    Snapshot
	block   Block
	pending bool
	saved   time.Time
	logged  time.Time
}

// NewReporter は Reporter を返す。read は、それまでに FFmpeg に流した入力のバイト数を返す。分からなければ -1 を返す。
// save は、進み具合を書き戻す。logger には、進み具合を LogInterval ごとに出す。
func NewReporter(cfg *Config, est Estimate, clk clock.Clock, read func() int64, save SaveFunc, logger *slog.Logger) *Reporter {
	return &Reporter{
		tracker: NewTracker(cfg, est), clock: clk, est: est, read: read, save: save, logger: logger,
		mu: sync.Mutex{}, last: Snapshot{Ratio: 0, Remaining: nil}, block: emptyBlock(), pending: false,
		saved: time.Time{}, logged: time.Time{},
	}
}

// Observe は、まとまり b を読む。前に書き戻してから WriteInterval 以上たっていれば書き戻す。ctx が終わっていれば読まない。
func (r *Reporter) Observe(ctx context.Context, b *Block) {
	if ctx.Err() != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock.Now()
	r.last = r.tracker.Observe(now, b, r.read())
	r.block = *b
	r.pending = true
	if r.logged.IsZero() || now.Sub(r.logged) >= LogInterval {
		r.logged = now
		r.log(ctx, "FFmpeg progress")
	}
	if r.saved.IsZero() || now.Sub(r.saved) >= WriteInterval {
		r.flush(ctx, now)
	}
}

// Finish は、まだ書き戻していない進み具合を、間を空けずに書き戻す。FFmpeg が終わったときに呼ぶ。
// ジョブを止めた後にも呼ぶので、ctx が終わっていても書く。
func (r *Reporter) Finish(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.pending {
		return
	}
	r.log(ctx, "FFmpeg progress at the end")
	r.flush(ctx, r.clock.Now())
}

func (r *Reporter) flush(ctx context.Context, now time.Time) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
	defer cancel()
	if err := r.save(ctx, &r.last); err != nil {
		// 進み具合は画面に出すだけなので、書けなくても変換は続ける。
		r.logger.WarnContext(ctx, "failed to save the progress", slog.Any("error", err))
	}
	r.saved = now
	r.pending = false
}

// log は、コマ数 (済み/見込み)、読み位置、速さ、出力の大きさ、ビットレート、捨てたコマ数を出す。
// 割合には使わない読み位置も出すのは、詰まったときに、デマクサだけが先に進んでいるのか、本当に遅いのかを見分けるためである。
func (r *Reporter) log(ctx context.Context, msg string) {
	remaining := slog.String("remaining", "unknown")
	if r.last.Remaining != nil {
		remaining = slog.Duration("remaining", r.last.Remaining.Round(time.Second))
	}
	r.logger.InfoContext(ctx, msg,
		slog.Float64("ratio", r.last.Ratio), remaining,
		slog.Int64("frame", r.block.Frame), slog.Float64("expected_frames", r.est.Frames),
		slog.Float64("read_mb", float64(r.read())/bytesPerMB), slog.Float64("input_mb", float64(r.est.Bytes)/bytesPerMB),
		slog.String("speed", r.block.Speed), slog.Int64("total_size", r.block.TotalSize),
		slog.String("bitrate", r.block.Bitrate), slog.Int64("drop_frames", r.block.DropFrames))
}
