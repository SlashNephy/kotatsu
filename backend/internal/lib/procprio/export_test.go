//go:build linux

package procprio

import "errors"

// テストから、Linux の優先度の下げ方を呼べるようにする。
const LowestNice = lowestNice

// Lower は lower を呼ぶ。
func Lower(pid int) error { return lower(pid) }

// IsProcessGone は、err が、プロセスがもう終わっていたことを表すかを返す。
func IsProcessGone(err error) bool { return errors.Is(err, errProcessGone) }
