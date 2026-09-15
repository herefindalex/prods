package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	storesqlite "prods/internal/storage/sqlite"
)

type restoreFaultFixture struct {
	databasePath string
	assetRoot    string
	configRoot   string
	controlRoot  string
	backupPath   string
	manifest     Manifest
}

func newRestoreFaultFixture(t *testing.T) restoreFaultFixture {
	t.Helper()

	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	databasePath := filepath.Join(dataRoot, "prods.db")
	assetRoot := filepath.Join(dataRoot, "assets")
	configRoot := filepath.Join(root, "config")
	controlRoot := filepath.Join(dataRoot, "control")
	backupRoot := filepath.Join(root, "backups")
	for _, directory := range []string{dataRoot, assetRoot, configRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	store, err := storesqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail:       "owner@example.test",
		OwnerDisplayName: "Owner",
		PasswordHash:     passwordHash,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US"},
		TimeZone:         "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	assetBody := []byte("backup-asset")
	assetChecksum := sha256.Sum256(assetBody)
	assetPath := filepath.Join("product", "restore-product", "asset-restore", "logo.png")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(assetRoot, assetPath)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, assetPath), assetBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateAsset(t.Context(), owner.ID, catalog.Asset{
		ID:               "asset-restore",
		OwnerType:        "website",
		OwnerID:          "working",
		OriginalFilename: "logo.png",
		StoragePath:      filepath.ToSlash(assetPath),
		MIMEType:         "image/png",
		SizeBytes:        int64(len(assetBody)),
		Checksum:         hex.EncodeToString(assetChecksum[:]),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "prods.ini"), []byte("state=backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir:          backupRoot,
		AssetDir:           assetRoot,
		ApplicationVersion: "test",
		Roots:              map[string]string{"config": configRoot},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "AFTER-BACKUP"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configRoot, "prods.ini"), []byte("state=live-after-backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetRoot, "new-only.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	return restoreFaultFixture{
		databasePath: databasePath,
		assetRoot:    assetRoot,
		configRoot:   configRoot,
		controlRoot:  controlRoot,
		backupPath:   filepath.Join(backupRoot, manifest.ID),
		manifest:     manifest,
	}
}

func (fixture restoreFaultFixture) config() RestoreConfig {
	return RestoreConfig{
		BackupPath: fixture.backupPath,
		ControlDir: fixture.controlRoot,
		Targets: map[string]RestoreTarget{
			"database": {Path: fixture.databasePath, Kind: "file"},
			"assets":   {Path: fixture.assetRoot, Kind: "directory"},
			"config":   {Path: fixture.configRoot, Kind: "directory"},
		},
	}
}

func TestPrepareRestoreFailureBeforePreparedDoesNotChangeLiveRoots(t *testing.T) {
	fixture := newRestoreFaultFixture(t)
	configFile := fixture.manifest.Roots["config"][0]
	if err := os.WriteFile(
		filepath.Join(fixture.backupPath, "roots", "config", filepath.FromSlash(configFile.Path)),
		[]byte("tampered"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := PrepareRestore(t.Context(), fixture.config()); err == nil {
		t.Fatal("PrepareRestore succeeded with a tampered pre-prepared root")
	}
	pending, err := PendingRestoreJournals(fixture.controlRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending journals = %v, want none", pending)
	}
	assertLiveAfterBackupState(t, fixture)
}

func TestPreparedRestoreRollsForwardFromEveryActivationBoundary(t *testing.T) {
	for _, completedRoots := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+completedRoots))+"-roots-active", func(t *testing.T) {
			fixture := newRestoreFaultFixture(t)
			journalPath, err := PrepareRestore(t.Context(), fixture.config())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := LoadRestoreJournal(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range journal.Order[:completedRoots] {
				if err := activateRestoreRoot(journal, name); err != nil {
					t.Fatalf("activate %s: %v", name, err)
				}
			}

			if err := ResumeRestore(t.Context(), journalPath); err != nil {
				t.Fatal(err)
			}
			assertVerifiedRestoreState(t, journalPath, fixture)
		})
	}
}

func TestPreparedRestoreReconcilesCrashBetweenLiveAndStageRename(t *testing.T) {
	fixture := newRestoreFaultFixture(t)
	journalPath, err := PrepareRestore(t.Context(), fixture.config())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := LoadRestoreJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	target := journal.Targets["database"]
	preRestore := target.Path + ".pre-restore-" + journal.OperationID
	if err := os.Rename(target.Path, preRestore); err != nil {
		t.Fatal(err)
	}
	if err := syncDirectory(filepath.Dir(target.Path)); err != nil {
		t.Fatal(err)
	}

	if err := ResumeRestore(t.Context(), journalPath); err != nil {
		t.Fatal(err)
	}
	assertVerifiedRestoreState(t, journalPath, fixture)
}

func TestPreparedRestoreFinishesFinalVerificationAfterAllEvidence(t *testing.T) {
	fixture := newRestoreFaultFixture(t)
	journalPath, err := PrepareRestore(t.Context(), fixture.config())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := LoadRestoreJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range journal.Order {
		if err := activateRestoreRoot(journal, name); err != nil {
			t.Fatalf("activate %s: %v", name, err)
		}
		journal.Completed[name] = journal.RootSHA256[name]
		if err := saveRestoreJournal(journalPath, journal); err != nil {
			t.Fatal(err)
		}
	}

	before, err := LoadRestoreJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if before.Phase != RestorePrepared {
		t.Fatalf("phase before final verification = %q", before.Phase)
	}
	if err := ResumeRestore(t.Context(), journalPath); err != nil {
		t.Fatal(err)
	}
	assertVerifiedRestoreState(t, journalPath, fixture)
}

func assertLiveAfterBackupState(t *testing.T, fixture restoreFaultFixture) {
	t.Helper()
	db := openRestoreTestDB(t, fixture.databasePath)
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM products WHERE part_number='AFTER-BACKUP'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("post-backup product count = %d, want 1", count)
	}
	body, err := os.ReadFile(filepath.Join(fixture.configRoot, "prods.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "state=live-after-backup\n" {
		t.Fatalf("live config = %q", body)
	}
}

func assertVerifiedRestoreState(t *testing.T, journalPath string, fixture restoreFaultFixture) {
	t.Helper()
	journal, err := LoadRestoreJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Phase != RestoreVerified || len(journal.Completed) != len(journal.Order) {
		t.Fatalf("verified journal = %+v", journal)
	}

	db := openRestoreTestDB(t, fixture.databasePath)
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM products WHERE part_number='AFTER-BACKUP'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("post-backup product survived restore: count=%d", count)
	}
	if _, err := os.Stat(filepath.Join(fixture.assetRoot, "new-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("post-backup asset still present: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(fixture.configRoot, "prods.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "state=backup\n" {
		t.Fatalf("restored config = %q", body)
	}
}
