package recovery

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"

	"prods/internal/platform"
)

// EstimateBackupResources returns a conservative source-size estimate used for
// admission. Actual writes must still handle quota, read-only, and ENOSPC
// failures because another process can consume capacity after the probe.
func EstimateBackupResources(ctx context.Context, databasePath, assetDir string, roots map[string]string) (uint64, uint64, error) {
	return EstimateBackupResourcesWithFiles(ctx, databasePath, assetDir, roots, nil)
}

func EstimateBackupResourcesWithFiles(ctx context.Context, databasePath, assetDir string, roots, files map[string]string) (uint64, uint64, error) {
	paths := []string{databasePath, databasePath + "-wal", assetDir}
	for _, path := range roots {
		paths = append(paths, path)
	}
	for _, path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	seen := make(map[string]struct{}, len(paths))
	var bytes, inodes uint64
	for _, path := range paths {
		clean := filepath.Clean(path)
		if clean == "." {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		err := filepath.WalkDir(clean, func(_ string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if inodes == math.MaxUint64 {
				return ErrInvalidBackup
			}
			inodes++
			if !entry.Type().IsRegular() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() < 0 || uint64(info.Size()) > math.MaxUint64-bytes {
				return ErrInvalidBackup
			}
			bytes += uint64(info.Size())
			return nil
		})
		if err != nil {
			return 0, 0, err
		}
	}
	return bytes, inodes, nil
}

func admitRestoreResources(ctx context.Context, config RestoreConfig, manifest Manifest) ([]*platform.Reservation, error) {
	if config.ResourceGate == nil {
		return nil, nil
	}
	order := restoreOrder(manifest)
	reservations := make([]*platform.Reservation, 0, len(order))
	for _, name := range order {
		requiredBytes, requiredInodes, err := restoreRootResources(manifest, name)
		if err != nil {
			releaseReservations(reservations)
			return nil, err
		}
		reservation, _, err := config.ResourceGate.Admit(ctx, platform.ResourceRequest{
			Operation:      "restore staging " + name,
			Path:           config.Targets[name].Path,
			RequiredBytes:  requiredBytes,
			ByteHeadroom:   config.ByteHeadroom,
			MinFreePercent: config.MinFreePercent,
			RequiredInodes: requiredInodes,
			InodeHeadroom:  config.InodeHeadroom,
		})
		if err != nil {
			releaseReservations(reservations)
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, nil
}

func restoreRootResources(manifest Manifest, name string) (uint64, uint64, error) {
	if name == "database" {
		if manifest.Database.Size < 0 {
			return 0, 0, ErrInvalidBackup
		}
		return uint64(manifest.Database.Size), 1, nil
	}
	if file, ok := manifest.Files[name]; ok {
		if file.Size < 0 {
			return 0, 0, ErrInvalidBackup
		}
		return uint64(file.Size), 1, nil
	}
	files := manifest.Roots[name]
	if name == "assets" {
		files = make([]ManifestFile, 0, len(manifest.Assets))
		for _, asset := range manifest.Assets {
			files = append(files, asset.ManifestFile)
		}
	}
	var bytes uint64
	directories := map[string]struct{}{".": {}}
	for _, file := range files {
		if file.Size < 0 || uint64(file.Size) > math.MaxUint64-bytes {
			return 0, 0, ErrInvalidBackup
		}
		bytes += uint64(file.Size)
		for directory := filepath.Dir(filepath.FromSlash(file.Path)); directory != "."; directory = filepath.Dir(directory) {
			directories[directory] = struct{}{}
		}
	}
	return bytes, uint64(len(files) + len(directories)), nil
}

func releaseReservations(reservations []*platform.Reservation) {
	for _, reservation := range reservations {
		reservation.Release()
	}
}
