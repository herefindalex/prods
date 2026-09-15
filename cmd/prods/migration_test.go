package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/recovery"
	storesqlite "prods/internal/storage/sqlite"
)

type migrationFixture struct {
	databasePath string
	journalPath  string
	backupID     string
}

func TestResumeMigrationRollsForwardBeforeDatabaseCommit(t *testing.T) {
	fixture := newMigrationFixture(t)
	if err := resumeMigration(t.Context(), fixture.databasePath, fixture.journalPath); err != nil {
		t.Fatal(err)
	}
	assertMigrationReconciled(t, fixture)
}

func TestResumeMigrationReconcilesCommitBeforeJournalCompletion(t *testing.T) {
	fixture := newMigrationFixture(t)
	store, err := storesqlite.OpenForUpgrade(fixture.databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Upgrade(t.Context(), fixture.backupID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	journal, err := recovery.LoadMigrationJournal(fixture.journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Phase != recovery.MigrationPrepared {
		t.Fatalf("journal phase before reconciliation = %q", journal.Phase)
	}
	if err := resumeMigration(t.Context(), fixture.databasePath, fixture.journalPath); err != nil {
		t.Fatal(err)
	}
	assertMigrationReconciled(t, fixture)
}

func TestResumeMigrationDoesNotRetryRecordedFailure(t *testing.T) {
	fixture := newMigrationFixture(t)
	if err := recovery.MarkMigrationFailed(fixture.journalPath, os.ErrPermission); err != nil {
		t.Fatal(err)
	}
	err := resumeMigration(t.Context(), fixture.databasePath, fixture.journalPath)
	if err == nil || !strings.Contains(err.Error(), "recovery required") {
		t.Fatalf("resume error = %v", err)
	}
	inspection := storesqlite.Inspect(fixture.databasePath)
	if inspection.State != storesqlite.DatabaseUpgrade || inspection.SchemaVersion != 1 {
		t.Fatalf("inspection = %+v", inspection)
	}
}

func newMigrationFixture(t *testing.T) migrationFixture {
	t.Helper()
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	databasePath := filepath.Join(dataRoot, "prods.db")
	assetRoot := filepath.Join(dataRoot, "assets")
	backupRoot := filepath.Join(root, "backups")
	controlRoot := filepath.Join(dataRoot, "control")
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
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(databasePath)+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		DROP TABLE search_submission_jobs;
		DROP TABLE search_notification_state;
		DROP TABLE search_integration_settings;
		DROP TABLE site_maintenance;
		DROP TABLE smtp_delivery_recipients;
		DROP TABLE smtp_delivery_attempts;
		DROP INDEX rfq_default_recipients_user_idx;
		DROP TABLE rfq_default_recipients;
		DROP TABLE rfq_recipient_settings;
		DROP INDEX rfq_recipients_user_idx;
		DROP TABLE rfq_recipients;
		CREATE TABLE rfq_recipients (
			rfq_id TEXT NOT NULL REFERENCES rfqs(id),
			recipient TEXT NOT NULL,
			PRIMARY KEY (rfq_id,recipient)
		);
		ALTER TABLE rfqs DROP COLUMN privacy_processed_by;
		ALTER TABLE rfqs DROP COLUMN privacy_processed_at;
		ALTER TABLE rfqs DROP COLUMN privacy_state;
		ALTER TABLE rfqs DROP COLUMN updated_at;
		ALTER TABLE rfqs DROP COLUMN updated_by;
		ALTER TABLE rfqs DROP COLUMN revision;
		ALTER TABLE rfqs DROP COLUMN status;
		ALTER TABLE publication_intents DROP COLUMN error_message;
		DROP TABLE schema_migrations;
		UPDATE system_state SET schema_version=1 WHERE singleton=1;
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storesqlite.OpenForUpgrade(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir:          backupRoot,
		AssetDir:           assetRoot,
		ApplicationVersion: "test",
	})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	plan, err := storesqlite.PendingMigrations(1)
	if err != nil {
		t.Fatal(err)
	}
	journalPath, err := recovery.PrepareMigrationJournal(
		controlRoot,
		filepath.Join(backupRoot, manifest.ID),
		1,
		storesqlite.CurrentSchemaVersion,
		migrationSteps(plan),
	)
	if err != nil {
		t.Fatal(err)
	}
	return migrationFixture{databasePath: databasePath, journalPath: journalPath, backupID: manifest.ID}
}

func assertMigrationReconciled(t *testing.T, fixture migrationFixture) {
	t.Helper()
	inspection := storesqlite.Inspect(fixture.databasePath)
	if inspection.State != storesqlite.DatabaseReady || inspection.SchemaVersion != storesqlite.CurrentSchemaVersion {
		t.Fatalf("inspection = %+v", inspection)
	}
	journal, err := recovery.LoadMigrationJournal(fixture.journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal.Phase != recovery.MigrationVerified {
		t.Fatalf("journal = %+v", journal)
	}
}
