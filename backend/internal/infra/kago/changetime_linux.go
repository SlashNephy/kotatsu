//go:build linux

package kago

import (
	"io/fs"
	"os"
	"syscall"
	"time"
)

// changeTime は、ファイルの状態を最後に変えた日時 (ctime) を返す。名前を変えたときも新しくなる。調べられなければ更新日時を返す。
func changeTime(_ *os.Root, _ string, info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Ctim.Unix())
	}

	return info.ModTime()
}
