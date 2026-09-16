package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/localization"
	"prods/internal/site"

	_ "modernc.org/sqlite"
)

type Store struct {
	db             *sql.DB
	writer         chan struct{}
	assetLifecycle sync.Mutex
}

func newStore(db *sql.DB) *Store {
	return &Store{db: db, writer: make(chan struct{}, 1)}
}

func (s *Store) beginWrite(ctx context.Context) (*sql.Tx, func(), error) {
	select {
	case s.writer <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	var once sync.Once
	release := func() {
		once.Do(func() { <-s.writer })
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		release()
		return nil, nil, err
	}
	return tx, release, nil
}

func (s *Store) withWriteTx(ctx context.Context, apply func(*sql.Tx) error) error {
	tx, release, err := s.beginWrite(ctx)
	if err != nil {
		return err
	}
	defer release()
	defer tx.Rollback()
	if err := apply(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type DatabaseState string

const (
	DatabaseFresh      DatabaseState = "fresh"
	DatabaseInstalling DatabaseState = "installing"
	DatabaseUpgrade    DatabaseState = "upgrade_required"
	DatabaseReady      DatabaseState = "ready"
	DatabaseRecovery   DatabaseState = "recovery_required"
)

type Inspection struct {
	State         DatabaseState
	Kind          string
	SchemaVersion int
	Reason        string
}

const (
	DatabaseKindSite = "site"
	DatabaseKindPOC  = "poc"
)

var (
	ErrDatabaseExists     = errors.New("database path already exists")
	ErrDatabaseNotReady   = errors.New("database is not ready")
	ErrInstallationState  = errors.New("installation cannot be completed from the current state")
	ErrOwnerAlreadyExists = errors.New("an owner already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type Installation struct {
	OwnerEmail       string
	OwnerDisplayName string
	PasswordHash     string
	DefaultLocale    string
	SupportedLocales []string
	TimeZone         string
	SampleData       *InstallationSampleData
}

type InstallationSampleData struct {
	Version          string
	DatasetVersion   string
	SourceURL        string
	SHA256           string
	Dictionaries     []catalog.DictionaryEntry
	Categories       []catalog.Category
	Specs            []catalog.SpecDefinition
	SpecSets         []catalog.SpecSet
	CategorySpecSets []InstallationCategorySpecSet
	Products         []catalog.Product
	SpecValues       []catalog.SpecValue
}

type InstallationCategorySpecSet struct {
	CategoryID string
	SpecSetID  string
}

type RFQSummary struct {
	ID             string
	Name           string
	Email          string
	Company        string
	Phone          string
	Country        string
	GeneralMessage string
	Status         inquiries.Status
	Revision       int64
	UpdatedBy      string
	UpdatedAt      string
	CreatedAt      string
	PrivacyState   string
	PrivacyAt      string
	PrivacyBy      string
	Items          []inquiries.Item
	Recipients     []inquiries.Recipient
}

// Inspect classifies durable state without creating or migrating anything.
// Any existing but unrecognized database is Recovery Required, never Fresh.
func Inspect(path string) Inspection {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Inspection{State: DatabaseFresh, Reason: "database does not exist"}
	}
	if err != nil {
		return Inspection{State: DatabaseRecovery, Reason: "database path cannot be inspected: " + err.Error()}
	}
	if !info.Mode().IsRegular() {
		return Inspection{State: DatabaseRecovery, Reason: "database path is not a regular file"}
	}
	db, err := openDatabase(path, "ro")
	if err != nil {
		return Inspection{State: DatabaseRecovery, Reason: "database cannot be opened read-only: " + err.Error()}
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA quick_check(1)`).Scan(&integrity); err != nil || integrity != "ok" {
		if err == nil {
			err = fmt.Errorf("quick_check returned %q", integrity)
		}
		return Inspection{State: DatabaseRecovery, Reason: "database integrity check failed: " + err.Error()}
	}
	var state, kind string
	var schemaVersion int
	if err := db.QueryRow(`SELECT installation_state, instance_kind, schema_version FROM system_state WHERE singleton=1`).Scan(&state, &kind, &schemaVersion); err != nil {
		return Inspection{State: DatabaseRecovery, Reason: "installation marker is missing or unreadable: " + err.Error()}
	}
	if !SupportedSchemaVersion(schemaVersion) {
		return Inspection{State: DatabaseRecovery, Reason: fmt.Sprintf("unsupported schema version %d", schemaVersion)}
	}
	if kind != DatabaseKindSite && kind != DatabaseKindPOC {
		return Inspection{State: DatabaseRecovery, Reason: fmt.Sprintf("unsupported instance kind %q", kind)}
	}
	switch state {
	case string(DatabaseInstalling):
		return Inspection{State: DatabaseInstalling, Kind: kind, SchemaVersion: schemaVersion, Reason: "installation has not completed"}
	case string(DatabaseReady):
		if schemaVersion < CurrentSchemaVersion {
			return Inspection{State: DatabaseUpgrade, Kind: kind, SchemaVersion: schemaVersion, Reason: "schema upgrade required"}
		}
		if err := verifyMigrationHistoryDB(context.Background(), db); err != nil {
			return Inspection{State: DatabaseRecovery, Kind: kind, SchemaVersion: schemaVersion, Reason: err.Error()}
		}
		return Inspection{State: DatabaseReady, Kind: kind, SchemaVersion: schemaVersion}
	default:
		return Inspection{State: DatabaseRecovery, Reason: fmt.Sprintf("unknown installation state %q", state)}
	}
}

// Create starts an explicit installation database. It never overwrites or
// adopts an existing path and leaves a durable Installing marker until the
// installer completes all required setup.
func Create(path string) (*Store, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, ErrDatabaseExists
	}
	if err != nil {
		return nil, fmt.Errorf("create sqlite path: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close new sqlite path: %w", err)
	}
	db, err := openDatabase(path, "rw")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store := newStore(db)
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO system_state(singleton,installation_state,instance_kind,schema_version)
		VALUES(1,?,?,1)`, DatabaseInstalling, DatabaseKindSite); err != nil {
		db.Close()
		return nil, fmt.Errorf("write installation marker: %w", err)
	}
	if err := store.upgrade(ctx, 1, "fresh-install", true); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema migration history: %w", err)
	}
	return store, nil
}

// OpenReady opens only a recognized completed installation and never creates
// a database or schema as a side effect.
func OpenReady(path string) (*Store, error) {
	inspection := Inspect(path)
	if inspection.State != DatabaseReady {
		return nil, fmt.Errorf("%w: state=%s: %s", ErrDatabaseNotReady, inspection.State, inspection.Reason)
	}
	db, err := openDatabase(path, "rw")
	if err != nil {
		return nil, fmt.Errorf("open ready sqlite: %w", err)
	}
	store := newStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.verifyMigrationHistory(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("verify schema migration history: %w", err)
	}
	if err := store.InstallOfficialPublicCopy(ctx, localization.OfficialPublicCopyCatalog()); err != nil {
		db.Close()
		return nil, fmt.Errorf("install official public copy: %w", err)
	}
	return store, nil
}

// OpenForUpgrade opens a recognized older schema without mutating it. The
// caller must first create and verify the required pre-upgrade backup, then
// invoke Upgrade with that backup receipt.
func OpenForUpgrade(path string) (*Store, error) {
	inspection := Inspect(path)
	if inspection.State != DatabaseUpgrade {
		return nil, fmt.Errorf("%w: state=%s: %s", ErrDatabaseNotReady, inspection.State, inspection.Reason)
	}
	db, err := openDatabase(path, "rw")
	if err != nil {
		return nil, fmt.Errorf("open upgrade sqlite: %w", err)
	}
	return newStore(db), nil
}

// OpenInstalling opens only the durable, incomplete installation state. It is
// used after a process restart to issue a new bootstrap token and resume the
// installer without adopting or replacing unrelated data.
func OpenInstalling(path string) (*Store, error) {
	inspection := Inspect(path)
	if inspection.State != DatabaseInstalling {
		return nil, fmt.Errorf("%w: state=%s: %s", ErrInstallationState, inspection.State, inspection.Reason)
	}
	db, err := openDatabase(path, "rw")
	if err != nil {
		return nil, fmt.Errorf("open installing sqlite: %w", err)
	}
	return newStore(db), nil
}

// CreatePOC is deliberately named and exists only to keep the isolated PoC
// journey reproducible. Product startup must not call it implicitly.
func CreatePOC(path string) (*Store, error) {
	store, err := Create(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := createSystemCategories(ctx, store.db, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		_ = store.Close()
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO site_settings(
		singleton,default_locale,supported_locales_json,time_zone,revision,updated_by,created_at,updated_at
	) VALUES(1,'en-US','["en-US"]','UTC',1,NULL,?,?) ON CONFLICT(singleton) DO NOTHING`, now, now); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.seed(ctx); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.InstallOfficialPublicCopy(ctx, localization.OfficialPublicCopyCatalog()); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("install POC official public copy: %w", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO taxonomy_content(subject_type,subject_id,source_locale)
		SELECT 'category',c.id,s.default_locale FROM categories c CROSS JOIN site_settings s WHERE s.singleton=1`); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create POC category content: %w", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO taxonomy_content(subject_type,subject_id,source_locale)
		SELECT 'dictionary',d.id,s.default_locale FROM dictionary_entries d CROSS JOIN site_settings s WHERE s.singleton=1`); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("create POC dictionary content: %w", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE system_state SET installation_state=?,instance_kind=? WHERE singleton=1`, DatabaseReady, DatabaseKindPOC); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("complete POC installation: %w", err)
	}
	return store, nil
}

// CompleteInstallation creates the first Owner and the minimum durable site
// state, then flips the installation marker in the same SQLite transaction.
// A crash can therefore expose either Installing or the complete Ready state,
// never a Ready database without its first Owner and system categories.
func (s *Store) CompleteInstallation(ctx context.Context, installation Installation) (identity.User, error) {
	installation.OwnerEmail = strings.TrimSpace(installation.OwnerEmail)
	installation.OwnerDisplayName = strings.TrimSpace(installation.OwnerDisplayName)
	installation.DefaultLocale = strings.TrimSpace(installation.DefaultLocale)
	installation.TimeZone = strings.TrimSpace(installation.TimeZone)
	if installation.OwnerEmail == "" || identity.NormalizeEmail(installation.OwnerEmail) == "" ||
		installation.PasswordHash == "" || installation.DefaultLocale == "" || installation.TimeZone == "" ||
		len(installation.SupportedLocales) == 0 {
		return identity.User{}, ErrInstallationState
	}
	if installation.OwnerDisplayName == "" {
		installation.OwnerDisplayName = installation.OwnerEmail
	}
	if installation.SampleData != nil {
		if strings.TrimSpace(installation.SampleData.Version) == "" ||
			strings.TrimSpace(installation.SampleData.SourceURL) == "" ||
			strings.TrimSpace(installation.SampleData.DatasetVersion) == "" ||
			len(installation.SampleData.SHA256) != 64 || len(installation.SampleData.Products) == 0 {
			return identity.User{}, ErrInstallationState
		}
	}
	defaultLocale, locales, err := localization.NormalizeSupported(installation.DefaultLocale, installation.SupportedLocales)
	if err != nil {
		return identity.User{}, ErrInstallationState
	}
	installation.DefaultLocale = defaultLocale
	if err := s.InstallOfficialPublicCopy(ctx, localization.OfficialPublicCopyCatalog()); err != nil {
		return identity.User{}, fmt.Errorf("install official public copy: %w", err)
	}
	encodedLocales, err := json.Marshal(locales)
	if err != nil {
		return identity.User{}, err
	}
	ownerID, err := randomID("usr")
	if err != nil {
		return identity.User{}, err
	}
	logID, err := randomID("log")
	if err != nil {
		return identity.User{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, release, err := s.beginWrite(ctx)
	if err != nil {
		return identity.User{}, err
	}
	defer release()
	defer tx.Rollback()
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT installation_state FROM system_state WHERE singleton=1`).Scan(&state); err != nil {
		return identity.User{}, err
	}
	if state != string(DatabaseInstalling) {
		return identity.User{}, ErrInstallationState
	}
	var userCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return identity.User{}, err
	}
	if userCount != 0 {
		return identity.User{}, ErrOwnerAlreadyExists
	}
	if err := createOwnerRole(ctx, tx, now); err != nil {
		return identity.User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(
		id,email,email_normalized,display_name,role,status,password_hash,auth_revision,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?)`, ownerID, installation.OwnerEmail, identity.NormalizeEmail(installation.OwnerEmail),
		installation.OwnerDisplayName, identity.RoleOwner, identity.UserActive, installation.PasswordHash, 1, now, now); err != nil {
		return identity.User{}, fmt.Errorf("create owner: %w", err)
	}
	if err := createSystemCategories(ctx, tx, now); err != nil {
		return identity.User{}, err
	}
	if err := createDefaultDocumentTypes(ctx, tx, now); err != nil {
		return identity.User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO site_settings(
		singleton,default_locale,supported_locales_json,time_zone,revision,updated_by,created_at,updated_at
	) VALUES(1,?,?,?,1,?,?,?)`, installation.DefaultLocale, string(encodedLocales), installation.TimeZone, ownerID, now, now); err != nil {
		return identity.User{}, fmt.Errorf("create site settings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE website_working SET default_locale=?,enabled_locales_json=? WHERE singleton=1`, installation.DefaultLocale, string(encodedLocales)); err != nil {
		return identity.User{}, fmt.Errorf("initialize working website locales: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE website_versions SET default_locale=?,enabled_locales_json=?
		WHERE site_epoch=(SELECT active_epoch FROM public_site_state WHERE singleton=1)`, installation.DefaultLocale, string(encodedLocales)); err != nil {
		return identity.User{}, fmt.Errorf("initialize active website locales: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO taxonomy_content(subject_type,subject_id,source_locale) SELECT 'category',id,? FROM categories`, installation.DefaultLocale); err != nil {
		return identity.User{}, fmt.Errorf("create system category content: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO taxonomy_content(subject_type,subject_id,source_locale) SELECT 'dictionary',id,? FROM dictionary_entries`, installation.DefaultLocale); err != nil {
		return identity.User{}, fmt.Errorf("create default dictionary content: %w", err)
	}
	if installation.SampleData != nil {
		seenDictionaryIDs := make(map[string]struct{}, len(installation.SampleData.Dictionaries))
		for index := range installation.SampleData.Dictionaries {
			entry := installation.SampleData.Dictionaries[index]
			entry.Revision = 1
			if entry.SourceLocale == "" {
				entry.SourceLocale = installation.DefaultLocale
			}
			if err := entry.Prepare(); err != nil {
				return identity.User{}, fmt.Errorf("sample dictionary entry %d: %w", index+1, err)
			}
			if entry.Kind == catalog.DictionaryDocumentType {
				return identity.User{}, fmt.Errorf("sample dictionary entry %d cannot replace a system document type", index+1)
			}
			if _, exists := seenDictionaryIDs[entry.ID]; exists {
				return identity.User{}, fmt.Errorf("sample dictionary entry %d duplicates id %q", index+1, entry.ID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO dictionary_entries(
				id,kind,name,slug,status,revision,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?)`, entry.ID, entry.Kind, entry.Name, entry.Slug, entry.Status, 1,
				ownerID, ownerID, now, now); err != nil {
				return identity.User{}, fmt.Errorf("install sample dictionary entry %q: %w", entry.ID, err)
			}
			if err := insertTaxonomySource(ctx, tx, "dictionary", entry.ID, entry.SourceLocale, entry.Description, entry.SourceLocales); err != nil {
				return identity.User{}, err
			}
			seenDictionaryIDs[entry.ID] = struct{}{}
		}
		seenCategoryIDs := make(map[string]struct{}, len(installation.SampleData.Categories))
		seenCategorySlugs := make(map[string]struct{}, len(installation.SampleData.Categories))
		for index := range installation.SampleData.Categories {
			category := installation.SampleData.Categories[index]
			if category.ParentID == "" {
				category.ParentID = "cat_root"
			}
			category.SystemKey = ""
			category.Revision = 1
			if category.SourceLocale == "" {
				category.SourceLocale = installation.DefaultLocale
			}
			if err := category.Prepare(); err != nil {
				return identity.User{}, fmt.Errorf("sample category %d: %w", index+1, err)
			}
			if _, exists := seenCategoryIDs[category.ID]; exists || category.ID == "cat_root" || category.ID == catalog.UncategorizedCategoryID {
				return identity.User{}, fmt.Errorf("sample category %d duplicates or replaces a system category", index+1)
			}
			slugKey := category.ParentID + "\x00" + category.Slug
			if _, exists := seenCategorySlugs[slugKey]; exists {
				return identity.User{}, fmt.Errorf("sample category %d duplicates a sibling slug", index+1)
			}
			if err := requireActiveCategory(ctx, tx, category.ParentID); err != nil {
				return identity.User{}, fmt.Errorf("sample category %d parent: %w", index+1, err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO categories(
				id,parent_id,system_key,name,slug,status,revision,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,NULL,?,?,?,?,?,?,?,?)`, category.ID, category.ParentID, category.Name, category.Slug,
				category.Status, 1, ownerID, ownerID, now, now); err != nil {
				return identity.User{}, fmt.Errorf("install sample category %q: %w", category.ID, err)
			}
			if err := insertTaxonomySource(ctx, tx, "category", category.ID, category.SourceLocale, category.Description, category.SourceLocales); err != nil {
				return identity.User{}, err
			}
			seenCategoryIDs[category.ID] = struct{}{}
			seenCategorySlugs[slugKey] = struct{}{}
		}
		seenSpecIDs := make(map[string]struct{}, len(installation.SampleData.Specs))
		for index := range installation.SampleData.Specs {
			spec := installation.SampleData.Specs[index]
			spec.ID = strings.TrimSpace(spec.ID)
			spec.Name = strings.TrimSpace(spec.Name)
			spec.PreferredUnit = strings.TrimSpace(spec.PreferredUnit)
			if spec.ID == "" || spec.Name == "" || (spec.Status != "" && spec.Status != catalog.EntryActive && spec.Status != catalog.EntryDisabled) {
				return identity.User{}, fmt.Errorf("sample specification %d: %w", index+1, catalog.ErrInvalidSpec)
			}
			if spec.Status == "" {
				spec.Status = catalog.EntryActive
			}
			if spec.SemanticVer < 1 {
				return identity.User{}, fmt.Errorf("sample specification %d: %w", index+1, catalog.ErrInvalidSpec)
			}
			if _, duplicate := seenSpecIDs[spec.ID]; duplicate {
				return identity.User{}, fmt.Errorf("sample specification %d duplicates id %q", index+1, spec.ID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO spec_definitions(
				id,name,preferred_unit,filterable,semantic_version,status,revision,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, spec.ID, spec.Name, spec.PreferredUnit, spec.Filterable, spec.SemanticVer,
				spec.Status, 1, ownerID, ownerID, now, now); err != nil {
				return identity.User{}, fmt.Errorf("install sample specification %q: %w", spec.ID, err)
			}
			seenSpecIDs[spec.ID] = struct{}{}
		}
		seenSpecSetIDs := make(map[string]struct{}, len(installation.SampleData.SpecSets))
		for index := range installation.SampleData.SpecSets {
			set := installation.SampleData.SpecSets[index]
			set.ID = strings.TrimSpace(set.ID)
			set.Name = strings.TrimSpace(set.Name)
			if set.ID == "" || set.Name == "" || len(set.SpecIDs) == 0 ||
				(set.Status != "" && set.Status != catalog.EntryActive && set.Status != catalog.EntryDisabled) {
				return identity.User{}, fmt.Errorf("sample specification set %d: %w", index+1, catalog.ErrInvalidSpec)
			}
			if set.Status == "" {
				set.Status = catalog.EntryActive
			}
			if _, duplicate := seenSpecSetIDs[set.ID]; duplicate {
				return identity.User{}, fmt.Errorf("sample specification set %d duplicates id %q", index+1, set.ID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO spec_sets(
				id,name,status,revision,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?)`, set.ID, set.Name, set.Status, 1, ownerID, ownerID, now, now); err != nil {
				return identity.User{}, fmt.Errorf("install sample specification set %q: %w", set.ID, err)
			}
			members := make(map[string]struct{}, len(set.SpecIDs))
			for memberIndex, specID := range set.SpecIDs {
				specID = strings.TrimSpace(specID)
				if _, exists := seenSpecIDs[specID]; !exists {
					return identity.User{}, fmt.Errorf("sample specification set %q references unknown specification %q", set.ID, specID)
				}
				if _, duplicate := members[specID]; duplicate {
					return identity.User{}, fmt.Errorf("sample specification set %q duplicates specification %q", set.ID, specID)
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO spec_set_members(spec_set_id,spec_id,sort_order) VALUES(?,?,?)`, set.ID, specID, memberIndex); err != nil {
					return identity.User{}, err
				}
				members[specID] = struct{}{}
			}
			seenSpecSetIDs[set.ID] = struct{}{}
		}
		seenCategoryAssignments := make(map[string]struct{}, len(installation.SampleData.CategorySpecSets))
		for index, assignment := range installation.SampleData.CategorySpecSets {
			if _, exists := seenCategoryIDs[assignment.CategoryID]; !exists {
				return identity.User{}, fmt.Errorf("sample category specification assignment %d references unknown category %q", index+1, assignment.CategoryID)
			}
			if _, exists := seenSpecSetIDs[assignment.SpecSetID]; !exists {
				return identity.User{}, fmt.Errorf("sample category specification assignment %d references unknown set %q", index+1, assignment.SpecSetID)
			}
			if _, duplicate := seenCategoryAssignments[assignment.CategoryID]; duplicate {
				return identity.User{}, fmt.Errorf("sample category %q has multiple specification sets", assignment.CategoryID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO category_spec_sets(category_id,spec_set_id) VALUES(?,?)`, assignment.CategoryID, assignment.SpecSetID); err != nil {
				return identity.User{}, err
			}
			seenCategoryAssignments[assignment.CategoryID] = struct{}{}
		}
		seenIDs := make(map[string]struct{}, len(installation.SampleData.Products))
		seenIdentities := make(map[string]struct{}, len(installation.SampleData.Products))
		for index := range installation.SampleData.Products {
			product := installation.SampleData.Products[index]
			if product.CategoryID == "" {
				product.CategoryID = catalog.UncategorizedCategoryID
			}
			if product.SourceLocale == "" {
				product.SourceLocale = installation.DefaultLocale
			}
			product.Revision = 1
			product.CreatedBy = ownerID
			product.UpdatedBy = ownerID
			if product.Slug == "" {
				product.Slug = catalog.SuggestedSlug(product.PartNumber, product.ID)
			}
			if product.Status == "" {
				product.Status = catalog.Hidden
			}
			if err := product.Prepare(); err != nil {
				return identity.User{}, fmt.Errorf("sample product %d: %w", index+1, err)
			}
			if err := validateProductReferences(ctx, tx, &product); err != nil {
				return identity.User{}, fmt.Errorf("sample product %d references: %w", index+1, err)
			}
			if err := product.Prepare(); err != nil {
				return identity.User{}, fmt.Errorf("sample product %d: %w", index+1, err)
			}
			if product.Status != catalog.Hidden && product.Status != catalog.Published {
				return identity.User{}, fmt.Errorf("sample product %d: %w", index+1, catalog.ErrInvalidProduct)
			}
			if _, exists := seenIDs[product.ID]; exists {
				return identity.User{}, fmt.Errorf("sample product %d duplicates product id %q", index+1, product.ID)
			}
			if product.RecordState == catalog.RecordCurrent {
				identityKey := product.IdentityMaker + "\x00" + product.IdentityPart
				if _, exists := seenIdentities[identityKey]; exists {
					return identity.User{}, fmt.Errorf("sample product %d duplicates a current product identity", index+1)
				}
				seenIdentities[identityKey] = struct{}{}
			}
			seenIDs[product.ID] = struct{}{}
			if err := insertProductTx(ctx, tx, product, now); err != nil {
				return identity.User{}, fmt.Errorf("install sample product %q: %w", product.ID, err)
			}
			if product.Status == catalog.Published {
				if err := appendPublicationIntent(ctx, tx, product.ID, product.Revision, "installation.sample_data", now); err != nil {
					return identity.User{}, err
				}
			}
		}
		seenSpecValues := make(map[string]struct{}, len(installation.SampleData.SpecValues))
		for index, value := range installation.SampleData.SpecValues {
			value.ProductID = strings.TrimSpace(value.ProductID)
			value.SpecID = strings.TrimSpace(value.SpecID)
			value.RawValue = strings.TrimSpace(value.RawValue)
			value.SourceLocale = strings.TrimSpace(value.SourceLocale)
			if value.SourceLocale == "" {
				value.SourceLocale = installation.DefaultLocale
			}
			if _, exists := seenIDs[value.ProductID]; !exists || value.SpecID == "" || value.RawValue == "" {
				return identity.User{}, fmt.Errorf("sample specification value %d: %w", index+1, catalog.ErrInvalidSpec)
			}
			if _, exists := seenSpecIDs[value.SpecID]; !exists {
				return identity.User{}, fmt.Errorf("sample specification value %d references unknown specification %q", index+1, value.SpecID)
			}
			key := value.ProductID + "\x00" + value.SpecID + "\x00" + value.SourceLocale
			if _, duplicate := seenSpecValues[key]; duplicate {
				return identity.User{}, fmt.Errorf("sample specification value %d is duplicated", index+1)
			}
			var applicable int
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
				SELECT 1 FROM products p
				JOIN category_spec_sets cs ON cs.category_id=p.category_id
				JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id
				WHERE p.id=? AND sm.spec_id=?
			)`, value.ProductID, value.SpecID).Scan(&applicable); err != nil {
				return identity.User{}, err
			}
			if applicable == 0 {
				return identity.User{}, fmt.Errorf("sample specification value %d is not applicable to product %q", index+1, value.ProductID)
			}
			valueID := value.ID
			if valueID == "" {
				valueID = fmt.Sprintf("spv_sample_%06d", index+1)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO product_spec_values(
				id,product_id,spec_id,raw_value,source_locale,source_revision,active,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,1,1,?,?,?,?)`, valueID, value.ProductID, value.SpecID, value.RawValue, value.SourceLocale,
				ownerID, ownerID, now, now); err != nil {
				return identity.User{}, fmt.Errorf("install sample specification value for product %q: %w", value.ProductID, err)
			}
			seenSpecValues[key] = struct{}{}
		}
	}
	details := map[string]any{}
	if installation.SampleData != nil {
		details["sample_data"] = map[string]any{
			"version": installation.SampleData.Version, "dataset_version": installation.SampleData.DatasetVersion, "source_url": installation.SampleData.SourceURL,
			"sha256": installation.SampleData.SHA256, "dictionary_count": len(installation.SampleData.Dictionaries),
			"category_count": len(installation.SampleData.Categories), "spec_count": len(installation.SampleData.Specs),
			"spec_set_count": len(installation.SampleData.SpecSets), "spec_value_count": len(installation.SampleData.SpecValues),
			"product_count": len(installation.SampleData.Products),
		}
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return identity.User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin_log(
		id,actor_id,action,target_type,target_id,result,details_json,created_at
	) VALUES(?,?,?,?,?,?,?,?)`, logID, ownerID, "installation.completed", "installation", "site", "success", string(detailsJSON), now); err != nil {
		return identity.User{}, fmt.Errorf("record installation audit: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE system_state SET installation_state=?
		WHERE singleton=1 AND installation_state=?`, DatabaseReady, DatabaseInstalling)
	if err != nil {
		return identity.User{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return identity.User{}, ErrInstallationState
	}
	if err := tx.Commit(); err != nil {
		return identity.User{}, err
	}
	owner := identity.User{
		ID: ownerID, Email: installation.OwnerEmail, DisplayName: installation.OwnerDisplayName,
		Role: identity.RoleOwner, Status: identity.UserActive, PasswordHash: installation.PasswordHash, AuthRevision: 1,
		Capabilities: make(map[identity.Capability]bool, len(identity.OwnerCapabilities)),
	}
	for _, capability := range identity.OwnerCapabilities {
		owner.Capabilities[capability] = true
	}
	return owner, nil
}

func (s *Store) ActiveUserByEmail(ctx context.Context, email string) (identity.User, error) {
	var user identity.User
	err := s.db.QueryRowContext(ctx, `SELECT id,email,display_name,role,status,password_hash,auth_revision
		FROM users WHERE email_normalized=? AND status='active' AND password_hash IS NOT NULL`,
		identity.NormalizeEmail(email)).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role,
		&user.Status, &user.PasswordHash, &user.AuthRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.User{}, ErrInvalidCredentials
	}
	if err != nil {
		return identity.User{}, err
	}
	user.Capabilities, err = loadCapabilities(ctx, s.db, user.Role)
	return user, err
}

func (s *Store) CreateAdminSession(ctx context.Context, rawToken, csrfToken string, user identity.User, expiresAt time.Time) error {
	if rawToken == "" || csrfToken == "" || user.ID == "" || user.Status != identity.UserActive || expiresAt.IsZero() {
		return ErrInvalidCredentials
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO admin_sessions(
			token_digest,user_id,csrf_token,auth_revision,expires_at,revoked_at,created_at
		) VALUES(?,?,?,?,?,NULL,?)`, identity.TokenDigest(rawToken), user.ID, csrfToken, user.AuthRevision,
			expiresAt.UTC().Format(time.RFC3339Nano), now)
		return err
	})
}

func (s *Store) AdminSession(ctx context.Context, rawToken string, now time.Time) (identity.AdminSession, error) {
	var result identity.AdminSession
	var expiresAt string
	err := s.db.QueryRowContext(ctx, `SELECT
		u.id,u.email,u.display_name,u.role,u.status,u.password_hash,u.auth_revision,
		s.csrf_token,s.expires_at
		FROM admin_sessions s
		JOIN users u ON u.id=s.user_id
		WHERE s.token_digest=? AND s.revoked_at IS NULL AND s.auth_revision=u.auth_revision
		  AND u.status='active' AND s.expires_at>?`, identity.TokenDigest(rawToken),
		now.UTC().Format(time.RFC3339Nano)).Scan(&result.User.ID, &result.User.Email, &result.User.DisplayName,
		&result.User.Role, &result.User.Status, &result.User.PasswordHash, &result.User.AuthRevision,
		&result.CSRFToken, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.AdminSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return identity.AdminSession{}, err
	}
	result.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return identity.AdminSession{}, fmt.Errorf("parse session expiry: %w", err)
	}
	result.User.Capabilities, err = loadCapabilities(ctx, s.db, result.User.Role)
	if err != nil {
		return identity.AdminSession{}, err
	}
	return result, nil
}

func (s *Store) RevokeAdminSession(ctx context.Context, rawToken string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at=?
			WHERE token_digest=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339Nano), identity.TokenDigest(rawToken))
		return err
	})
}

func openDatabase(path, mode string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	location, err := databaseFileURL(filepath.ToSlash(abs), runtime.GOOS)
	if err != nil {
		return nil, err
	}
	query := location.Query()
	query.Set("mode", mode)
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	if mode != "ro" {
		query.Add("_pragma", "journal_mode(WAL)")
		query.Add("_pragma", "synchronous(FULL)")
	} else {
		query.Add("_pragma", "query_only(1)")
	}
	location.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func databaseFileURL(absolutePath, goos string) (*url.URL, error) {
	if absolutePath == "" {
		return nil, errors.New("sqlite path is empty")
	}
	location := &url.URL{Scheme: "file"}
	if goos != "windows" {
		if !strings.HasPrefix(absolutePath, "/") {
			return nil, errors.New("sqlite path is not absolute")
		}
		location.Path = absolutePath
		return location, nil
	}
	if strings.HasPrefix(absolutePath, "//") {
		parts := strings.SplitN(strings.TrimPrefix(absolutePath, "//"), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, errors.New("sqlite UNC path is invalid")
		}
		location.Host = parts[0]
		location.Path = "/" + parts[1]
		return location, nil
	}
	if len(absolutePath) < 3 || absolutePath[1] != ':' || absolutePath[2] != '/' {
		return nil, errors.New("sqlite Windows path is not absolute")
	}
	location.Path = "/" + absolutePath
	return location, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Snapshot writes a transactionally consistent SQLite image. The destination
// must not exist; callers publish their own manifest only after all companion
// roots have been copied and verified.
func (s *Store) Snapshot(ctx context.Context, destination string) error {
	if _, err := os.Stat(destination); err == nil {
		return ErrDatabaseExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("sqlite snapshot: %w", err)
	}
	return nil
}

func (s *Store) Ready(ctx context.Context) error {
	var state string
	if err := s.db.QueryRowContext(ctx, `SELECT installation_state FROM system_state WHERE singleton=1`).Scan(&state); err != nil {
		return err
	}
	if state != string(DatabaseReady) {
		return ErrDatabaseNotReady
	}
	return nil
}

// DefaultLocale returns the installed site's authoritative default locale.
func (s *Store) DefaultLocale(ctx context.Context) (string, error) {
	var locale string
	if err := s.db.QueryRowContext(ctx, `SELECT default_locale FROM site_settings WHERE singleton=1`).Scan(&locale); err != nil {
		return "", err
	}
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return "", errors.New("site default locale is empty")
	}
	return locale, nil
}

func (s *Store) SiteLocales(ctx context.Context) (string, []string, error) {
	var defaultLocale, encoded string
	if err := s.db.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json FROM site_settings WHERE singleton=1`).Scan(&defaultLocale, &encoded); err != nil {
		return "", nil, err
	}
	var supported []string
	if err := json.Unmarshal([]byte(encoded), &supported); err != nil {
		return "", nil, err
	}
	return localization.NormalizeSupported(defaultLocale, supported)
}

type contextExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func createSystemCategories(ctx context.Context, execer contextExecer, now string) error {
	if _, err := execer.ExecContext(ctx, `INSERT INTO categories(
		id,parent_id,system_key,name,slug,status,revision,created_at,updated_at
	) VALUES('cat_root',NULL,'root','All Products','all-products','active',1,?,?),
		      ('cat_uncategorized','cat_root','uncategorized','Uncategorized','uncategorized','active',1,?,?)`, now, now, now, now); err != nil {
		return fmt.Errorf("create system categories: %w", err)
	}
	return nil
}

func createDefaultDocumentTypes(ctx context.Context, execer contextExecer, now string) error {
	defaults := []struct{ id, name, slug string }{
		{"dt_datasheet", "Datasheet", "datasheet"},
		{"dt_product_brief", "Product Brief", "product-brief"},
		{"dt_application_note", "Application Note", "application-note"},
		{"dt_other", "Other", "other"},
	}
	for _, item := range defaults {
		if _, err := execer.ExecContext(ctx, `INSERT INTO dictionary_entries(
			id,kind,name,slug,status,revision,created_at,updated_at
		) VALUES(?,'document_type',?,?, 'active',1,?,?) ON CONFLICT(id) DO NOTHING`, item.id, item.name, item.slug, now, now); err != nil {
			return fmt.Errorf("create default document type %s: %w", item.id, err)
		}
	}
	return nil
}

func createOwnerRole(ctx context.Context, execer contextExecer, now string) error {
	if _, err := execer.ExecContext(ctx, `INSERT INTO roles(id,name,system_key,status,created_at,updated_at)
		VALUES(?, 'Owner', 'owner', 'active', ?, ?)
		ON CONFLICT(id) DO NOTHING`, identity.RoleOwner, now, now); err != nil {
		return fmt.Errorf("create owner role: %w", err)
	}
	for _, capability := range identity.OwnerCapabilities {
		if _, err := execer.ExecContext(ctx, `INSERT INTO role_capabilities(role_id,capability)
			VALUES(?,?) ON CONFLICT(role_id,capability) DO NOTHING`, identity.RoleOwner, capability); err != nil {
			return fmt.Errorf("grant owner capability %s: %w", capability, err)
		}
	}
	return nil
}

type contextQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadCapabilities(ctx context.Context, queryer contextQueryer, roleID string) (map[identity.Capability]bool, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT rc.capability
		FROM role_capabilities rc JOIN roles r ON r.id=rc.role_id
		WHERE rc.role_id=? AND r.status='active'`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[identity.Capability]bool)
	for rows.Next() {
		var capability identity.Capability
		if err := rows.Scan(&capability); err != nil {
			return nil, err
		}
		result[capability] = true
	}
	return result, rows.Err()
}

func (s *Store) migrate(ctx context.Context) error {
	const schema = `CREATE TABLE IF NOT EXISTS products (
		id TEXT PRIMARY KEY,
		category_id TEXT NOT NULL DEFAULT 'cat_uncategorized' REFERENCES categories(id),
		part_number TEXT NOT NULL,
		identity_part_number TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		manufacturer_id TEXT,
		manufacturer TEXT NOT NULL,
		identity_manufacturer TEXT NOT NULL DEFAULT '',
		brand_id TEXT,
		brand TEXT NOT NULL DEFAULT '',
		lifecycle_id TEXT,
		package_form_factor TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL,
		features TEXT NOT NULL DEFAULT '',
		specification TEXT NOT NULL,
		document_url TEXT NOT NULL,
		status TEXT NOT NULL CHECK (status IN ('published','hidden')),
		record_state TEXT NOT NULL DEFAULT 'current' CHECK (record_state IN ('current','archived')),
		revision INTEGER NOT NULL,
		cloned_from_id TEXT REFERENCES products(id),
		created_by TEXT REFERENCES users(id),
		updated_by TEXT REFERENCES users(id),
		search_folded TEXT NOT NULL,
		search_projection_version TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		CHECK(record_state = 'current' OR status = 'hidden')
	);
CREATE INDEX IF NOT EXISTS products_public_search
ON products(record_state, status, search_projection_version, search_folded);
CREATE TABLE IF NOT EXISTS product_url_names (
    product_id TEXT PRIMARY KEY REFERENCES products(id),
    slug TEXT NOT NULL,
    custom_path TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS product_url_names_slug ON product_url_names(slug);
	CREATE INDEX IF NOT EXISTS products_current_identity
		ON products(record_state, identity_manufacturer, identity_part_number);
	CREATE TABLE IF NOT EXISTS rfqs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		company TEXT NOT NULL,
		phone TEXT NOT NULL,
		country TEXT NOT NULL,
		general_message TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS rfq_items (
		rfq_id TEXT NOT NULL REFERENCES rfqs(id),
		item_index INTEGER NOT NULL,
		kind TEXT NOT NULL CHECK (kind IN ('catalog','requested')),
		product_id TEXT,
		requested TEXT NOT NULL,
		raw_query TEXT NOT NULL,
		quantity TEXT NOT NULL,
		notes TEXT NOT NULL,
		public_snapshot_json TEXT,
		PRIMARY KEY (rfq_id, item_index)
	);
	CREATE TABLE IF NOT EXISTS rfq_recipients (
		rfq_id TEXT NOT NULL REFERENCES rfqs(id),
		recipient TEXT NOT NULL,
		PRIMARY KEY (rfq_id, recipient)
	);
	CREATE TABLE IF NOT EXISTS idempotency_receipts (
		idempotency_key TEXT PRIMARY KEY,
		canonical_version TEXT NOT NULL,
		request_hash TEXT NOT NULL,
		rfq_id TEXT NOT NULL REFERENCES rfqs(id),
		receipt_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS system_state (
		singleton INTEGER PRIMARY KEY CHECK(singleton=1),
		installation_state TEXT NOT NULL CHECK(installation_state IN ('installing','ready')),
		instance_kind TEXT NOT NULL CHECK(instance_kind IN ('site','poc')),
		schema_version INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS roles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		system_key TEXT UNIQUE,
		status TEXT NOT NULL CHECK(status IN ('active','disabled')),
		revision INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS role_capabilities (
		role_id TEXT NOT NULL REFERENCES roles(id),
		capability TEXT NOT NULL,
		PRIMARY KEY(role_id,capability)
	);
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		email_normalized TEXT NOT NULL UNIQUE,
		display_name TEXT NOT NULL,
		role TEXT NOT NULL REFERENCES roles(id),
		status TEXT NOT NULL CHECK(status IN ('active','disabled')),
		password_hash TEXT,
		auth_revision INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS admin_sessions (
		token_digest TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id),
		csrf_token TEXT NOT NULL,
		auth_revision INTEGER NOT NULL,
		expires_at TEXT NOT NULL,
		revoked_at TEXT,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS admin_sessions_user
		ON admin_sessions(user_id, revoked_at, expires_at);
	CREATE TABLE IF NOT EXISTS set_password_tokens (
		token_digest TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES users(id),
		auth_revision INTEGER NOT NULL,
		expires_at TEXT NOT NULL,
		used_at TEXT,
		created_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS set_password_tokens_user ON set_password_tokens(user_id,used_at,expires_at);
	CREATE TABLE IF NOT EXISTS site_settings (
		singleton INTEGER PRIMARY KEY CHECK(singleton=1),
		default_locale TEXT NOT NULL,
		supported_locales_json TEXT NOT NULL,
		time_zone TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS categories (
		id TEXT PRIMARY KEY,
		parent_id TEXT REFERENCES categories(id),
		system_key TEXT UNIQUE,
		name TEXT NOT NULL,
		slug TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('active','disabled')),
		revision INTEGER NOT NULL,
		created_by TEXT REFERENCES users(id),
		updated_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		CHECK(system_key IN ('root','uncategorized') OR system_key IS NULL)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS categories_sibling_slug
		ON categories(COALESCE(parent_id,''),slug);
	CREATE TABLE IF NOT EXISTS dictionary_entries (
		id TEXT PRIMARY KEY,
		kind TEXT NOT NULL CHECK(kind IN ('manufacturer','brand','application','lifecycle','document_type')),
		name TEXT NOT NULL,
		slug TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL CHECK(status IN ('active','disabled')),
		revision INTEGER NOT NULL,
		created_by TEXT REFERENCES users(id),
		updated_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS dictionary_entries_kind_status ON dictionary_entries(kind,status,name);
	CREATE TABLE IF NOT EXISTS product_applications (
		product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
		application_id TEXT NOT NULL REFERENCES dictionary_entries(id),
		sort_order INTEGER NOT NULL,
		PRIMARY KEY(product_id,application_id)
	);
	CREATE INDEX IF NOT EXISTS product_applications_application
		ON product_applications(application_id,product_id);
	CREATE TABLE IF NOT EXISTS spec_definitions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		preferred_unit TEXT NOT NULL,
		filterable INTEGER NOT NULL CHECK(filterable IN (0,1)),
		semantic_version INTEGER NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('active','disabled')),
		revision INTEGER NOT NULL,
		created_by TEXT REFERENCES users(id),
		updated_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS spec_sets (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('active','disabled')),
		revision INTEGER NOT NULL,
		created_by TEXT REFERENCES users(id),
		updated_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS spec_set_members (
		spec_set_id TEXT NOT NULL REFERENCES spec_sets(id),
		spec_id TEXT NOT NULL REFERENCES spec_definitions(id),
		sort_order INTEGER NOT NULL,
		PRIMARY KEY(spec_set_id,spec_id)
	);
	CREATE TABLE IF NOT EXISTS category_spec_sets (
		category_id TEXT PRIMARY KEY REFERENCES categories(id),
		spec_set_id TEXT NOT NULL REFERENCES spec_sets(id)
	);
	CREATE TABLE IF NOT EXISTS product_spec_values (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL REFERENCES products(id),
		spec_id TEXT NOT NULL REFERENCES spec_definitions(id),
		raw_value TEXT NOT NULL,
		source_locale TEXT NOT NULL,
		source_revision INTEGER NOT NULL,
		active INTEGER NOT NULL CHECK(active IN (0,1)),
		created_by TEXT REFERENCES users(id),
		updated_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		UNIQUE(product_id,spec_id,source_locale)
	);
	CREATE TABLE IF NOT EXISTS normalized_values (
		id TEXT PRIMARY KEY,
		spec_value_id TEXT NOT NULL REFERENCES product_spec_values(id),
		value_json TEXT NOT NULL,
		source_revision INTEGER NOT NULL,
		spec_semantic_version INTEGER NOT NULL,
		normalizer_version TEXT NOT NULL,
		source TEXT NOT NULL CHECK(source IN ('automatic','manual')),
		status TEXT NOT NULL CHECK(status IN ('current','stale','failed')),
		failure_reason TEXT NOT NULL,
		created_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS normalized_values_lookup ON normalized_values(spec_value_id,status,source);
CREATE TABLE IF NOT EXISTS assets (
		id TEXT PRIMARY KEY,
		owner_type TEXT NOT NULL,
		owner_id TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		checksum TEXT NOT NULL,
		created_by TEXT REFERENCES users(id),
 created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS asset_gc_orphans (
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
CREATE INDEX IF NOT EXISTS asset_gc_deletions_state_idx ON asset_gc_deletions(state,updated_at);
CREATE TABLE IF NOT EXISTS product_documents (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL REFERENCES products(id),
		label TEXT NOT NULL,
		document_type_id TEXT NOT NULL REFERENCES dictionary_entries(id),
		asset_id TEXT REFERENCES assets(id),
		external_url TEXT,
		language TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		created_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		CHECK((asset_id IS NOT NULL) <> (external_url IS NOT NULL))
	);
	CREATE TABLE IF NOT EXISTS product_images (
		id TEXT PRIMARY KEY,
		product_id TEXT NOT NULL REFERENCES products(id),
		asset_id TEXT REFERENCES assets(id),
		external_url TEXT,
		alt_text TEXT NOT NULL,
		sort_order INTEGER NOT NULL,
		is_primary INTEGER NOT NULL CHECK(is_primary IN (0,1)),
		created_by TEXT REFERENCES users(id),
		created_at TEXT NOT NULL,
		CHECK((asset_id IS NOT NULL) <> (external_url IS NOT NULL))
	);
	CREATE INDEX IF NOT EXISTS product_images_product ON product_images(product_id,sort_order,id);
	CREATE TABLE IF NOT EXISTS admin_log (
		id TEXT PRIMARY KEY,
		actor_id TEXT REFERENCES users(id),
		action TEXT NOT NULL,
		target_type TEXT NOT NULL,
		target_id TEXT NOT NULL,
		result TEXT NOT NULL,
		details_json TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE TRIGGER IF NOT EXISTS admin_log_immutable_update
		BEFORE UPDATE ON admin_log BEGIN SELECT RAISE(ABORT, 'admin_log is immutable'); END;
	CREATE TRIGGER IF NOT EXISTS admin_log_immutable_delete
		BEFORE DELETE ON admin_log BEGIN SELECT RAISE(ABORT, 'admin_log is immutable'); END;
CREATE TABLE IF NOT EXISTS publication_intents (
		id TEXT PRIMARY KEY,
		entity_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		desired_revision INTEGER NOT NULL,
		cause TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('pending','processing','completed','superseded','failed')),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS website_working (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1),
  revision INTEGER NOT NULL,
  config_json TEXT NOT NULL,
  updated_by TEXT REFERENCES users(id),
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS website_versions (
  version INTEGER PRIMARY KEY AUTOINCREMENT,
  source_working_revision INTEGER NOT NULL,
  site_epoch INTEGER NOT NULL UNIQUE,
  config_json TEXT NOT NULL,
  created_by TEXT REFERENCES users(id),
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS website_runtime_state (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1),
  custom_css_disabled INTEGER NOT NULL CHECK(custom_css_disabled IN (0,1)),
  generation INTEGER NOT NULL,
  updated_by TEXT REFERENCES users(id),
  updated_at TEXT NOT NULL
);
INSERT INTO website_runtime_state(singleton,custom_css_disabled,generation,updated_at)
VALUES(1,0,1,CURRENT_TIMESTAMP) ON CONFLICT(singleton) DO NOTHING;
CREATE TABLE IF NOT EXISTS public_site_state (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1),
  active_epoch INTEGER NOT NULL,
  product_prefix TEXT NOT NULL,
  url_pattern TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
INSERT INTO public_site_state(singleton,active_epoch,product_prefix,url_pattern,updated_at)
  VALUES(1,1,'/products','compact',CURRENT_TIMESTAMP)
  ON CONFLICT(singleton) DO NOTHING;
CREATE TABLE IF NOT EXISTS public_visibility (
  product_id TEXT PRIMARY KEY REFERENCES products(id),
  generation INTEGER NOT NULL,
  visible INTEGER NOT NULL CHECK(visible IN (0,1)),
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS public_activations (
  product_id TEXT PRIMARY KEY REFERENCES products(id),
  source_revision INTEGER NOT NULL,
  public_revision INTEGER NOT NULL,
  site_epoch INTEGER NOT NULL,
  route TEXT NOT NULL UNIQUE,
  artifact_id TEXT NOT NULL UNIQUE,
  manifest_hash TEXT NOT NULL,
  visibility_generation INTEGER NOT NULL,
  view_json TEXT NOT NULL,
    activated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS public_route_history (
    route TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id),
    target_route TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL CHECK(state IN ('redirect','gone')),
    site_epoch INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS public_route_history_product ON public_route_history(product_id);
CREATE TABLE IF NOT EXISTS public_search_projection (
    product_id TEXT PRIMARY KEY REFERENCES products(id),
    source_revision INTEGER NOT NULL,
    site_epoch INTEGER NOT NULL,
    visibility_generation INTEGER NOT NULL,
    projection_version TEXT NOT NULL,
    part_number_folded TEXT NOT NULL,
    product_name_folded TEXT NOT NULL,
    manufacturer_folded TEXT NOT NULL,
    brand_folded TEXT NOT NULL,
    category_folded TEXT NOT NULL,
    all_folded TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS public_search_projection_version ON public_search_projection(projection_version,product_id);
CREATE TABLE IF NOT EXISTS publication_dirty (
    kind TEXT NOT NULL,
  entity_id TEXT NOT NULL,
  desired_revision INTEGER NOT NULL,
  reason TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(kind,entity_id)
);
	CREATE TABLE IF NOT EXISTS import_templates (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		current_version INTEGER NOT NULL,
		created_by TEXT NOT NULL REFERENCES users(id),
		updated_by TEXT NOT NULL REFERENCES users(id),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS import_template_versions (
		template_id TEXT NOT NULL REFERENCES import_templates(id),
		version INTEGER NOT NULL,
		snapshot_json TEXT NOT NULL,
		created_by TEXT NOT NULL REFERENCES users(id),
		created_at TEXT NOT NULL,
		PRIMARY KEY(template_id,version)
	);
	CREATE TABLE IF NOT EXISTS import_runs (
		id TEXT PRIMARY KEY,
		request_hash TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('committed','interrupted','failed','cancelled')),
		actor_id TEXT NOT NULL REFERENCES users(id),
		template_id TEXT,
		template_version INTEGER,
		total_rows INTEGER NOT NULL,
		created_count INTEGER NOT NULL,
		updated_count INTEGER NOT NULL,
		no_change_count INTEGER NOT NULL,
		failed_count INTEGER NOT NULL,
		receipt_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		completed_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS import_jobs (
		id TEXT PRIMARY KEY,
		actor_id TEXT NOT NULL REFERENCES users(id),
		status TEXT NOT NULL CHECK(status IN ('queued','parsing','preview_ready','committing','committed','failed','cancelled','interrupted')),
		phase TEXT NOT NULL,
		original_filename TEXT NOT NULL,
		checksum TEXT NOT NULL,
		template_snapshot_json TEXT NOT NULL,
		checked_rows INTEGER NOT NULL,
		total_rows INTEGER NOT NULL,
		create_count INTEGER NOT NULL,
		update_count INTEGER NOT NULL,
		no_change_count INTEGER NOT NULL,
		failed_count INTEGER NOT NULL,
		preview_path TEXT NOT NULL,
		report_path TEXT NOT NULL,
		error_message TEXT NOT NULL,
		cancel_requested INTEGER NOT NULL CHECK(cancel_requested IN (0,1)),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS product_bulk_runs (
		id TEXT PRIMARY KEY,
		actor_id TEXT NOT NULL REFERENCES users(id),
		action TEXT NOT NULL CHECK(action IN ('publish','hide','archive','change_category','change_lifecycle')),
		target_id TEXT NOT NULL DEFAULT '',
		request_hash TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('prepared','running','completed')),
		plan_json TEXT NOT NULL,
		receipt_json TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS product_bulk_items (
		run_id TEXT NOT NULL REFERENCES product_bulk_runs(id) ON DELETE CASCADE,
		product_id TEXT NOT NULL REFERENCES products(id),
		selection_index INTEGER NOT NULL,
		expected_revision INTEGER NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('prepared','succeeded','no_change','conflict','invalid','failed')),
		result_json TEXT NOT NULL DEFAULT '',
		completed_at TEXT,
		PRIMARY KEY(run_id,product_id),
		UNIQUE(run_id,selection_index)
	);
	CREATE INDEX IF NOT EXISTS product_bulk_runs_status_idx ON product_bulk_runs(status,updated_at);`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := s.backfillProductURLNames(ctx); err != nil {
		return err
	}
	return s.initializeWebsiteConfiguration(ctx, site.DefaultConfiguration())
}

func (s *Store) backfillProductURLNames(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.part_number FROM products p WHERE NOT EXISTS (SELECT 1 FROM product_url_names n WHERE n.product_id=p.id) ORDER BY p.id`)
	if err != nil {
		return err
	}
	type missingURLName struct{ id, partNumber string }
	var missing []missingURLName
	for rows.Next() {
		var item missingURLName
		if err := rows.Scan(&item.id, &item.partNumber); err != nil {
			rows.Close()
			return err
		}
		missing = append(missing, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range missing {
		slug := catalog.SuggestedSlug(item.partNumber, item.id)
		var duplicate int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_url_names WHERE slug=?`, slug).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate != 0 {
			suffix := strings.TrimPrefix(item.id, "prd_")
			if len(suffix) > 12 {
				suffix = suffix[:12]
			}
			slug = slug + "-" + suffix
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_url_names(product_id,slug,custom_path,revision,updated_at) VALUES(?,?,'',1,?)`, item.id, slug, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) seed(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM products").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	products := []catalog.Product{
		{ID: "synthetic-published", PartNumber: "SYNTH-3V3-01", Name: "Synthetic dual-range regulator", Manufacturer: "Example Components", Description: "Synthetic data for POC-01 only.", Specification: "3.0–3.6V, 4.5–5.5V", DocumentURL: "https://example.com/synthetic-datasheet.pdf", Status: catalog.Published},
		{ID: "synthetic-hidden", PartNumber: "SECRET-POC-01", Name: "Hidden synthetic component", Manufacturer: "Example Components", Description: "Must never be public.", Specification: "Hidden", Status: catalog.Hidden},
	}
	for i := 1; i <= 6; i++ {
		products = append(products, catalog.Product{ID: fmt.Sprintf("synthetic-page-%d", i), PartNumber: fmt.Sprintf("PAGE-%03d", i), Name: "Synthetic pagination product", Manufacturer: "Example Components", Description: "Synthetic pagination fixture.", Specification: "1V", Status: catalog.Published})
	}
	for i := range products {
		if err := products[i].Prepare(); err != nil {
			return err
		}
		if err := s.InsertProduct(ctx, products[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InsertProduct(ctx context.Context, product catalog.Product) error {
	if err := product.Prepare(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := ensureCurrentIdentityAvailable(ctx, tx, product, ""); err != nil {
			return err
		}
		return insertProductTx(ctx, tx, product, now)
	})
}

func ensureCurrentIdentityAvailable(ctx context.Context, tx *sql.Tx, product catalog.Product, excludeID string) error {
	if product.RecordState != catalog.RecordCurrent {
		return nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM products
		WHERE record_state='current' AND identity_manufacturer=? AND identity_part_number=? AND id<>?`,
		product.IdentityMaker, product.IdentityPart, excludeID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return catalog.ErrIdentityConflict
	}
	return nil
}

func scanProduct(scanner interface{ Scan(...any) error }) (catalog.Product, error) {
	var p catalog.Product
	var applicationsJSON string
	err := scanner.Scan(&p.ID, &p.Slug, &p.CustomPath, &p.CategoryID, &p.PartNumber, &p.IdentityPart, &p.Name,
		&p.ManufacturerID, &p.Manufacturer, &p.IdentityMaker, &p.BrandID, &p.Brand, &p.LifecycleID,
		&p.PackageFormFactor, &p.Description, &p.Features, &p.Specification, &p.DocumentURL,
		&p.Status, &p.RecordState, &p.Revision, &p.ClonedFromID, &p.CreatedBy, &p.UpdatedBy,
		&p.SearchFolded, &p.ProjectionVer, &p.Lifecycle, &applicationsJSON)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(applicationsJSON), &p.Applications); err != nil {
		return catalog.Product{}, fmt.Errorf("decode product applications: %w", err)
	}
	p.ApplicationIDs = make([]string, 0, len(p.Applications))
	for _, application := range p.Applications {
		p.ApplicationIDs = append(p.ApplicationIDs, application.ID)
	}
	return p, nil
}

const productColumns = `id, COALESCE((SELECT slug FROM product_url_names WHERE product_id=products.id), ''), COALESCE((SELECT custom_path FROM product_url_names WHERE product_id=products.id), ''), category_id, part_number, identity_part_number, name,
	COALESCE(manufacturer_id, ''), manufacturer, identity_manufacturer, COALESCE(brand_id, ''), brand, COALESCE(lifecycle_id, ''),
	package_form_factor, description, features, specification, document_url,
	status, record_state, revision, COALESCE(cloned_from_id, ''), COALESCE(created_by, ''), COALESCE(updated_by, ''),
	search_folded, search_projection_version,
	COALESCE((SELECT name FROM dictionary_entries WHERE id=products.lifecycle_id AND kind='lifecycle'), ''),
	COALESCE((SELECT json_group_array(json(payload)) FROM (
		SELECT json_object('id',d.id,'name',d.name,'slug',d.slug) AS payload
		FROM product_applications pa JOIN dictionary_entries d ON d.id=pa.application_id
		WHERE pa.product_id=products.id ORDER BY pa.sort_order,d.name,d.id
	)), '[]')`

func (s *Store) PublishedProduct(ctx context.Context, id string) (catalog.Product, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+productColumns+`
		FROM products WHERE id = ? AND record_state = 'current' AND status = 'published'`, id)
	return scanProduct(row)
}

func (s *Store) ListPublished(ctx context.Context, query string, page, pageSize int) ([]catalog.Product, int, error) {
	if page < 1 {
		page = 1
	}
	folded := catalog.FoldSearch(query)
	where := `record_state = 'current' AND status = 'published' AND search_projection_version = ?`
	args := []any{catalog.SearchProjectionVersion}
	if folded != "" {
		where += ` AND search_folded LIKE ? ESCAPE '\'`
		args = append(args, "%"+catalog.EscapeLike(folded)+"%")
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM products WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT `+productColumns+` FROM products WHERE `+where+`
		ORDER BY part_number, id LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var products []catalog.Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, 0, err
		}
		products = append(products, product)
	}
	return products, total, rows.Err()
}

func (s *Store) HideProduct(ctx context.Context, id string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE products
			SET status = 'hidden', revision = revision + 1, updated_at = ?
			WHERE id = ? AND record_state='current' AND status = 'published'`, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (s *Store) ProductCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM products").Scan(&count)
	return count, err
}

func (s *Store) SubmitRFQ(ctx context.Context, key string, submission inquiries.Submission) (inquiries.Receipt, error) {
	if len(key) < 24 {
		return inquiries.Receipt{}, inquiries.ErrInvalidRFQ
	}
	hash, err := submission.CanonicalHash()
	if err != nil {
		return inquiries.Receipt{}, err
	}
	tx, release, err := s.beginWrite(ctx)
	if err != nil {
		return inquiries.Receipt{}, err
	}
	defer release()
	defer tx.Rollback()

	var existingVersion, existingHash, encodedReceipt string
	err = tx.QueryRowContext(ctx, `SELECT canonical_version, request_hash, receipt_json
		FROM idempotency_receipts WHERE idempotency_key = ?`, key).Scan(&existingVersion, &existingHash, &encodedReceipt)
	if err == nil {
		if existingVersion != inquiries.CanonicalVersion {
			return inquiries.Receipt{}, ErrInvalidRFQReceipt
		}
		if existingHash != hash {
			return inquiries.Receipt{}, inquiries.ErrIdempotencyConflict
		}
		var receipt inquiries.Receipt
		if err := json.Unmarshal([]byte(encodedReceipt), &receipt); err != nil {
			return inquiries.Receipt{}, err
		}
		receipt.Replay = true
		return receipt, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return inquiries.Receipt{}, err
	}

	rfqID, err := randomID("rfq")
	if err != nil {
		return inquiries.Receipt{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO rfqs (
		id, name, email, company, phone, country, general_message, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, rfqID, submission.Name, submission.Email,
		submission.Company, submission.Phone, submission.Country, submission.GeneralMessage, now); err != nil {
		return inquiries.Receipt{}, err
	}
	for index, item := range submission.Items {
		var snapshot *string
		if item.Kind == "catalog" {
			product, err := productInTx(ctx, tx, item.ProductID)
			if err != nil {
				return inquiries.Receipt{}, inquiries.ErrInvalidRFQ
			}
			encoded, _ := json.Marshal(map[string]any{
				"id": product.ID, "revision": product.Revision, "part_number": product.PartNumber,
				"name": product.Name, "manufacturer": product.Manufacturer,
			})
			value := string(encoded)
			snapshot = &value
		} else if item.Requested == "" && item.RawQuery == "" {
			return inquiries.Receipt{}, inquiries.ErrInvalidRFQ
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO rfq_items (
			rfq_id, item_index, kind, product_id, requested, raw_query, quantity, notes, public_snapshot_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, rfqID, index, item.Kind, nullable(item.ProductID),
			item.Requested, item.RawQuery, item.Quantity, item.Notes, snapshot); err != nil {
			return inquiries.Receipt{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO rfq_recipients(rfq_id,recipient_index,kind,user_id,email)
		SELECT ?,recipient_index,kind,user_id,email FROM rfq_default_recipients ORDER BY recipient_index`, rfqID); err != nil {
		return inquiries.Receipt{}, err
	}
	receipt := inquiries.Receipt{RFQID: rfqID, Message: "We received your request for quotation."}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return inquiries.Receipt{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO idempotency_receipts (
		idempotency_key, canonical_version, request_hash, rfq_id, receipt_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?)`, key, inquiries.CanonicalVersion, hash, rfqID, string(encoded), now); err != nil {
		return inquiries.Receipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return inquiries.Receipt{}, err
	}
	return receipt, nil
}

func productInTx(ctx context.Context, tx *sql.Tx, id string) (catalog.Product, error) {
	return scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+`
		FROM products WHERE id = ? AND record_state='current' AND status = 'published'`, id))
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) ListRFQs(ctx context.Context) ([]RFQSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,email,company,phone,country,general_message,
		status,revision,COALESCE(updated_by,''),updated_at,created_at,privacy_state,
		COALESCE(privacy_processed_at,''),COALESCE(privacy_processed_by,'')
		FROM rfqs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var summaries []RFQSummary
	for rows.Next() {
		var item RFQSummary
		if err := rows.Scan(&item.ID, &item.Name, &item.Email, &item.Company, &item.Phone,
			&item.Country, &item.GeneralMessage, &item.Status, &item.Revision,
			&item.UpdatedBy, &item.UpdatedAt, &item.CreatedAt, &item.PrivacyState,
			&item.PrivacyAt, &item.PrivacyBy); err != nil {
			return nil, err
		}
		summaries = append(summaries, item)
	}
	for i := range summaries {
		itemRows, err := s.db.QueryContext(ctx, `SELECT kind, COALESCE(product_id, ''), requested, raw_query, quantity, notes
			FROM rfq_items WHERE rfq_id = ? ORDER BY item_index`, summaries[i].ID)
		if err != nil {
			return nil, err
		}
		for itemRows.Next() {
			var item inquiries.Item
			if err := itemRows.Scan(&item.Kind, &item.ProductID, &item.Requested, &item.RawQuery, &item.Quantity, &item.Notes); err != nil {
				itemRows.Close()
				return nil, err
			}
			summaries[i].Items = append(summaries[i].Items, item)
		}
		if err := itemRows.Close(); err != nil {
			return nil, err
		}
		recipientRows, err := s.db.QueryContext(ctx, `SELECT r.kind,COALESCE(r.user_id,''),
			CASE WHEN r.kind='user' THEN COALESCE(u.email,'') ELSE r.email END,
			COALESCE(u.display_name,'')
			FROM rfq_recipients r LEFT JOIN users u ON u.id=r.user_id
			WHERE r.rfq_id=? ORDER BY r.recipient_index`, summaries[i].ID)
		if err != nil {
			return nil, err
		}
		for recipientRows.Next() {
			var recipient inquiries.Recipient
			if err := recipientRows.Scan(&recipient.Kind, &recipient.UserID, &recipient.Email, &recipient.DisplayName); err != nil {
				recipientRows.Close()
				return nil, err
			}
			summaries[i].Recipients = append(summaries[i].Recipients, recipient)
		}
		if err := recipientRows.Close(); err != nil {
			return nil, err
		}
	}
	return summaries, rows.Err()
}

func NewKey() (string, error) { return randomID("sub") }

func randomID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
