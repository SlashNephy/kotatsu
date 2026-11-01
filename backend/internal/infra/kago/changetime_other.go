//go:build !linux && !darwin && !windows

package kago

import (
	"io/fs"
	"os"
	"time"
)

// changeTime は、Linux、macOS、Windows 以外では ctime を調べられないので、更新日時を返す。
func changeTime(_ *os.Root, _ string, info fs.FileInfo) time.Time {
	return info.ModTime()
}
