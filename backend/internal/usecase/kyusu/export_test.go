package kyusu

import "time"

// Reap は reap をテストから呼ぶ。
func (l *Live) Reap(now time.Time) {
	l.reap(now)
}

// Reap は reap をテストから呼ぶ。
func (v *VOD) Reap(now time.Time) {
	v.reap(now)
}
