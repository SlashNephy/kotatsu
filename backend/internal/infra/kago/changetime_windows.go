//go:build windows

package kago

import (
	"io/fs"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileBasicInfo は、GetFileInformationByHandleEx の FileBasicInfo で受け取る FILE_BASIC_INFO。
// 日時の LARGE_INTEGER は、同じ並びの Filetime で受け取る。
type fileBasicInfo struct {
	CreationTime   windows.Filetime
	LastAccessTime windows.Filetime
	LastWriteTime  windows.Filetime
	ChangeTime     windows.Filetime
	FileAttributes uint32
	_              uint32
}

// changeTime は、root の中のファイル rel の状態を最後に変えた日時 (NTFS の ChangeTime) を返す。名前を変えたときも新しくなる。
// fs.FileInfo の Win32FileAttributeData には ChangeTime がないので、ファイルを開いて調べる。調べられなければ更新日時を返す。
func changeTime(root *os.Root, rel string, info fs.FileInfo) time.Time {
	f, err := root.Open(rel)
	if err != nil {
		return info.ModTime()
	}
	defer f.Close()

	var basic fileBasicInfo
	if err := windows.GetFileInformationByHandleEx(windows.Handle(f.Fd()), windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic))); err != nil {
		return info.ModTime()
	}

	return time.Unix(0, basic.ChangeTime.Nanoseconds())
}
