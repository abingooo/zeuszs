//go:build !windows

package main

import (
	"fmt"
	"math"

	"golang.org/x/sys/unix"
)

func availableDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("inspect free disk space for %s: %w", path, err)
	}
	if int64(stat.Bavail) < 0 || int64(stat.Bsize) <= 0 {
		return 0, fmt.Errorf("filesystem reported invalid free disk space for %s", path)
	}
	blocks := uint64(stat.Bavail)
	blockSize := uint64(stat.Bsize)
	if blocks > math.MaxUint64/blockSize {
		return math.MaxUint64, nil
	}
	return blocks * blockSize, nil
}
