package recovery

import (
	"context"
	"path/filepath"
	"testing"

	storesqlite "prods/internal/storage/sqlite"
)

type pinRecordingSnapshotter struct {
	*storesqlite.Store
	pinned   bool
	released bool
}

func (store *pinRecordingSnapshotter) SnapshotWithAssetPins(ctx context.Context, destination, operationID string) error {
	store.pinned = true
	return store.Store.SnapshotWithAssetPins(ctx, destination, operationID)
}

func (store *pinRecordingSnapshotter) ReleaseAssetPins(ctx context.Context, operationID string) error {
	store.released = true
	return store.Store.ReleaseAssetPins(ctx, operationID)
}

func TestCreateBackupPinsSnapshotAssetsUntilCompanionCopyCompletes(t *testing.T) {
	root := t.TempDir()
	database, err := storesqlite.CreatePOC(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := &pinRecordingSnapshotter{Store: database}
	if _, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: filepath.Join(root, "backups"),
		AssetDir:  filepath.Join(root, "assets"),
	}); err != nil {
		t.Fatal(err)
	}
	if !store.pinned || !store.released {
		t.Fatalf("snapshot pin lifecycle pinned=%v released=%v", store.pinned, store.released)
	}
}
