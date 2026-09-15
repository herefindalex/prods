package recovery

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	storesqlite "prods/internal/storage/sqlite"
)

func TestCreateBackupPublishesOnlyCompleteVerifiedManifest(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail: "owner@example.test", PasswordHash: password, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	assetRoot := filepath.Join(root, "data", "assets")
	assetPath := filepath.Join("website", "working", "asset-logo", "logo.png")
	body := []byte("immutable-image")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(assetRoot, assetPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, assetPath), body, 0o600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(body)
	if _, err := store.CreateAsset(t.Context(), owner.ID, catalog.Asset{
		ID: "asset-logo", OwnerType: "website", OwnerID: "working", OriginalFilename: "logo.png",
		StoragePath: assetPath, MIMEType: "image/png", SizeBytes: int64(len(body)), Checksum: hex.EncodeToString(hash[:]),
	}); err != nil {
		t.Fatal(err)
	}
	configRoot := filepath.Join(root, "config")
	if err := os.MkdirAll(configRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "prods.ini"), []byte("listen=:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	backupRoot := filepath.Join(root, "backups")
	t.Cleanup(func() { makeBackupTreeWritable(backupRoot) })
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: backupRoot, AssetDir: assetRoot, ApplicationVersion: "test",
		Roots: map[string]string{"config": configRoot}, ApplyReadOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != storesqlite.CurrentSchemaVersion || len(manifest.Assets) != 1 || len(manifest.Roots["config"]) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if !manifest.ContentVerified || !manifest.ReadOnlyApplied {
		t.Fatalf("backup completion evidence = %+v", manifest)
	}
	backupPath := filepath.Join(backupRoot, manifest.ID)
	loaded, err := LoadManifest(backupPath)
	if err != nil || loaded.ID != manifest.ID || !loaded.ContentVerified || !loaded.ReadOnlyApplied {
		t.Fatalf("loaded manifest = %+v, %v", loaded, err)
	}
	if info, err := os.Stat(filepath.Join(backupPath, "manifest.json")); err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("manifest protection mode = %v, %v", info, err)
	}
	if copied, err := os.ReadFile(filepath.Join(backupPath, "assets", assetPath)); err != nil || string(copied) != string(body) {
		t.Fatalf("copied asset = %q, %v", copied, err)
	}
	snapshot, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(backupPath, "database", "prods.db"))+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var users int
	if err := snapshot.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("snapshot users=%d err=%v", users, err)
	}

	if err := os.Mkdir(filepath.Join(backupRoot, ".interrupted.incomplete"), 0o700); err != nil {
		t.Fatal(err)
	}
	listed, err := ListBackups(backupRoot)
	if err != nil || len(listed) != 1 || listed[0].ID != manifest.ID {
		t.Fatalf("listed backups = %+v, %v", listed, err)
	}
}

func TestCreateBackupRejectsAssetBytesThatDoNotMatchSnapshot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.Create(filepath.Join(root, "data", "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, _ := identity.HashPassword("ownerpass1")
	owner, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail: "owner@example.test", PasswordHash: password, DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	assetRoot := filepath.Join(root, "assets")
	assetPath := filepath.Join("website", "working", "asset-bad", "bad.png")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(assetRoot, assetPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, assetPath), []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("expected"))
	if _, err := store.CreateAsset(t.Context(), owner.ID, catalog.Asset{
		ID: "asset-bad", OwnerType: "website", OwnerID: "working", OriginalFilename: "bad.png", StoragePath: assetPath,
		MIMEType: "image/png", SizeBytes: int64(len("expected")), Checksum: hex.EncodeToString(expected[:]),
	}); err != nil {
		t.Fatal(err)
	}
	backupRoot := filepath.Join(root, "backups")
	if _, err := CreateBackup(t.Context(), store, BackupConfig{BackupDir: backupRoot, AssetDir: assetRoot}); err == nil {
		t.Fatal("backup accepted mismatched asset bytes")
	}
	listed, err := ListBackups(backupRoot)
	if err != nil || len(listed) != 0 {
		t.Fatalf("invalid backup became restorable: %+v, %v", listed, err)
	}
}
