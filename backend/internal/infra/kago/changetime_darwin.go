//go:build darwin

package kago

import (
	"io/fs"
	"syscall"
	"time"
)

// changeTime は、ファイルの状態を最後に変えた日時 (ctime) を返す。名前を変えたときも新しくなる。調べられなければ更新日時を返す。
func changeTime(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Ctimespec.Unix())
	}

	return info.ModTime()
}
