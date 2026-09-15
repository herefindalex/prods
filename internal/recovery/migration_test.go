package recovery

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	storesqlite "prods/internal/storage/sqlite"
)

func TestMigrationJournalRequiresVerifiedBackupAndTracksCompletion(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	assetRoot := filepath.Join(root, "data", "assets")
	backupRoot := filepath.Join(root, "backups")
	controlRoot := filepath.Join(root, "data", "control")
	if err := os.MkdirAll(assetRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeSchemaForMigrationTest(t, databasePath)
	store, err = storesqlite.OpenForUpgrade(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir:          backupRoot,
		AssetDir:           assetRoot,
		ApplicationVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	plan, err := storesqlite.PendingMigrations(1)
	if err != nil {
		t.Fatal(err)
	}
	steps := make([]MigrationStep, 0, len(plan))
	for _, item := range plan {
		steps = append(steps, MigrationStep{
			Version: item.Version, Name: item.Name, Checksum: item.Checksum, Transactional: item.Transactional,
		})
	}
	journalPath, err := PrepareMigrationJournal(
		controlRoot,
		filepath.Join(backupRoot, manifest.ID),
		1,
		storesqlite.CurrentSchemaVersion,
		steps,
	)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := LoadMigrationJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Phase != MigrationPrepared || journal.BackupID != manifest.ID {
		t.Fatalf("journal = %+v", journal)
	}
	if err := ValidateMigrationJournalSource(journal); err != nil {
		t.Fatal(err)
	}
	if err := CompleteMigrationJournal(journalPath, 1); err == nil {
		t.Fatal("migration journal accepted the pre-migration schema as complete")
	}
	if err := CompleteMigrationJournal(journalPath, storesqlite.CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
	pending, err := PendingMigrationJournals(controlRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending journals = %v", pending)
	}
}

func TestFailedMigrationJournalStaysPendingForRecovery(t *testing.T) {
	journalPath := newPreparedMigrationJournal(t)
	cause := errors.New("injected migration failure")
	if err := MarkMigrationFailed(journalPath, cause); err != nil {
		t.Fatal(err)
	}
	journal, err := LoadMigrationJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Phase != MigrationFailed || journal.Failure != cause.Error() {
		t.Fatalf("journal = %+v", journal)
	}
	pending, err := PendingMigrationJournals(filepath.Dir(journalPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0] != journalPath {
		t.Fatalf("pending journals = %v", pending)
	}
	if err := SupersedeMigrationJournals(filepath.Dir(journalPath), "restore-explicit-1"); err != nil {
		t.Fatal(err)
	}
	journal, err = LoadMigrationJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Phase != MigrationSuperseded || !strings.Contains(journal.Failure, "restore-explicit-1") {
		t.Fatalf("superseded journal = %+v", journal)
	}
	pending, err = PendingMigrationJournals(filepath.Dir(journalPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after explicit restore = %v", pending)
	}
}

func newPreparedMigrationJournal(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	assetRoot := filepath.Join(root, "data", "assets")
	backupRoot := filepath.Join(root, "backups")
	controlRoot := filepath.Join(root, "data", "control")
	if err := os.MkdirAll(assetRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	downgradeSchemaForMigrationTest(t, databasePath)
	store, err = storesqlite.OpenForUpgrade(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{BackupDir: backupRoot, AssetDir: assetRoot, ApplicationVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	plan, err := storesqlite.PendingMigrations(1)
	if err != nil {
		t.Fatal(err)
	}
	steps := make([]MigrationStep, 0, len(plan))
	for _, item := range plan {
		steps = append(steps, MigrationStep{Version: item.Version, Name: item.Name, Checksum: item.Checksum, Transactional: item.Transactional})
	}
	path, err := PrepareMigrationJournal(controlRoot, filepath.Join(backupRoot, manifest.ID), 1, storesqlite.CurrentSchemaVersion, steps)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func downgradeSchemaForMigrationTest(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TABLE schema_migrations; UPDATE system_state SET schema_version=1 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
}
