package recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/storage/sqlite"
)

type assetGCTestStore struct {
	cleared  bool
	pending  []sqlite.AssetFileDeletion
	recorded map[string]error
	planErr  error
}

func (store *assetGCTestStore) ClearAssetPins(context.Context) error {
	store.cleared = true
	return nil
}

func (store *assetGCTestStore) PlanAssetGarbageCollection(context.Context, time.Time, time.Duration, int) (int, int, error) {
	return 2, len(store.pending), store.planErr
}

func TestAssetGCManagerRuntimeFailureRemainsVisible(t *testing.T) {
	store := &assetGCTestStore{planErr: errors.New("database unavailable")}
	manager, err := NewAssetGCManager(context.Background(), store, AssetGCConfig{AssetDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	manager.runCycle(t.Context())
	health := manager.RuntimeHealth()
	if health.ConsecutiveFailures != 1 || health.FailureStage != "garbage_collection" || health.LastFailureUTC.IsZero() {
		t.Fatalf("asset GC runtime health=%+v", health)
	}
	store.planErr = nil
	manager.runCycle(t.Context())
	health = manager.RuntimeHealth()
	if health.ConsecutiveFailures != 0 || health.FailureStage != "" || health.LastSuccessUTC.IsZero() {
		t.Fatalf("recovered asset GC runtime health=%+v", health)
	}
}

func (store *assetGCTestStore) PendingAssetFileDeletions(context.Context, int) ([]sqlite.AssetFileDeletion, error) {
	return store.pending, nil
}

func (store *assetGCTestStore) RecordAssetFileDeletion(_ context.Context, assetID string, err error) error {
	if store.recorded == nil {
		store.recorded = make(map[string]error)
	}
	store.recorded[assetID] = err
	return nil
}

func (store *assetGCTestStore) PurgeAssetDeletionHistory(context.Context, time.Time) error {
	return nil
}

func TestAssetGCManagerDeletesOnlyManagedFilesAndRecordsResult(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("products", "product_1", "asset_1", "content.png")
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &assetGCTestStore{pending: []sqlite.AssetFileDeletion{{AssetID: "asset_1", StoragePath: path, State: "planned"}}}
	manager, err := NewAssetGCManager(context.Background(), store, AssetGCConfig{AssetDir: root, Grace: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !store.cleared {
		t.Fatal("stale backup pins were not reconciled at startup")
	}
	result, err := manager.RunOnce(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Marked != 2 || result.Planned != 1 || result.FilesDeleted != 1 || result.DeletionFailed != 0 {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(fullPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("asset file still exists: %v", err)
	}
	if err := store.recorded["asset_1"]; err != nil {
		t.Fatalf("recorded deletion error=%v", err)
	}
}

func TestAssetGCManagerFailsClosedForUnavailableRootAndEscapingPath(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "offline")
	store := &assetGCTestStore{pending: []sqlite.AssetFileDeletion{{AssetID: "asset_offline", StoragePath: "product/file.png"}}}
	manager, err := NewAssetGCManager(context.Background(), store, AssetGCConfig{AssetDir: missingRoot})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.RunOnce(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletionFailed != 1 || store.recorded["asset_offline"] == nil {
		t.Fatalf("offline result=%+v recorded=%v", result, store.recorded)
	}

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeManagedAssetFile(root, "../outside.png"); err == nil {
		t.Fatal("escaping asset path was accepted")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was changed: %v", err)
	}
}
