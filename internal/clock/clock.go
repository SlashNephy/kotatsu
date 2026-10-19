// Package clock は、現在時刻の取得を差し替えられるようにする。
// テストでは clocktest.Fake を渡し、時刻を進めて期限切れなどを再現する。
package clock

import "time"

// Clock は現在時刻を返す。
type Clock interface {
	Now() time.Time
	// StartStopwatch は、今から経過時間を測り始める。
	StartStopwatch() Stopwatch
}

// Stopwatch は、測り始めてからの経過時間を返す。壁時計が飛んでも (NTP が時計を戻したときなど)、経過時間は飛ばない。
type Stopwatch interface {
	Elapsed() time.Duration
}

// Real は実際の時刻を UTC で返す。
type Real struct{}

var _ Clock = new(Real)

func (Real) Now() time.Time {
	return time.Now().UTC()
}

// StartStopwatch は、単調時計で経過時間を測る Stopwatch を返す。
// Now は UTC にするときに単調時計の読みを落とすので、time.Now をそのまま持つ。
func (Real) StartStopwatch() Stopwatch {
	return realStopwatch{start: time.Now()}
}

type realStopwatch struct{ start time.Time }

var _ Stopwatch = new(realStopwatch)

func (s realStopwatch) Elapsed() time.Duration {
	return time.Since(s.start)
}
