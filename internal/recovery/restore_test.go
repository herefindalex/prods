package recovery

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	storesqlite "prods/internal/storage/sqlite"
)

func TestPreparedRestoreRollsForwardAndReconcilesUnrecordedActivation(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	databasePath := filepath.Join(dataDir, "prods.db")
	assetRoot := filepath.Join(dataDir, "assets")
	configRoot := filepath.Join(root, "config")
	backupRoot := filepath.Join(root, "backups")
	controlRoot := filepath.Join(dataDir, "control")
	for _, directory := range []string{dataDir, assetRoot, configRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := storesqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := identity.HashPassword("ownerpass1")
	owner, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail: "owner@example.test", PasswordHash: password, DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAdminSession(t.Context(), "old-session", "old-csrf", owner, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join("website", "working", "asset-restore", "logo.png")
	assetBody := []byte("backup-asset")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(assetRoot, assetPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, assetPath), assetBody, 0o600); err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(assetBody)
	if _, err := store.CreateAsset(t.Context(), owner.ID, catalog.Asset{
		ID: "asset-restore", OwnerType: "website", OwnerID: "working", OriginalFilename: "logo.png", StoragePath: assetPath,
		MIMEType: "image/png", SizeBytes: int64(len(assetBody)), Checksum: hex.EncodeToString(checksum[:]),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "prods.ini"), []byte("state=backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: backupRoot, AssetDir: assetRoot, Roots: map[string]string{"config": configRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "AFTER-BACKUP"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "prods.ini"), []byte("state=live-after-backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, "new-only.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	journalPath, err := PrepareRestore(t.Context(), RestoreConfig{
		BackupPath: filepath.Join(backupRoot, manifest.ID), ControlDir: controlRoot,
		Targets: map[string]RestoreTarget{
			"database": {Path: databasePath, Kind: "file"},
			"assets":   {Path: assetRoot, Kind: "directory"},
			"config":   {Path: configRoot, Kind: "directory"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := openRestoreTestDB(t, databasePath)
	var products int
	if err := before.QueryRow(`SELECT COUNT(*) FROM products WHERE part_number='AFTER-BACKUP'`).Scan(&products); err != nil || products != 1 {
		t.Fatalf("prepare changed live database products=%d err=%v", products, err)
	}
	before.Close()

	journal, err := LoadRestoreJournal(journalPath)
	if err != nil || journal.Phase != RestorePrepared || len(journal.Completed) != 0 {
		t.Fatalf("prepared journal = %+v, %v", journal, err)
	}
	// Simulate a crash after the database root was durably activated but before
	// its completion was recorded in the journal.
	if err := activateRestoreRoot(journal, "database"); err != nil {
		t.Fatal(err)
	}
	if err := ResumeRestore(t.Context(), journalPath); err != nil {
		t.Fatal(err)
	}
	journal, err = LoadRestoreJournal(journalPath)
	if err != nil || journal.Phase != RestoreVerified || len(journal.Completed) != len(journal.Order) {
		t.Fatalf("verified journal = %+v, %v", journal, err)
	}
	if err := ResumeRestore(t.Context(), journalPath); err != nil {
		t.Fatalf("verified resume is not idempotent: %v", err)
	}

	restored := openRestoreTestDB(t, databasePath)
	defer restored.Close()
	if err := restored.QueryRow(`SELECT COUNT(*) FROM products WHERE part_number='AFTER-BACKUP'`).Scan(&products); err != nil || products != 0 {
		t.Fatalf("restored products=%d err=%v", products, err)
	}
	var sessions int
	if err := restored.QueryRow(`SELECT COUNT(*) FROM admin_sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("restored sessions=%d err=%v", sessions, err)
	}
	if body, err := os.ReadFile(filepath.Join(assetRoot, assetPath)); err != nil || string(body) != string(assetBody) {
		t.Fatalf("restored asset=%q err=%v", body, err)
	}
	if _, err := os.Stat(filepath.Join(assetRoot, "new-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("new-only asset survived restore: %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(configRoot, "prods.ini")); err != nil || string(body) != "state=backup\n" {
		t.Fatalf("restored config=%q err=%v", body, err)
	}
	for name, target := range journal.Targets {
		if _, err := os.Stat(target.Path + ".pre-restore-" + journal.OperationID); err != nil {
			t.Fatalf("pre-restore root %s not preserved: %v", name, err)
		}
	}
	pending, err := PendingRestoreJournals(controlRoot)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending journals=%v err=%v", pending, err)
	}
}

func openRestoreTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	return db
}
