package kyusu

import "time"

// Reap は reap をテストから呼ぶ。
func (l *Live) Reap(now time.Time) {
	l.reap(now)
}
