package recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"prods/internal/platform"
)

type failingSnapshotter struct {
	called bool
}

func (snapshotter *failingSnapshotter) Snapshot(context.Context, string) error {
	snapshotter.called = true
	return errors.New("snapshot failed")
}

func TestBackupResourceAdmissionRejectsBeforeSnapshotAndReleasesOnError(t *testing.T) {
	gate := platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{VolumeID: "backup-volume", TotalBytes: 100, FreeBytes: 100}, nil
	})
	rejected := &failingSnapshotter{}
	_, err := CreateBackup(t.Context(), rejected, BackupConfig{
		BackupDir: t.TempDir(), AssetDir: t.TempDir(), ResourceGate: gate,
		RequiredBytes: 90, ByteHeadroom: 20,
	})
	if !errors.Is(err, platform.ErrResourceCritical) {
		t.Fatalf("backup admission error = %v", err)
	}
	if rejected.called {
		t.Fatal("snapshot ran after resource admission was rejected")
	}

	failing := &failingSnapshotter{}
	_, err = CreateBackup(t.Context(), failing, BackupConfig{
		BackupDir: t.TempDir(), AssetDir: t.TempDir(), ResourceGate: gate,
		RequiredBytes: 60,
	})
	if err == nil || !failing.called {
		t.Fatalf("snapshot failure = %v called=%v", err, failing.called)
	}
	reservation, _, err := gate.Admit(t.Context(), platform.ResourceRequest{
		Operation: "after failed backup", Path: t.TempDir(), RequiredBytes: 60,
	})
	if err != nil {
		t.Fatalf("reservation leaked after failed backup: %v", err)
	}
	reservation.Release()
}

func TestEstimateBackupResourcesIncludesDatabaseWALAssetsAndRoots(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "prods.db")
	assets := filepath.Join(root, "assets")
	config := filepath.Join(root, "config")
	hostConfig := filepath.Join(root, "runtime", "prods.ini")
	for path, body := range map[string]string{
		database:                               "database",
		database + "-wal":                      "wal",
		filepath.Join(assets, "a.bin"):         "asset",
		filepath.Join(config, "settings.json"): "config",
		hostConfig:                             "host-config",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bytes, inodes, err := EstimateBackupResourcesWithFiles(
		t.Context(), database, assets, map[string]string{"config": config}, map[string]string{"host-config": hostConfig},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bytes != uint64(len("database")+len("wal")+len("asset")+len("config")+len("host-config")) {
		t.Fatalf("estimated bytes = %d", bytes)
	}
	if inodes < 7 {
		t.Fatalf("estimated inodes = %d", inodes)
	}
}

func TestRestoreResourceAdmissionRejectsBeforePreparedOrStaging(t *testing.T) {
	fixture := newRestoreFaultFixture(t)
	config := fixture.config()
	config.ResourceGate = platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{VolumeID: "restore-volume", TotalBytes: 100, FreeBytes: 0}, nil
	})
	config.ByteHeadroom = 1
	if _, err := PrepareRestore(t.Context(), config); !errors.Is(err, platform.ErrResourceCritical) {
		t.Fatalf("restore admission error = %v", err)
	}
	pending, err := PendingRestoreJournals(fixture.controlRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("restore journal was prepared after rejected admission: %v", pending)
	}
	assertLiveAfterBackupState(t, fixture)
}
