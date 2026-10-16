//go:build !linux && !darwin

package kago

import (
	"errors"
	"fmt"
)

// FreeSpace は、Linux と macOS 以外では空き容量を調べられないので、エラーを返す。
func FreeSpace(path string) (uint64, error) {
	return 0, fmt.Errorf("failed to get the free space of %s: %w", path, errors.ErrUnsupported)
}

// TotalSpace は、Linux と macOS 以外では容量を調べられないので、エラーを返す。
func TotalSpace(path string) (uint64, error) {
	return 0, fmt.Errorf("failed to get the total space of %s: %w", path, errors.ErrUnsupported)
}
