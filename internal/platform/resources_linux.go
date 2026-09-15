//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func probeResourceVolume(path string) (ResourceStats, error) {
	existing, info, err := nearestExistingPath(path)
	if err != nil {
		return ResourceStats{}, err
	}
	var stats syscall.Statfs_t
	if err := syscall.Statfs(existing, &stats); err != nil {
		return ResourceStats{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ResourceStats{}, fmt.Errorf("filesystem device identity unavailable")
	}
	return ResourceStats{
		VolumeID:       fmt.Sprintf("dev:%d", stat.Dev),
		TotalBytes:     stats.Blocks * uint64(stats.Bsize),
		FreeBytes:      stats.Bavail * uint64(stats.Bsize),
		FreeInodes:     stats.Ffree,
		SupportsInodes: true,
	}, nil
}

func nearestExistingPath(path string) (string, os.FileInfo, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	for candidate := filepath.Clean(absolute); ; candidate = filepath.Dir(candidate) {
		info, statErr := os.Stat(candidate)
		if statErr == nil {
			return candidate, info, nil
		}
		if !os.IsNotExist(statErr) {
			return "", nil, statErr
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", nil, statErr
		}
	}
}
