package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prods/internal/catalog"
)

func TestAssetGarbageCollectionUsesGraceRevalidationAndDurableDeletionWork(t *testing.T) {
	store, owner := installedStore(t)
	ctx := context.Background()
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{
		PartNumber: "GC-ORPHAN-1",
		CategoryID: "cat_uncategorized",
		Status:     catalog.Hidden,
	})
	if err != nil {
		t.Fatal(err)
	}
	asset := createGCAsset(t, store, owner.ID, product.ID, "asset_orphan")

	start := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	marked, planned, err := store.PlanAssetGarbageCollection(ctx, start, 7*24*time.Hour, 10)
	if err != nil || marked != 1 || planned != 0 {
		t.Fatalf("first scan marked=%d planned=%d err=%v", marked, planned, err)
	}
	_, planned, err = store.PlanAssetGarbageCollection(ctx, start.Add(6*24*time.Hour), 7*24*time.Hour, 10)
	if err != nil || planned != 0 {
		t.Fatalf("pre-grace planned=%d err=%v", planned, err)
	}
	_, planned, err = store.PlanAssetGarbageCollection(ctx, start.Add(8*24*time.Hour), 7*24*time.Hour, 10)
	if err != nil || planned != 1 {
		t.Fatalf("expired planned=%d err=%v", planned, err)
	}

	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE id=?`, asset.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("asset metadata count=%d err=%v", count, err)
	}
	pending, err := store.PendingAssetFileDeletions(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].AssetID != asset.ID || pending[0].StoragePath != asset.StoragePath {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if err := store.RecordAssetFileDeletion(ctx, asset.ID, errors.New("temporary filesystem failure")); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingAssetFileDeletions(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].State != "failed" || pending[0].LastError == "" {
		t.Fatalf("failed pending=%+v err=%v", pending, err)
	}
	if err := store.RecordAssetFileDeletion(ctx, asset.ID, nil); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingAssetFileDeletions(ctx, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed pending=%+v err=%v", pending, err)
	}
}

func TestAssetGarbageCollectionClearsRecoveredOrphanAndHonorsSnapshotPin(t *testing.T) {
	store, owner := installedStore(t)
	ctx := context.Background()
	product, err := store.CreateProduct(ctx, owner.ID, catalog.Product{
		PartNumber: "GC-RECOVER-1",
		CategoryID: "cat_uncategorized",
		Status:     catalog.Hidden,
	})
	if err != nil {
		t.Fatal(err)
	}
	asset := createGCAsset(t, store, owner.ID, product.ID, "asset_recovered")
	start := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	if marked, planned, err := store.PlanAssetGarbageCollection(ctx, start, 7*24*time.Hour, 10); err != nil || marked != 1 || planned != 0 {
		t.Fatalf("mark orphan marked=%d planned=%d err=%v", marked, planned, err)
	}

	image, err := store.AddProductImage(ctx, owner.ID, product.Revision, catalog.ProductImage{
		ID:        "image_gc_recovered",
		ProductID: product.ID,
		AssetID:   asset.ID,
		AltText:   "Recovered reference",
		Primary:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PlanAssetGarbageCollection(ctx, start.Add(8*24*time.Hour), 7*24*time.Hour, 10); err != nil {
		t.Fatal(err)
	}
	var orphanCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM asset_gc_orphans WHERE asset_id=?`, asset.ID).Scan(&orphanCount); err != nil || orphanCount != 0 {
		t.Fatalf("recovered orphan count=%d err=%v", orphanCount, err)
	}

	if err := store.DeleteProductImage(ctx, owner.ID, product.ID, image.ID, product.Revision+1); err != nil {
		t.Fatal(err)
	}
	remark := start.Add(9 * 24 * time.Hour)
	if marked, planned, err := store.PlanAssetGarbageCollection(ctx, remark, 7*24*time.Hour, 10); err != nil || marked != 1 || planned != 0 {
		t.Fatalf("remark orphan marked=%d planned=%d err=%v", marked, planned, err)
	}

	snapshotPath := filepath.Join(t.TempDir(), "backup.db")
	if err := store.SnapshotWithAssetPins(ctx, snapshotPath, "backup_test_pin"); err != nil {
		t.Fatal(err)
	}
	if _, planned, err := store.PlanAssetGarbageCollection(ctx, remark.Add(8*24*time.Hour), 7*24*time.Hour, 10); err != nil || planned != 0 {
		t.Fatalf("pinned planned=%d err=%v", planned, err)
	}
	if err := store.ReleaseAssetPins(ctx, "backup_test_pin"); err != nil {
		t.Fatal(err)
	}
	releasedAt := remark.Add(8 * 24 * time.Hour)
	if marked, planned, err := store.PlanAssetGarbageCollection(ctx, releasedAt, 7*24*time.Hour, 10); err != nil || marked != 1 || planned != 0 {
		t.Fatalf("released remark=%d planned=%d err=%v", marked, planned, err)
	}
	if _, planned, err := store.PlanAssetGarbageCollection(ctx, releasedAt.Add(8*24*time.Hour), 7*24*time.Hour, 10); err != nil || planned != 1 {
		t.Fatalf("released after grace planned=%d err=%v", planned, err)
	}
}

func TestPreAssetLifecycleSchemaSnapshotSkipsPinsForOfflineUpgradeBackup(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.db.Exec(`DROP TABLE asset_gc_deletions; DROP TABLE asset_backup_pins; DROP TABLE asset_gc_orphans;`); err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(t.TempDir(), "pre-v9.db")
	if err := store.SnapshotWithAssetPins(context.Background(), snapshotPath, "pre_upgrade"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseAssetPins(context.Background(), "pre_upgrade"); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearAssetPins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(snapshotPath); err != nil || !info.Mode().IsRegular() {
		t.Fatal(err)
	}
}

func createGCAsset(t *testing.T, store *Store, actorID, productID, assetID string) catalog.Asset {
	t.Helper()
	asset, err := store.CreateAsset(context.Background(), actorID, catalog.Asset{
		ID:               assetID,
		OwnerType:        "product",
		OwnerID:          productID,
		OriginalFilename: "asset.png",
		StoragePath:      filepath.Join("products", productID, assetID, "content.png"),
		MIMEType:         "image/png",
		SizeBytes:        1,
		Checksum:         strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	return asset
}
