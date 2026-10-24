//go:build !linux && !darwin

package kago

import (
	"io/fs"
	"time"
)

// changeTime は、Linux と macOS 以外では ctime を調べられないので、更新日時を返す。
func changeTime(info fs.FileInfo) time.Time {
	return info.ModTime()
}
