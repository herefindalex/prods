//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func probeResourceVolume(path string) (ResourceStats, error) {
	existing, info, err := nearestExistingPath(path)
	if err != nil {
		return ResourceStats{}, err
	}
	if !info.IsDir() {
		existing = filepath.Dir(existing)
	}
	volume := filepath.VolumeName(existing)
	if volume == "" {
		return ResourceStats{}, fmt.Errorf("filesystem volume identity unavailable")
	}
	pointer, err := windows.UTF16PtrFromString(existing)
	if err != nil {
		return ResourceStats{}, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free); err != nil {
		return ResourceStats{}, err
	}
	return ResourceStats{VolumeID: volume, TotalBytes: total, FreeBytes: available}, nil
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
