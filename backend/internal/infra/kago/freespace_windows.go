//go:build windows

package kago

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// FreeSpace は、path を含むボリュームの、このプロセスのユーザーが使える空き容量 (バイト) を返す。ディスククォータがあれば、その残りまで。
func FreeSpace(path string) (uint64, error) {
	free, _, err := diskSpace(path)

	return free, err
}

// TotalSpace は、path を含むボリュームの容量 (バイト) を返す。
func TotalSpace(path string) (uint64, error) {
	_, total, err := diskSpace(path)

	return total, err
}

// diskSpace は、GetDiskFreeSpaceEx で、path を含むボリュームの、呼び出し元が使える空き容量と容量を返す。
func diskSpace(path string) (uint64, uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get the disk space of %s: %w", path, err)
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, 0, fmt.Errorf("failed to get the disk space of %s: %w", path, err)
	}

	return free, total, nil
}
