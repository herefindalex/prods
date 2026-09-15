package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	MinimumSupportedSchemaVersion = 1
	CurrentSchemaVersion          = 17
)

var (
	ErrMigrationBackupRequired = errors.New("verified pre-upgrade backup receipt is required")
	ErrMigrationHistory        = errors.New("schema migration history is invalid")
)

type migrationDefinition struct {
	Version       int
	Name          string
	SQL           string
	Transactional bool
}

type MigrationInfo struct {
	Version       int
	Name          string
	Checksum      string
	Transactional bool
}

var migrationDefinitions = []migrationDefinition{
	{
		Version:       1,
		Name:          "initial-schema-baseline",
		SQL:           "baseline: schema created by the v1 installer",
		Transactional: true,
	},
	{
		Version:       2,
		Name:          "versioned-migration-history",
		Transactional: true,
		SQL: `CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			checksum TEXT NOT NULL,
			backup_id TEXT NOT NULL,
			applied_at TEXT NOT NULL
		);`,
	},
	{
		Version:       3,
		Name:          "durable-backup-schedule-and-runs",
		Transactional: true,
		SQL: `CREATE TABLE IF NOT EXISTS backup_settings (
			singleton INTEGER PRIMARY KEY CHECK(singleton=1),
			enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
			local_time TEXT NOT NULL,
			retention_daily INTEGER NOT NULL CHECK(retention_daily>=1),
			retention_weekly INTEGER NOT NULL CHECK(retention_weekly>=1),
			retention_monthly INTEGER NOT NULL CHECK(retention_monthly>=1),
			retention_pre_upgrade INTEGER NOT NULL CHECK(retention_pre_upgrade>=1),
			retention_pre_restore INTEGER NOT NULL CHECK(retention_pre_restore>=1),
			version INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO backup_settings(singleton,enabled,local_time,retention_daily,retention_weekly,retention_monthly,retention_pre_upgrade,retention_pre_restore,version,updated_at)
		VALUES(1,1,'03:00',7,8,12,3,3,1,strftime('%Y-%m-%dT%H:%M:%fZ','now'))
		ON CONFLICT(singleton) DO NOTHING;
		CREATE TABLE IF NOT EXISTS backup_runs (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL CHECK(kind IN ('scheduled','manual','pre-upgrade','pre-restore')),
			status TEXT NOT NULL CHECK(status IN ('running','succeeded','failed')),
			scheduled_for TEXT,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			backup_id TEXT,
			size_bytes INTEGER,
			content_verified INTEGER,
			read_only_applied INTEGER,
			error_message TEXT,
			warning_message TEXT,
			UNIQUE(kind,scheduled_for)
		);
		CREATE INDEX IF NOT EXISTS backup_runs_status_started_idx ON backup_runs(status,started_at);`,
	},
	{
		Version:       4,
		Name:          "editable-rfq-traffic-protection",
		Transactional: true,
		SQL: `CREATE TABLE IF NOT EXISTS traffic_settings (
			singleton INTEGER PRIMARY KEY CHECK(singleton=1),
			rfq_limit INTEGER NOT NULL CHECK(rfq_limit BETWEEN 1 AND 10000),
			rfq_window_seconds INTEGER NOT NULL CHECK(rfq_window_seconds BETWEEN 1 AND 86400),
			version INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO traffic_settings(singleton,rfq_limit,rfq_window_seconds,version,updated_at)
		VALUES(1,10,600,1,strftime('%Y-%m-%dT%H:%M:%fZ','now'))
ON CONFLICT(singleton) DO NOTHING;`,
	},
	{
		Version:       5,
		Name:          "rfq-lifecycle-and-typed-recipients",
		Transactional: true,
		SQL: `ALTER TABLE rfqs ADD COLUMN status TEXT NOT NULL DEFAULT 'new'
CHECK(status IN ('new','in_progress','closed','spam'));
ALTER TABLE rfqs ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK(revision >= 1);
ALTER TABLE rfqs ADD COLUMN updated_by TEXT REFERENCES users(id);
ALTER TABLE rfqs ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
UPDATE rfqs SET updated_at=created_at WHERE updated_at='';

ALTER TABLE rfq_recipients RENAME TO rfq_recipients_v1;
CREATE TABLE rfq_recipients (
  rfq_id TEXT NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
  recipient_index INTEGER NOT NULL CHECK(recipient_index >= 0),
  kind TEXT NOT NULL CHECK(kind IN ('user','email')),
  user_id TEXT REFERENCES users(id),
  email TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (rfq_id,recipient_index),
  CHECK(
    (kind='user' AND user_id IS NOT NULL AND email='') OR
    (kind='email' AND user_id IS NULL AND email<>'')
  )
);
INSERT INTO rfq_recipients(rfq_id,recipient_index,kind,user_id,email)
SELECT rfq_id,
       ROW_NUMBER() OVER(PARTITION BY rfq_id ORDER BY recipient)-1,
       'email',NULL,recipient
FROM rfq_recipients_v1
WHERE trim(recipient)<>'';
DROP TABLE rfq_recipients_v1;
CREATE INDEX rfq_recipients_user_idx ON rfq_recipients(user_id);`,
	},
	{
		Version:       6,
		Name:          "versioned-default-rfq-recipients",
		Transactional: true,
		SQL: `CREATE TABLE rfq_recipient_settings (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1),
  revision INTEGER NOT NULL CHECK(revision >= 1),
  updated_by TEXT REFERENCES users(id),
  updated_at TEXT NOT NULL
);
INSERT INTO rfq_recipient_settings(singleton,revision,updated_by,updated_at)
VALUES(1,1,NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'));
CREATE TABLE rfq_default_recipients (
  recipient_index INTEGER PRIMARY KEY CHECK(recipient_index >= 0),
  kind TEXT NOT NULL CHECK(kind IN ('user','email')),
  user_id TEXT REFERENCES users(id),
  email TEXT NOT NULL DEFAULT '',
  CHECK(
    (kind='user' AND user_id IS NOT NULL AND email='') OR
    (kind='email' AND user_id IS NULL AND email<>'')
  )
);
CREATE INDEX rfq_default_recipients_user_idx ON rfq_default_recipients(user_id);`,
	},
	{
		Version:       7,
		Name:          "durable-manual-rfq-delivery",
		Transactional: true,
		SQL: `CREATE TABLE smtp_delivery_attempts (
  id TEXT PRIMARY KEY,
  delivery_key TEXT NOT NULL UNIQUE,
  rfq_id TEXT NOT NULL REFERENCES rfqs(id),
  rfq_revision INTEGER NOT NULL,
  content_version TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  content_json TEXT NOT NULL,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','sending','accepted','partial','failed','unknown')),
  created_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX smtp_delivery_attempts_rfq_idx ON smtp_delivery_attempts(rfq_id,created_at,id);
CREATE TABLE smtp_delivery_recipients (
  attempt_id TEXT NOT NULL REFERENCES smtp_delivery_attempts(id) ON DELETE CASCADE,
  recipient_index INTEGER NOT NULL CHECK(recipient_index >= 0),
  kind TEXT NOT NULL CHECK(kind IN ('user','email')),
  source_user_id TEXT REFERENCES users(id),
  email TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending','sending','accepted','failed','unknown')),
  error_class TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  completed_at TEXT,
  PRIMARY KEY(attempt_id,recipient_index)
);
CREATE INDEX smtp_delivery_recipients_status_idx ON smtp_delivery_recipients(status,attempt_id,recipient_index);`,
	},
	{
		Version:       8,
		Name:          "separate-rfq-export-capability",
		Transactional: true,
		SQL: `INSERT OR IGNORE INTO role_capabilities(role_id,capability)
SELECT id,'rfq.export' FROM roles WHERE system_key='owner';`,
	},
	{
		Version:       9,
		Name:          "durable-asset-lifecycle-and-backup-pins",
		Transactional: true,
		SQL: `CREATE TABLE IF NOT EXISTS asset_gc_orphans (
 asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
 orphaned_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS asset_backup_pins (
 operation_id TEXT NOT NULL,
 asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
 created_at TEXT NOT NULL,
 PRIMARY KEY(operation_id,asset_id)
);
CREATE INDEX IF NOT EXISTS asset_backup_pins_asset_idx ON asset_backup_pins(asset_id);
CREATE TABLE IF NOT EXISTS asset_gc_deletions (
 asset_id TEXT PRIMARY KEY,
 storage_path TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('planned','failed','completed')),
 last_error TEXT NOT NULL DEFAULT '',
 planned_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 completed_at TEXT
);
CREATE INDEX IF NOT EXISTS asset_gc_deletions_state_idx ON asset_gc_deletions(state,updated_at);`,
	},
	{
		Version:       10,
		Name:          "rfq-personal-data-lifecycle",
		Transactional: true,
		SQL: `ALTER TABLE rfqs ADD COLUMN privacy_state TEXT NOT NULL DEFAULT 'retained'
 CHECK(privacy_state IN ('retained','anonymized'));
ALTER TABLE rfqs ADD COLUMN privacy_processed_at TEXT;
ALTER TABLE rfqs ADD COLUMN privacy_processed_by TEXT REFERENCES users(id);`,
	},
	{
		Version:       11,
		Name:          "durable-manual-site-maintenance",
		Transactional: true,
		SQL: `CREATE TABLE site_maintenance (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 active INTEGER NOT NULL CHECK(active IN (0,1)),
 message TEXT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision>=1),
 updated_by TEXT REFERENCES users(id),
 updated_at TEXT NOT NULL
);
INSERT INTO site_maintenance(singleton,active,message,revision,updated_by,updated_at)
VALUES(1,0,'',1,NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'));`,
	},
	{
		Version:       12,
		Name:          "structured-publication-job-errors",
		Transactional: true,
		SQL: `ALTER TABLE publication_intents ADD COLUMN error_message TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS publication_intents_status_updated_idx
ON publication_intents(status,updated_at DESC,id DESC);`,
	},
	{
		Version:       13,
		Name:          "durable-search-engine-submissions",
		Transactional: true,
		SQL: `CREATE TABLE search_integration_settings (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 revision INTEGER NOT NULL CHECK(revision>=1),
 indexnow_enabled INTEGER NOT NULL CHECK(indexnow_enabled IN (0,1)),
 indexnow_key TEXT NOT NULL,
 google_enabled INTEGER NOT NULL CHECK(google_enabled IN (0,1)),
 google_site_url TEXT NOT NULL,
 updated_by TEXT REFERENCES users(id),
 updated_at TEXT NOT NULL
);
INSERT INTO search_integration_settings(singleton,revision,indexnow_enabled,indexnow_key,google_enabled,google_site_url,updated_by,updated_at)
VALUES(1,1,0,'',0,'',NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'));
CREATE TABLE search_notification_state (
 provider TEXT NOT NULL CHECK(provider IN ('indexnow','google_search_console')),
 subject TEXT NOT NULL,
 fingerprint TEXT NOT NULL,
 present INTEGER NOT NULL CHECK(present IN (0,1)),
 updated_at TEXT NOT NULL,
 PRIMARY KEY(provider,subject)
);
CREATE TABLE search_submission_jobs (
 id TEXT PRIMARY KEY,
 provider TEXT NOT NULL CHECK(provider IN ('indexnow','google_search_console')),
 subject TEXT NOT NULL,
 payload_json TEXT NOT NULL,
 dedupe_key TEXT NOT NULL UNIQUE,
 status TEXT NOT NULL CHECK(status IN ('pending','processing','retry_wait','accepted','failed','canceled')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0),
 next_attempt_at TEXT NOT NULL,
 http_status INTEGER NOT NULL DEFAULT 0,
 response_message TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 accepted_at TEXT
);
CREATE INDEX search_submission_jobs_due_idx ON search_submission_jobs(status,next_attempt_at,created_at,id);`,
	},
	{
		Version:       14,
		Name:          "versioned-site-time-zone-settings",
		Transactional: true,
		SQL: `ALTER TABLE site_settings ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK(revision>=1);
ALTER TABLE site_settings ADD COLUMN updated_by TEXT REFERENCES users(id);`,
	},
	{
		Version:       15,
		Name:          "durable-user-invitation-mail-attempts",
		Transactional: true,
		SQL: `CREATE TABLE user_invitation_mail_attempts (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES users(id),
 auth_revision INTEGER NOT NULL,
 token_digest TEXT NOT NULL,
 recipient_email TEXT NOT NULL,
 content_version TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','accepted','failed','unknown')),
 error_class TEXT NOT NULL DEFAULT '',
 error_message TEXT NOT NULL DEFAULT '',
 created_by TEXT NOT NULL REFERENCES users(id),
 created_at TEXT NOT NULL,
 completed_at TEXT,
 CHECK((status='pending' AND completed_at IS NULL) OR (status<>'pending' AND completed_at IS NOT NULL))
);
CREATE INDEX user_invitation_mail_attempts_user_idx ON user_invitation_mail_attempts(user_id,created_at DESC,id DESC);`,
	},
	{
		Version:       16,
		Name:          "product-content-translations",
		Transactional: true,
		SQL: `ALTER TABLE site_settings ADD COLUMN content_multilingual_enabled INTEGER NOT NULL DEFAULT 0 CHECK(content_multilingual_enabled IN (0,1));
CREATE TABLE product_content_metadata (
 product_id TEXT PRIMARY KEY REFERENCES products(id) ON DELETE CASCADE,
 source_locale TEXT NOT NULL
);
INSERT INTO product_content_metadata(product_id,source_locale)
SELECT p.id,s.default_locale FROM products p CROSS JOIN site_settings s WHERE s.singleton=1;
CREATE TABLE product_translations (
 product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
 locale TEXT NOT NULL,
 name TEXT NOT NULL DEFAULT '',
 description TEXT NOT NULL DEFAULT '',
 features TEXT NOT NULL DEFAULT '',
 specification TEXT NOT NULL DEFAULT '',
 revision INTEGER NOT NULL CHECK(revision>=1),
 updated_by TEXT REFERENCES users(id),
 updated_at TEXT NOT NULL,
 PRIMARY KEY(product_id,locale)
);
CREATE INDEX product_translations_locale_idx ON product_translations(locale,product_id);`,
	},
	{
		Version: 17, Name: "v06-localization-public-copy-and-taxonomy", Transactional: true,
		SQL: `CREATE TABLE locale_registry (
 locale TEXT PRIMARY KEY,
 sort_order INTEGER NOT NULL UNIQUE,
 native_name TEXT NOT NULL,
 official_bundle_version TEXT NOT NULL
);
INSERT INTO locale_registry(locale,sort_order,native_name,official_bundle_version) VALUES
 ('en-US',10,'English (United States)','v1'),
 ('zh-TW',20,'繁體中文','v1'),
 ('zh-CN',30,'简体中文','v1'),
 ('ja-JP',40,'日本語','v1'),
 ('ko-KR',50,'한국어','v1'),
 ('de-DE',60,'Deutsch','v1'),
 ('fr-FR',70,'Français','v1'),
 ('it-IT',80,'Italiano','v1'),
 ('es-ES',90,'Español','v1'),
 ('pt-BR',100,'Português (Brasil)','v1');

ALTER TABLE website_working ADD COLUMN default_locale TEXT NOT NULL DEFAULT 'en-US';
ALTER TABLE website_working ADD COLUMN enabled_locales_json TEXT NOT NULL DEFAULT '["en-US"]';
ALTER TABLE website_working ADD COLUMN content_editing_enabled INTEGER NOT NULL DEFAULT 0 CHECK(content_editing_enabled IN (0,1));
ALTER TABLE website_working ADD COLUMN public_copy_overrides_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE website_versions ADD COLUMN default_locale TEXT NOT NULL DEFAULT 'en-US';
ALTER TABLE website_versions ADD COLUMN enabled_locales_json TEXT NOT NULL DEFAULT '["en-US"]';
ALTER TABLE website_versions ADD COLUMN content_editing_enabled INTEGER NOT NULL DEFAULT 0 CHECK(content_editing_enabled IN (0,1));
ALTER TABLE website_versions ADD COLUMN public_copy_overrides_json TEXT NOT NULL DEFAULT '{}';
UPDATE website_working SET
 default_locale=COALESCE((SELECT default_locale FROM site_settings WHERE singleton=1),'en-US'),
 enabled_locales_json=COALESCE((SELECT supported_locales_json FROM site_settings WHERE singleton=1),'["en-US"]'),
 content_editing_enabled=COALESCE((SELECT content_multilingual_enabled FROM site_settings WHERE singleton=1),0);
UPDATE website_versions SET
 default_locale=COALESCE((SELECT default_locale FROM site_settings WHERE singleton=1),'en-US'),
 enabled_locales_json=COALESCE((SELECT supported_locales_json FROM site_settings WHERE singleton=1),'["en-US"]'),
 content_editing_enabled=COALESCE((SELECT content_multilingual_enabled FROM site_settings WHERE singleton=1),0);

ALTER TABLE product_content_metadata ADD COLUMN source_locales_json TEXT NOT NULL DEFAULT '{}';
CREATE TABLE taxonomy_content (
 subject_type TEXT NOT NULL CHECK(subject_type IN ('category','dictionary')),
 subject_id TEXT NOT NULL,
 source_locale TEXT NOT NULL,
 source_locales_json TEXT NOT NULL DEFAULT '{}',
 description TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(subject_type,subject_id)
);
INSERT INTO taxonomy_content(subject_type,subject_id,source_locale)
SELECT 'category',c.id,s.default_locale FROM categories c CROSS JOIN site_settings s WHERE s.singleton=1;
INSERT INTO taxonomy_content(subject_type,subject_id,source_locale)
SELECT 'dictionary',d.id,s.default_locale FROM dictionary_entries d CROSS JOIN site_settings s WHERE s.singleton=1;
CREATE TABLE taxonomy_translations (
 subject_type TEXT NOT NULL CHECK(subject_type IN ('category','dictionary')),
 subject_id TEXT NOT NULL,
 locale TEXT NOT NULL,
 name TEXT NOT NULL DEFAULT '',
 description TEXT NOT NULL DEFAULT '',
 revision INTEGER NOT NULL CHECK(revision>=1),
 updated_by TEXT REFERENCES users(id),
 updated_at TEXT NOT NULL,
 PRIMARY KEY(subject_type,subject_id,locale)
);
CREATE INDEX taxonomy_translations_locale_idx ON taxonomy_translations(locale,subject_type,subject_id);

CREATE TABLE public_copy_bundles (
 version TEXT PRIMARY KEY,
 source TEXT NOT NULL CHECK(source='official'),
 review_status TEXT NOT NULL,
 installed_at TEXT NOT NULL
);
CREATE TABLE public_copy_active (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 official_bundle_version TEXT NOT NULL REFERENCES public_copy_bundles(version),
 activated_at TEXT NOT NULL
);
CREATE TABLE public_copy_definitions (
 copy_key TEXT NOT NULL,
	definition_version INTEGER NOT NULL CHECK(definition_version>=1),
	description TEXT NOT NULL,
	value_kind TEXT NOT NULL CHECK(value_kind IN ('plain','rich','plural','select')),
	required_placeholders_json TEXT NOT NULL DEFAULT '[]',
	allowed_placeholders_json TEXT NOT NULL DEFAULT '[]',
	sample_json TEXT NOT NULL DEFAULT '{}',
	official_bundle_version TEXT NOT NULL REFERENCES public_copy_bundles(version),
	PRIMARY KEY(copy_key,official_bundle_version)
);
CREATE TABLE public_copy_defaults (
	copy_key TEXT NOT NULL,
	locale TEXT NOT NULL REFERENCES locale_registry(locale),
	value TEXT NOT NULL,
	definition_version INTEGER NOT NULL CHECK(definition_version>=1),
	official_bundle_version TEXT NOT NULL REFERENCES public_copy_bundles(version),
	PRIMARY KEY(copy_key,locale,official_bundle_version),
	FOREIGN KEY(copy_key,official_bundle_version) REFERENCES public_copy_definitions(copy_key,official_bundle_version) ON DELETE CASCADE
);
CREATE INDEX public_copy_defaults_locale_idx ON public_copy_defaults(locale,copy_key);`,
	},
}

func SupportedSchemaVersion(version int) bool {
	return version >= MinimumSupportedSchemaVersion && version <= CurrentSchemaVersion
}

func PendingMigrations(fromVersion int) ([]MigrationInfo, error) {
	if !SupportedSchemaVersion(fromVersion) {
		return nil, fmt.Errorf("unsupported source schema version %d", fromVersion)
	}
	definitions, err := validateMigrationDefinitions()
	if err != nil {
		return nil, err
	}
	var pending []MigrationInfo
	for _, definition := range definitions {
		if definition.Version <= fromVersion {
			continue
		}
		pending = append(pending, MigrationInfo{
			Version:       definition.Version,
			Name:          definition.Name,
			Checksum:      migrationChecksum(definition),
			Transactional: definition.Transactional,
		})
	}
	return pending, nil
}

// Upgrade applies the immutable ordered migration set. A completed and
// verified pre-upgrade backup ID is mandatory for an existing installation.
func (s *Store) Upgrade(ctx context.Context, verifiedBackupID string) error {
	verifiedBackupID = strings.TrimSpace(verifiedBackupID)
	if verifiedBackupID == "" {
		return ErrMigrationBackupRequired
	}
	var state string
	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT installation_state,schema_version FROM system_state WHERE singleton=1`).Scan(&state, &version); err != nil {
		return fmt.Errorf("read schema migration state: %w", err)
	}
	if state != string(DatabaseReady) {
		return fmt.Errorf("%w: installation state %q", ErrDatabaseNotReady, state)
	}
	return s.upgrade(ctx, version, verifiedBackupID, false)
}

func (s *Store) upgrade(ctx context.Context, fromVersion int, backupID string, freshInstall bool) error {
	if !SupportedSchemaVersion(fromVersion) {
		return fmt.Errorf("unsupported source schema version %d", fromVersion)
	}
	if fromVersion == CurrentSchemaVersion {
		return s.verifyMigrationHistory(ctx)
	}
	if !freshInstall && strings.TrimSpace(backupID) == "" {
		return ErrMigrationBackupRequired
	}

	definitions, err := validateMigrationDefinitions()
	if err != nil {
		return err
	}
	current := fromVersion
	for _, definition := range definitions {
		if definition.Version <= current {
			continue
		}
		if definition.Version != current+1 {
			return fmt.Errorf("migration sequence gap after schema %d", current)
		}
		if !definition.Transactional {
			return fmt.Errorf("migration %d requires the non-transactional migration executor", definition.Version)
		}
		if err := s.applyTransactionalMigration(ctx, current, definition, backupID); err != nil {
			return fmt.Errorf("migration %d %s: %w", definition.Version, definition.Name, err)
		}
		current = definition.Version
	}
	if current != CurrentSchemaVersion {
		return fmt.Errorf("migration set ended at schema %d, want %d", current, CurrentSchemaVersion)
	}
	return s.verifyMigrationHistory(ctx)
}

func (s *Store) applyTransactionalMigration(ctx context.Context, expectedVersion int, definition migrationDefinition, backupID string) error {
	tx, release, err := s.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	defer tx.Rollback()

	var actualVersion int
	if err := tx.QueryRowContext(ctx, `SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&actualVersion); err != nil {
		return err
	}
	if actualVersion != expectedVersion {
		return fmt.Errorf("schema changed concurrently: got %d want %d", actualVersion, expectedVersion)
	}
	if _, err := tx.ExecContext(ctx, definition.SQL); err != nil {
		return err
	}

	appliedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if expectedVersion == 1 {
		baseline := migrationDefinitions[0]
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum,backup_id,applied_at)
			VALUES(?,?,?,?,?)`, baseline.Version, baseline.Name, migrationChecksum(baseline), backupID, appliedAt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,name,checksum,backup_id,applied_at)
		VALUES(?,?,?,?,?)`, definition.Version, definition.Name, migrationChecksum(definition), backupID, appliedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE system_state SET schema_version=? WHERE singleton=1 AND schema_version=?`, definition.Version, expectedVersion); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) verifyMigrationHistory(ctx context.Context) error {
	return verifyMigrationHistoryDB(ctx, s.db)
}

func verifyMigrationHistoryDB(ctx context.Context, db *sql.DB) error {
	definitions, err := validateMigrationDefinitions()
	if err != nil {
		return err
	}
	var current int
	if err := db.QueryRowContext(ctx, `SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&current); err != nil {
		return fmt.Errorf("%w: %v", ErrMigrationHistory, err)
	}
	if current != CurrentSchemaVersion {
		return fmt.Errorf("%w: schema is %d, want %d", ErrMigrationHistory, current, CurrentSchemaVersion)
	}

	rows, err := db.QueryContext(ctx, `SELECT version,name,checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMigrationHistory, err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		if index >= len(definitions) {
			return fmt.Errorf("%w: unexpected extra migration receipt", ErrMigrationHistory)
		}
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			return fmt.Errorf("%w: %v", ErrMigrationHistory, err)
		}
		expected := definitions[index]
		if version != expected.Version || name != expected.Name || checksum != migrationChecksum(expected) {
			return fmt.Errorf("%w: migration %d receipt mismatch", ErrMigrationHistory, version)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrMigrationHistory, err)
	}
	if index != len(definitions) {
		return fmt.Errorf("%w: have %d receipts, want %d", ErrMigrationHistory, index, len(definitions))
	}
	return nil
}

func validateMigrationDefinitions() ([]migrationDefinition, error) {
	if len(migrationDefinitions) == 0 {
		return nil, errors.New("migration definitions are empty")
	}
	for index, definition := range migrationDefinitions {
		expectedVersion := index + MinimumSupportedSchemaVersion
		if definition.Version != expectedVersion || strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.SQL) == "" {
			return nil, fmt.Errorf("invalid migration definition at index %d", index)
		}
	}
	if migrationDefinitions[len(migrationDefinitions)-1].Version != CurrentSchemaVersion {
		return nil, errors.New("migration definitions do not reach current schema")
	}
	return migrationDefinitions, nil
}

func migrationChecksum(definition migrationDefinition) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\ntransactional=%t\n%s", definition.Version, definition.Name, definition.Transactional, definition.SQL)))
	return hex.EncodeToString(digest[:])
}
