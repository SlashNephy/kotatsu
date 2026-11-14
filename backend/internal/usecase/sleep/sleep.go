// Package sleep は、録画、ライブ視聴、エンコードの間と次の録画の直前は OS をスリープさせず、
// 次の録画の少し前に OS をスリープから起こす予約を置く (#1522、設計書 11.7)。
//
// 判定の間隔ごとに、DB とこのプロセスの値から判定し、変わったときだけ OS に伝える。
// OS に伝える口 (System) は、Windows と macOS だけにある。all の構成のときだけ動かす。
package sleep

import (
	"context"
	"log/slog"
	"time"

	"github.com/SlashNephy/kotatsu/internal/lib/clock"
)

const (
	// DefaultInterval は、スリープさせないかを判定する間隔 (付録 A)。
	// 起こす前倒しより十分短く、Windows の無人のスリープ解除のタイムアウト (既定で約 2 分) より短くする。
	// 起きてから眠り直すまでに、必ず 1 回以上判定して抑止を取るためである。
	DefaultInterval = 30 * time.Second
	// DefaultWakeLead は、次の録画の開始の何分前に、OS に起こさせるか (付録 A)。
	// 起きてから録画を始められるまでの時間は、実機で測っていない。チューナーとディスクが戻るのを待てるよう、長めにする。
	DefaultWakeLead = 5 * time.Minute
	// DefaultInhibitLead は、次の録画の開始の何分前から、スリープさせないか (付録 A)。
	// 起きた時点で、もう抑止の範囲に入っているよう、起こす前倒しより長くする。
	DefaultInhibitLead = 10 * time.Minute
	// readTimeout は、1 回の判定で DB を読む時間の上限。DB が固まっても、止めるときに抑止を外せるようにする。
	readTimeout = 10 * time.Second
)

// スリープさせない理由。ログに出す。
const (
	ReasonRecording   = "recording"
	ReasonEncoding    = "encoding"
	ReasonWatching    = "watching live"
	ReasonDue         = "recording is due"
	ReasonNextIsClose = "next recording is near"
)

//go:generate go tool mockgen -typed -destination=mock_system_test.go -package=sleep_test . System

// System は、OS にスリープの抑止と起こす予約を伝える口。
type System interface {
	// SetInhibit は、操作の無いことで OS が眠るのを止めるか (on)、止めるのをやめるか。
	SetInhibit(on bool) error
	// ScheduleWake は、at に OS をスリープから起こす予約を置く。前に置いた予約は置き換える。
	ScheduleWake(at time.Time) error
	// CancelWake は、置いた予約を消す。前のプロセスが置いたまま残した予約も消す。予約がなければ何もしない。
	CancelWake() error
}

// Activity は、判定に使う、今の録画、エンコード、視聴と、次の録画の予定。
type Activity struct {
	// Recordings は、mikan が録っている、録画とタイムシフトの割り振りの数。
	Recordings int64
	// Encodings は、動いている後処理のジョブ (録画の変換、CM の区間の見分け、チャプターの書き込み) の数。
	Encodings int64
	// Viewers は、このプロセスが流しているライブの配信と、ライブのトランスコードのセッションの数。録画の再生は入れない。
	Viewers int64
	// Airing は、録画する予定の時刻に入っていて、まだ終わっていない予約の数。割り振りを mikan が取る前の予約を拾う。
	Airing int64
	// NextStart は、今より後に始まる次の予約の、録画する予定の開始 (マージン込み)。なければゼロ。
	NextStart time.Time
}

//go:generate go tool mockgen -typed -destination=mock_source_test.go -package=sleep_test . Source

// Source は、判定に使う Activity を読む。
type Source interface {
	Activity(ctx context.Context, now time.Time) (*Activity, error)
}

// Options は Controller の設定。0 の項目は既定値にする。
type Options struct {
	Interval    time.Duration
	WakeLead    time.Duration
	InhibitLead time.Duration
}

func (o *Options) withDefaults() *Options {
	out := Options{Interval: DefaultInterval, WakeLead: DefaultWakeLead, InhibitLead: DefaultInhibitLead}
	if o == nil {
		return &out
	}
	if o.Interval > 0 {
		out.Interval = o.Interval
	}
	if o.WakeLead > 0 {
		out.WakeLead = o.WakeLead
	}
	if o.InhibitLead > 0 {
		out.InhibitLead = o.InhibitLead
	}

	return &out
}

// Decision は、判定の結果。
type Decision struct {
	// Inhibit は、スリープさせないかどうか。
	Inhibit bool
	// Reasons は、スリープさせない理由。ログに出す。
	Reasons []string
	// WakeAt は、起こす予約の時刻。置かないならゼロ。
	WakeAt time.Time
}

// Decide は、activity と now から、スリープさせないかと、起こす予約の時刻を決める。
//
// 起こす時刻は、次の録画の開始から、起こす前倒しだけ前にし、秒に切り捨てる。macOS の予約は、置いたときと同じ時刻でないと取り消せないからである。
// 時刻を過ぎていれば置かない。もう起きていて、抑止の前倒しからの抑止が効いているからである。
func Decide(activity *Activity, now time.Time, opts *Options) *Decision {
	opts = opts.withDefaults()
	d := &Decision{Inhibit: false, Reasons: []string{}, WakeAt: time.Time{}}
	if activity.Recordings > 0 {
		d.Reasons = append(d.Reasons, ReasonRecording)
	}
	if activity.Encodings > 0 {
		d.Reasons = append(d.Reasons, ReasonEncoding)
	}
	if activity.Viewers > 0 {
		d.Reasons = append(d.Reasons, ReasonWatching)
	}
	if activity.Airing > 0 {
		d.Reasons = append(d.Reasons, ReasonDue)
	}
	if !activity.NextStart.IsZero() {
		if activity.NextStart.Sub(now) < opts.InhibitLead {
			d.Reasons = append(d.Reasons, ReasonNextIsClose)
		}
		if wakeAt := activity.NextStart.Add(-opts.WakeLead).Truncate(time.Second); wakeAt.After(now) {
			d.WakeAt = wakeAt
		}
	}
	d.Inhibit = len(d.Reasons) > 0

	return d
}

// Controller は、判定の間隔ごとに判定し、変わったときだけ System に伝える。
type Controller struct {
	source Source
	system System
	clock  clock.Clock
	logger *slog.Logger
	opts   *Options

	// inhibiting は、OS に抑止を伝えてあるかどうか。
	inhibiting bool
	// wakeAt は、OS に置いた起こす予約の時刻。置いていなければゼロ。
	wakeAt time.Time
	// 続けて失敗している間は、警告を 1 回だけ出す。成功したら戻す。
	readWarned, inhibitWarned, wakeWarned bool
}

// NewController は Controller を返す。opts は nil でもよい。
func NewController(source Source, system System, clk clock.Clock, logger *slog.Logger, opts *Options) *Controller {
	return &Controller{
		source: source, system: system, clock: clk, logger: logger, opts: opts.withDefaults(),
		inhibiting: false, wakeAt: time.Time{}, readWarned: false, inhibitWarned: false, wakeWarned: false,
	}
}

// Run は、ctx が終わるまで判定を続ける。終わるときは、抑止を外し、起こす予約を消す。
// Kotatsu が動いていなければ、起こしても録画は始まらないからである。
func (c *Controller) Run(ctx context.Context) error {
	c.logger.InfoContext(ctx, "sleep control started",
		slog.Duration("interval", c.opts.Interval), slog.Duration("wake_lead", c.opts.WakeLead), slog.Duration("inhibit_lead", c.opts.InhibitLead))
	// 前のプロセスが止まる前に消せなかった予約を、置き直す前に消す。
	if err := c.system.CancelWake(); err != nil {
		c.logger.WarnContext(ctx, "failed to cancel the wake scheduled before the start", slog.Any("error", err))
	}
	ticker := time.NewTicker(c.opts.Interval)
	defer ticker.Stop()
	for {
		c.Step(ctx)
		select {
		case <-ctx.Done():
			c.Stop(context.WithoutCancel(ctx))

			return nil
		case <-ticker.C:
		}
	}
}

// Step は、1 回判定し、変わったことを System に伝える。伝えられなければ、状態を変えずに警告し、次の判定でもう一度試す。
func (c *Controller) Step(ctx context.Context) {
	now := c.clock.Now()
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	activity, err := c.source.Activity(readCtx, now)
	cancel()
	if err != nil {
		// 読めなければ、今の状態を保つ。DB が止まっても、mikan は録り続けるからである (#1073)。
		if !c.readWarned {
			c.logger.WarnContext(ctx, "failed to read the activity for the sleep control; keeping the current state", slog.Any("error", err))
			c.readWarned = true
		}

		return
	}
	c.readWarned = false
	d := Decide(activity, now, c.opts)
	c.applyInhibit(ctx, d)
	// 置いた予約の時刻を過ぎていれば、OS は予約を使い終えているので、消さずに忘れる。
	if !c.wakeAt.IsZero() && !c.wakeAt.After(now) {
		c.wakeAt = time.Time{}
	}
	c.applyWake(ctx, d.WakeAt)
}

// Stop は、抑止を外し、起こす予約を消す。
func (c *Controller) Stop(ctx context.Context) {
	if c.inhibiting {
		if err := c.system.SetInhibit(false); err != nil {
			c.logger.WarnContext(ctx, "failed to release the sleep inhibition", slog.Any("error", err))
		}
		c.inhibiting = false
	}
	if err := c.system.CancelWake(); err != nil {
		c.logger.WarnContext(ctx, "failed to cancel the wake", slog.Any("error", err))
	}
	c.wakeAt = time.Time{}
	c.logger.InfoContext(ctx, "sleep control stopped")
}

func (c *Controller) applyInhibit(ctx context.Context, d *Decision) {
	if d.Inhibit == c.inhibiting {
		return
	}
	if err := c.system.SetInhibit(d.Inhibit); err != nil {
		if !c.inhibitWarned {
			c.logger.WarnContext(ctx, "failed to change the sleep inhibition; retrying at the next check",
				slog.Bool("inhibit", d.Inhibit), slog.Any("error", err))
			c.inhibitWarned = true
		}

		return
	}
	c.inhibitWarned = false
	c.inhibiting = d.Inhibit
	if d.Inhibit {
		c.logger.InfoContext(ctx, "preventing the system from sleeping", slog.Any("reasons", d.Reasons))
	} else {
		c.logger.InfoContext(ctx, "allowing the system to sleep")
	}
}

func (c *Controller) applyWake(ctx context.Context, at time.Time) {
	if at.Equal(c.wakeAt) {
		return
	}
	var err error
	if at.IsZero() {
		err = c.system.CancelWake()
	} else {
		err = c.system.ScheduleWake(at)
	}
	if err != nil {
		if !c.wakeWarned {
			c.logger.WarnContext(ctx, "failed to schedule the wake; retrying at the next check", slog.Time("at", at), slog.Any("error", err))
			c.wakeWarned = true
		}

		return
	}
	c.wakeWarned = false
	c.wakeAt = at
	if at.IsZero() {
		c.logger.InfoContext(ctx, "cancelled the wake")
	} else {
		c.logger.InfoContext(ctx, "scheduled a wake before the next recording", slog.Time("at", at))
	}
}
