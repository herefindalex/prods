//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsResourceProbeUsesContainingDirectoryForFile(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "prods.db")
	if err := os.WriteFile(databasePath, []byte("sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}

	stats, err := probeResourceVolume(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if stats.VolumeID == "" || stats.TotalBytes == 0 || stats.FreeBytes == 0 {
		t.Fatalf("database volume stats = %+v", stats)
	}
}

func TestWindowsResourceProbeUsesExistingAncestorForMissingFile(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "not-created", "prods.db")

	stats, err := probeResourceVolume(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if stats.VolumeID == "" || stats.TotalBytes == 0 || stats.FreeBytes == 0 {
		t.Fatalf("database volume stats = %+v", stats)
	}
}
