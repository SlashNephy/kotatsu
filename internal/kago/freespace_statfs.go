//go:build linux || darwin

package kago

import (
	"fmt"
	"syscall"
)

// FreeSpace は、path を含むファイルシステムの、root 以外のユーザーが使える空き容量 (バイト) を返す。
func FreeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("failed to statfs %s: %w", path, err)
	}

	bsize, ok := positive(stat.Bsize)
	if !ok {
		return 0, fmt.Errorf("failed to statfs %s: unexpected block size %d", path, stat.Bsize)
	}

	return stat.Bavail * bsize, nil
}

// positive は、正の整数を uint64 にする。Statfs_t の Bsize の型は OS ごとに違う (Linux は int64、macOS は uint32)。
func positive[T ~int32 | ~int64 | ~uint32 | ~uint64](v T) (uint64, bool) {
	if v <= 0 {
		return 0, false
	}

	return uint64(v), true
}
