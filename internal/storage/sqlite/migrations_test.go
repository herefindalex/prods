package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFreshDatabaseRecordsImmutableMigrationHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var version int
	if err := store.db.QueryRow(`SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, CurrentSchemaVersion)
	}
	if err := store.verifyMigrationHistory(t.Context()); err != nil {
		t.Fatal(err)
	}
	inspection := Inspect(path)
	if inspection.State != DatabaseInstalling || inspection.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("inspection = %+v", inspection)
	}
}

func TestVersionSeventeenToEighteenAddsDurableProductBulkRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
DROP TABLE product_bulk_items;
DROP TABLE product_bulk_runs;
DELETE FROM schema_migrations WHERE version=18;
UPDATE system_state SET schema_version=17 WHERE singleton=1;
`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	inspection := Inspect(path)
	if inspection.State != DatabaseUpgrade || inspection.SchemaVersion != 17 {
		t.Fatalf("inspection=%+v", inspection)
	}
	upgradeStore, err := OpenForUpgrade(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := upgradeStore.Upgrade(t.Context(), "backup-v17"); err != nil {
		t.Fatal(err)
	}
	defer upgradeStore.Close()

	var version int
	if err := upgradeStore.db.QueryRow(`SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != CurrentSchemaVersion {
		t.Fatalf("schema version=%d, want %d", version, CurrentSchemaVersion)
	}
	var tables int
	if err := upgradeStore.db.QueryRow(`
SELECT COUNT(*) FROM sqlite_master
WHERE type='table' AND name IN ('product_bulk_runs','product_bulk_items')
`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 2 {
		t.Fatalf("durable Product Bulk tables=%d, want 2", tables)
	}
	var receiptBackup string
	if err := upgradeStore.db.QueryRow(`SELECT backup_id FROM schema_migrations WHERE version=18`).Scan(&receiptBackup); err != nil {
		t.Fatal(err)
	}
	if receiptBackup != "backup-v17" {
		t.Fatalf("migration receipt backup=%q", receiptBackup)
	}
}

func TestVersionOneUpgradeRequiresBackupAndBecomesReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	// CreatePOC builds the current schema. Reconstruct the v1 RFQ shape so this
	// test exercises the complete ordered upgrade rather than merely changing
	// the version marker under a current schema.
	if _, err := store.db.Exec(`
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
			PRIMARY KEY (rfq_id, recipient)
		);
		ALTER TABLE rfqs DROP COLUMN updated_at;
		ALTER TABLE rfqs DROP COLUMN updated_by;
		ALTER TABLE rfqs DROP COLUMN revision;
		ALTER TABLE rfqs DROP COLUMN status;
		ALTER TABLE rfqs DROP COLUMN privacy_processed_by;
		ALTER TABLE rfqs DROP COLUMN privacy_processed_at;
		ALTER TABLE rfqs DROP COLUMN privacy_state;
		ALTER TABLE publication_intents DROP COLUMN error_message;
		DROP TABLE site_maintenance;
		DROP TABLE search_submission_jobs;
		DROP TABLE search_notification_state;
		DROP TABLE search_integration_settings;
		DROP TABLE user_invitation_mail_attempts;
		DROP TABLE product_translations;
		DROP TABLE product_content_metadata;
		DROP TABLE public_copy_defaults;
		DROP TABLE public_copy_definitions;
		DROP TABLE public_copy_active;
		DROP TABLE public_copy_bundles;
		DROP TABLE taxonomy_translations;
		DROP TABLE taxonomy_content;
		DROP TABLE locale_registry;
		DROP TABLE product_bulk_items;
		DROP TABLE product_bulk_runs;
		ALTER TABLE website_versions DROP COLUMN public_copy_overrides_json;
		ALTER TABLE website_versions DROP COLUMN content_editing_enabled;
		ALTER TABLE website_versions DROP COLUMN enabled_locales_json;
		ALTER TABLE website_versions DROP COLUMN default_locale;
		ALTER TABLE website_working DROP COLUMN public_copy_overrides_json;
		ALTER TABLE website_working DROP COLUMN content_editing_enabled;
		ALTER TABLE website_working DROP COLUMN enabled_locales_json;
		ALTER TABLE website_working DROP COLUMN default_locale;
		ALTER TABLE site_settings DROP COLUMN content_multilingual_enabled;
		ALTER TABLE site_settings DROP COLUMN updated_by;
		ALTER TABLE site_settings DROP COLUMN revision;
		DROP TABLE schema_migrations;
		UPDATE system_state SET schema_version=1 WHERE singleton=1;
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	inspection := Inspect(path)
	if inspection.State != DatabaseUpgrade || inspection.SchemaVersion != 1 {
		t.Fatalf("inspection = %+v", inspection)
	}
	if _, err := OpenReady(path); !errors.Is(err, ErrDatabaseNotReady) {
		t.Fatalf("OpenReady error = %v", err)
	}

	upgradeStore, err := OpenForUpgrade(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := upgradeStore.Upgrade(t.Context(), ""); !errors.Is(err, ErrMigrationBackupRequired) {
		t.Fatalf("Upgrade without backup error = %v", err)
	}
	if err := upgradeStore.Upgrade(t.Context(), "backup-pre-upgrade-1"); err != nil {
		t.Fatal(err)
	}
	if err := upgradeStore.Close(); err != nil {
		t.Fatal(err)
	}

	inspection = Inspect(path)
	if inspection.State != DatabaseReady || inspection.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("post-upgrade inspection = %+v", inspection)
	}
	ready, err := OpenReady(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ready.Close()
	var receipts, backupReceipts int
	if err := ready.db.QueryRow(`SELECT COUNT(*),SUM(CASE WHEN backup_id='backup-pre-upgrade-1' THEN 1 ELSE 0 END) FROM schema_migrations`).Scan(&receipts, &backupReceipts); err != nil {
		t.Fatal(err)
	}
	if receipts != len(migrationDefinitions) || backupReceipts != len(migrationDefinitions) {
		t.Fatalf("migration receipts=%d backup receipts=%d", receipts, backupReceipts)
	}
}

func TestVersionFourUpgradePreservesRFQAndClassifiesLegacyRecipient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	created := "2026-09-14T12:00:00Z"
	if _, err := store.db.Exec(`
		INSERT INTO rfqs(id,name,email,company,phone,country,general_message,created_at)
		VALUES('rfq_legacy','Legacy Buyer','legacy@example.test','','','','',?);
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
		INSERT INTO rfq_recipients(rfq_id,recipient) VALUES('rfq_legacy','sales@example.test');
		ALTER TABLE rfqs DROP COLUMN updated_at;
		ALTER TABLE rfqs DROP COLUMN updated_by;
		ALTER TABLE rfqs DROP COLUMN revision;
		ALTER TABLE rfqs DROP COLUMN status;
		ALTER TABLE rfqs DROP COLUMN privacy_processed_by;
		ALTER TABLE rfqs DROP COLUMN privacy_processed_at;
		ALTER TABLE rfqs DROP COLUMN privacy_state;
		ALTER TABLE publication_intents DROP COLUMN error_message;
		DROP TABLE site_maintenance;
		DROP TABLE search_submission_jobs;
		DROP TABLE search_notification_state;
		DROP TABLE search_integration_settings;
		DROP TABLE user_invitation_mail_attempts;
		DROP TABLE product_translations;
		DROP TABLE product_content_metadata;
		DROP TABLE public_copy_defaults;
		DROP TABLE public_copy_definitions;
		DROP TABLE public_copy_active;
		DROP TABLE public_copy_bundles;
		DROP TABLE taxonomy_translations;
		DROP TABLE taxonomy_content;
		DROP TABLE locale_registry;
		ALTER TABLE website_versions DROP COLUMN public_copy_overrides_json;
		ALTER TABLE website_versions DROP COLUMN content_editing_enabled;
		ALTER TABLE website_versions DROP COLUMN enabled_locales_json;
		ALTER TABLE website_versions DROP COLUMN default_locale;
		ALTER TABLE website_working DROP COLUMN public_copy_overrides_json;
		ALTER TABLE website_working DROP COLUMN content_editing_enabled;
		ALTER TABLE website_working DROP COLUMN enabled_locales_json;
		ALTER TABLE website_working DROP COLUMN default_locale;
		ALTER TABLE site_settings DROP COLUMN content_multilingual_enabled;
		ALTER TABLE site_settings DROP COLUMN updated_by;
		ALTER TABLE site_settings DROP COLUMN revision;
		DELETE FROM schema_migrations WHERE version IN (5,6,7,8,9,10,11,12,13,14,15,16,17,18);
		UPDATE system_state SET schema_version=4 WHERE singleton=1;
	`, created); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if inspection := Inspect(path); inspection.State != DatabaseUpgrade || inspection.SchemaVersion != 4 {
		t.Fatalf("pre-upgrade inspection = %+v", inspection)
	}
	store, err = OpenForUpgrade(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Upgrade(t.Context(), "backup-v4"); err != nil {
		t.Fatal(err)
	}
	rfq, err := store.RFQ(t.Context(), "rfq_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if rfq.Status != "new" || rfq.Revision != 1 || rfq.UpdatedAt != created || rfq.PrivacyState != "retained" || len(rfq.Recipients) != 1 ||
		rfq.Recipients[0].Kind != "email" || rfq.Recipients[0].Email != "sales@example.test" {
		t.Fatalf("migrated RFQ = %#v", rfq)
	}
}

func TestTransactionalMigrationFailureLeavesOldVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`DROP TABLE schema_migrations;
		CREATE TABLE schema_migrations(unexpected INTEGER);
		UPDATE system_state SET schema_version=1 WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	upgradeStore, err := OpenForUpgrade(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgradeStore.Close()
	if err := upgradeStore.Upgrade(t.Context(), "backup-pre-upgrade-failure"); err == nil {
		t.Fatal("Upgrade succeeded with an incompatible migration-history table")
	}
	var version int
	if err := upgradeStore.db.QueryRow(`SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("schema version after rolled-back migration = %d", version)
	}
}

func TestOpenReadyRejectsMigrationReceiptTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE schema_migrations SET checksum='tampered' WHERE version=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	inspection := Inspect(path)
	if inspection.State != DatabaseRecovery {
		t.Fatalf("inspection = %+v", inspection)
	}
	if _, err := OpenReady(path); !errors.Is(err, ErrDatabaseNotReady) {
		t.Fatalf("OpenReady error = %v", err)
	}
}

func TestVersionSeventeenUpgradeSeedsPinnedLocalesAndPreservesPublishedLocaleSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		UPDATE site_settings SET default_locale='zh-TW',supported_locales_json='["en-US","zh-TW","fr-FR"]',content_multilingual_enabled=1 WHERE singleton=1;
		INSERT INTO product_translations(product_id,locale,name,description,features,specification,revision,updated_at)
		SELECT id,'zh-TW','保留的翻譯','','','',1,'2026-09-15T00:00:00Z' FROM products ORDER BY id LIMIT 1;
		DROP TABLE public_copy_defaults;
		DROP TABLE public_copy_definitions;
		DROP TABLE public_copy_active;
		DROP TABLE public_copy_bundles;
		DROP TABLE taxonomy_translations;
		DROP TABLE taxonomy_content;
		DROP TABLE locale_registry;
		DROP TABLE product_bulk_items;
		DROP TABLE product_bulk_runs;
		ALTER TABLE product_content_metadata DROP COLUMN source_locales_json;
		ALTER TABLE website_versions DROP COLUMN public_copy_overrides_json;
		ALTER TABLE website_versions DROP COLUMN content_editing_enabled;
		ALTER TABLE website_versions DROP COLUMN enabled_locales_json;
		ALTER TABLE website_versions DROP COLUMN default_locale;
		ALTER TABLE website_working DROP COLUMN public_copy_overrides_json;
		ALTER TABLE website_working DROP COLUMN content_editing_enabled;
		ALTER TABLE website_working DROP COLUMN enabled_locales_json;
		ALTER TABLE website_working DROP COLUMN default_locale;
		DELETE FROM schema_migrations WHERE version IN (17,18);
		UPDATE system_state SET schema_version=16 WHERE singleton=1;
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	upgradeStore, err := OpenForUpgrade(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgradeStore.Close()
	if err := upgradeStore.Upgrade(t.Context(), "backup-v16"); err != nil {
		t.Fatal(err)
	}

	var localeCount int
	if err := upgradeStore.db.QueryRow(`SELECT COUNT(*) FROM locale_registry`).Scan(&localeCount); err != nil {
		t.Fatal(err)
	}
	if localeCount != 10 {
		t.Fatalf("locale registry count = %d, want 10", localeCount)
	}
	var workingDefault, workingEnabled, workingOverrides string
	var workingEditing bool
	if err := upgradeStore.db.QueryRow(`SELECT default_locale,enabled_locales_json,content_editing_enabled,public_copy_overrides_json FROM website_working WHERE singleton=1`).Scan(
		&workingDefault, &workingEnabled, &workingEditing, &workingOverrides,
	); err != nil {
		t.Fatal(err)
	}
	if workingDefault != "zh-TW" || workingEnabled != `["en-US","zh-TW","fr-FR"]` || !workingEditing || workingOverrides != "{}" {
		t.Fatalf("working localization = %q %q %v %q", workingDefault, workingEnabled, workingEditing, workingOverrides)
	}
	var activeDefault, activeEnabled string
	var activeEditing bool
	if err := upgradeStore.db.QueryRow(`SELECT default_locale,enabled_locales_json,content_editing_enabled FROM website_versions ORDER BY version DESC LIMIT 1`).Scan(
		&activeDefault, &activeEnabled, &activeEditing,
	); err != nil {
		t.Fatal(err)
	}
	if activeDefault != workingDefault || activeEnabled != workingEnabled || activeEditing != workingEditing {
		t.Fatalf("active localization = %q %q %v", activeDefault, activeEnabled, activeEditing)
	}
	var taxonomyRows int
	if err := upgradeStore.db.QueryRow(`SELECT COUNT(*) FROM taxonomy_content`).Scan(&taxonomyRows); err != nil {
		t.Fatal(err)
	}
	if taxonomyRows == 0 {
		t.Fatal("taxonomy source provenance was not seeded")
	}
	var preservedTranslation string
	if err := upgradeStore.db.QueryRow(`SELECT name FROM product_translations WHERE locale='zh-TW' LIMIT 1`).Scan(&preservedTranslation); err != nil {
		t.Fatal(err)
	}
	if preservedTranslation != "保留的翻譯" {
		t.Fatalf("preserved translation = %q", preservedTranslation)
	}
}

func TestInspectRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE system_state SET schema_version=? WHERE singleton=1`, CurrentSchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	inspection := Inspect(path)
	if inspection.State != DatabaseRecovery {
		t.Fatalf("inspection = %+v", inspection)
	}
}
