package recovery

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"prods/internal/platform"
)

const (
	RestorePrepared = "prepared"
	RestoreVerified = "verified"
)

type RestoreTarget struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // file or directory
}

type RestoreConfig struct {
	BackupPath     string
	ControlDir     string
	Targets        map[string]RestoreTarget
	ResourceGate   *platform.ResourceGate
	ByteHeadroom   uint64
	InodeHeadroom  uint64
	MinFreePercent uint8
}

type RestoreJournal struct {
	OperationID    string                   `json:"operation_id"`
	ManifestPath   string                   `json:"manifest_path"`
	ManifestSHA256 string                   `json:"manifest_sha256"`
	ManifestID     string                   `json:"manifest_id"`
	SchemaVersion  int                      `json:"schema_version"`
	Phase          string                   `json:"phase"`
	Order          []string                 `json:"order"`
	Targets        map[string]RestoreTarget `json:"targets"`
	Stages         map[string]string        `json:"stages"`
	RootSHA256     map[string]string        `json:"root_sha256"`
	Completed      map[string]string        `json:"completed"`
	PreparedUTC    string                   `json:"prepared_utc"`
	VerifiedUTC    string                   `json:"verified_utc,omitempty"`
}

func PrepareRestore(ctx context.Context, config RestoreConfig) (string, error) {
	manifest, err := LoadManifest(config.BackupPath)
	if err != nil {
		return "", err
	}
	if err := validateRestoreTargets(config, manifest); err != nil {
		return "", err
	}
	reservations, err := admitRestoreResources(ctx, config, manifest)
	if err != nil {
		return "", err
	}
	defer releaseReservations(reservations)
	manifestPath := filepath.Join(config.BackupPath, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	operationID, err := backupID()
	if err != nil {
		return "", err
	}
	journal := RestoreJournal{
		OperationID: operationID, ManifestPath: manifestPath, ManifestSHA256: digestBytes(manifestBytes),
		ManifestID: manifest.ID, SchemaVersion: manifest.SchemaVersion, Phase: RestorePrepared,
		Order: restoreOrder(manifest), Targets: config.Targets, Stages: make(map[string]string),
		RootSHA256: make(map[string]string), Completed: make(map[string]string), PreparedUTC: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, name := range journal.Order {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		target := config.Targets[name]
		stage := restoreStagePath(target.Path, operationID)
		journal.Stages[name] = stage
		if err := stageRestoreRoot(ctx, config.BackupPath, manifest, name, stage, target.Kind); err != nil {
			return "", fmt.Errorf("stage restore root %s: %w", name, err)
		}
		if name == "database" {
			if err := applyRestoredDatabasePolicy(stage, manifest.SchemaVersion); err != nil {
				return "", err
			}
		}
		hash, err := hashRestoreRoot(stage, target.Kind)
		if err != nil {
			return "", err
		}
		journal.RootSHA256[name] = hash
	}
	if err := verifyStagedCrossRoot(journal); err != nil {
		return "", err
	}
	if err := os.MkdirAll(config.ControlDir, 0o700); err != nil {
		return "", err
	}
	journalPath := filepath.Join(config.ControlDir, "restore-"+operationID+".json")
	if err := saveRestoreJournal(journalPath, journal); err != nil {
		return "", err
	}
	return journalPath, nil
}

func ResumeRestore(ctx context.Context, journalPath string) error {
	journal, err := LoadRestoreJournal(journalPath)
	if err != nil {
		return err
	}
	if journal.Phase == RestoreVerified {
		return verifyActivatedRestore(journal)
	}
	if journal.Phase != RestorePrepared {
		return fmt.Errorf("%w: unknown restore phase %q", ErrInvalidBackup, journal.Phase)
	}
	manifestBytes, err := os.ReadFile(journal.ManifestPath)
	if err != nil || digestBytes(manifestBytes) != journal.ManifestSHA256 {
		return fmt.Errorf("%w: restore source manifest changed", ErrInvalidBackup)
	}
	for _, name := range journal.Order {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		expected := journal.RootSHA256[name]
		if completed := journal.Completed[name]; completed != "" {
			actual, err := hashRestoreRoot(journal.Targets[name].Path, journal.Targets[name].Kind)
			if err != nil || actual != expected || completed != expected {
				return fmt.Errorf("%w: completed root %s evidence mismatch", ErrInvalidBackup, name)
			}
			continue
		}
		actual, hashErr := hashRestoreRoot(journal.Targets[name].Path, journal.Targets[name].Kind)
		if hashErr == nil && actual == expected {
			journal.Completed[name] = expected
			if err := saveRestoreJournal(journalPath, journal); err != nil {
				return err
			}
			continue
		}
		if err := activateRestoreRoot(journal, name); err != nil {
			return err
		}
		actual, err = hashRestoreRoot(journal.Targets[name].Path, journal.Targets[name].Kind)
		if err != nil || actual != expected {
			return fmt.Errorf("%w: activated root %s hash mismatch", ErrInvalidBackup, name)
		}
		journal.Completed[name] = expected
		if err := saveRestoreJournal(journalPath, journal); err != nil {
			return err
		}
	}
	if err := verifyActivatedRestore(journal); err != nil {
		return err
	}
	journal.Phase = RestoreVerified
	journal.VerifiedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	return saveRestoreJournal(journalPath, journal)
}

func LoadRestoreJournal(path string) (RestoreJournal, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return RestoreJournal{}, err
	}
	var journal RestoreJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		return RestoreJournal{}, err
	}
	if journal.OperationID == "" || journal.ManifestPath == "" || len(journal.Order) < 2 || journal.Phase == "" {
		return RestoreJournal{}, ErrInvalidBackup
	}
	return journal, nil
}

func PendingRestoreJournals(controlDir string) ([]string, error) {
	entries, err := os.ReadDir(controlDir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "restore-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(controlDir, entry.Name())
		journal, err := LoadRestoreJournal(path)
		if err != nil {
			return nil, err
		}
		if journal.Phase == RestorePrepared {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func validateRestoreTargets(config RestoreConfig, manifest Manifest) error {
	if strings.TrimSpace(config.ControlDir) == "" || strings.TrimSpace(config.BackupPath) == "" {
		return ErrInvalidBackup
	}
	required := restoreOrder(manifest)
	if len(config.Targets) != len(required) {
		return ErrInvalidBackup
	}
	for _, name := range required {
		target, exists := config.Targets[name]
		if !exists || strings.TrimSpace(target.Path) == "" {
			return fmt.Errorf("%w: missing target %s", ErrInvalidBackup, name)
		}
		expectedKind := "directory"
		if name == "database" {
			expectedKind = "file"
		} else if _, exists := manifest.Files[name]; exists {
			expectedKind = "file"
		}
		if target.Kind != expectedKind {
			return fmt.Errorf("%w: target %s must be %s", ErrInvalidBackup, name, expectedKind)
		}
		if expectedKind == "directory" && pathContains(target.Path, config.ControlDir) {
			return fmt.Errorf("%w: control journal lies inside restored root %s", ErrUnsafeBackupPath, name)
		}
		if pathContains(config.ControlDir, target.Path) {
			return fmt.Errorf("%w: restore target %s lies inside the control journal root", ErrUnsafeBackupPath, name)
		}
	}
	return nil
}

func restoreOrder(manifest Manifest) []string {
	order := []string{"database", "assets"}
	names := make(map[string]struct{}, len(manifest.Roots)+len(manifest.Files))
	for name := range manifest.Roots {
		names[name] = struct{}{}
	}
	for name := range manifest.Files {
		names[name] = struct{}{}
	}
	for _, preferred := range []string{"secrets", "config", "host-config"} {
		if _, exists := names[preferred]; exists {
			order = append(order, preferred)
			delete(names, preferred)
		}
	}
	var remaining []string
	for name := range names {
		remaining = append(remaining, name)
	}
	sort.Strings(remaining)
	return append(order, remaining...)
}

func restoreStagePath(target, operationID string) string {
	clean := filepath.Clean(target)
	return filepath.Join(filepath.Dir(clean), "."+filepath.Base(clean)+".restore-"+operationID)
}

func stageRestoreRoot(ctx context.Context, backupPath string, manifest Manifest, name, stage, kind string) error {
	if _, err := os.Stat(stage); err == nil {
		return ErrBackupExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if kind == "file" {
		file := manifest.Database
		if name != "database" {
			var exists bool
			file, exists = manifest.Files[name]
			if !exists {
				return ErrInvalidBackup
			}
		}
		source := filepath.Join(backupPath, filepath.FromSlash(file.Path))
		if err := requireContainedRegularFile(backupPath, source); err != nil {
			return err
		}
		if err := verifyManifestFile(source, file); err != nil {
			return err
		}
		return copyBackupFile(source, stage, 0o600)
	}
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return err
	}
	var files []ManifestFile
	var sourceRoot string
	if name == "assets" {
		sourceRoot = filepath.Join(backupPath, "assets")
		for _, asset := range manifest.Assets {
			files = append(files, asset.ManifestFile)
		}
	} else {
		sourceRoot = filepath.Join(backupPath, "roots", name)
		files = manifest.Roots[name]
	}
	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !safeRelativePath(file.Path) {
			return ErrUnsafeBackupPath
		}
		source := filepath.Join(sourceRoot, filepath.FromSlash(file.Path))
		if err := requireContainedRegularFile(sourceRoot, source); err != nil {
			return err
		}
		if err := verifyManifestFile(source, file); err != nil {
			return err
		}
		if err := copyBackupFile(source, filepath.Join(stage, filepath.FromSlash(file.Path)), 0o600); err != nil {
			return err
		}
	}
	return syncTree(stage)
}

func applyRestoredDatabasePolicy(path string, schemaVersion int) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=rw&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("%w: staged database integrity check failed", ErrInvalidBackup)
	}
	var actualVersion int
	if err := db.QueryRow(`SELECT schema_version FROM system_state WHERE singleton=1`).Scan(&actualVersion); err != nil || actualVersion != schemaVersion {
		return fmt.Errorf("%w: staged schema version mismatch", ErrInvalidBackup)
	}
	if _, err := db.Exec(`DELETE FROM admin_sessions`); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM set_password_tokens`); err != nil {
		return err
	}
	return nil
}

func activateRestoreRoot(journal RestoreJournal, name string) error {
	target := journal.Targets[name]
	stage := journal.Stages[name]
	preRestore := target.Path + ".pre-restore-" + journal.OperationID
	if _, err := os.Stat(target.Path); err == nil {
		if _, preErr := os.Stat(preRestore); preErr == nil {
			return fmt.Errorf("%w: both live and pre-restore roots exist for %s", ErrInvalidBackup, name)
		} else if !errors.Is(preErr, os.ErrNotExist) {
			return preErr
		}
		if err := os.Rename(target.Path, preRestore); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(target.Path)); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(stage); err != nil {
		return fmt.Errorf("%w: staged root %s unavailable", ErrInvalidBackup, name)
	}
	if err := os.Rename(stage, target.Path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target.Path))
}

func verifyStagedCrossRoot(journal RestoreJournal) error {
	copy := journal
	copy.Targets = make(map[string]RestoreTarget, len(journal.Targets))
	for name, target := range journal.Targets {
		target.Path = journal.Stages[name]
		copy.Targets[name] = target
	}
	return verifyActivatedRestore(copy)
}

func verifyActivatedRestore(journal RestoreJournal) error {
	for _, name := range journal.Order {
		actual, err := hashRestoreRoot(journal.Targets[name].Path, journal.Targets[name].Kind)
		if err != nil || actual != journal.RootSHA256[name] {
			return fmt.Errorf("%w: root %s final verification failed", ErrInvalidBackup, name)
		}
	}
	database := journal.Targets["database"].Path
	assets := journal.Targets["assets"].Path
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(database)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("%w: restored database integrity check failed", ErrInvalidBackup)
	}
	rows, err := db.Query(`SELECT storage_path,checksum,size_bytes FROM assets ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var relative, expected string
		var expectedSize int64
		if err := rows.Scan(&relative, &expected, &expectedSize); err != nil {
			return err
		}
		if !safeRelativePath(relative) {
			return ErrUnsafeBackupPath
		}
		file, err := inspectFile(filepath.Join(assets, filepath.FromSlash(relative)), relative)
		if err != nil || file.SHA256 != expected || file.Size != expectedSize {
			return fmt.Errorf("%w: restored asset %s mismatch", ErrInvalidBackup, relative)
		}
	}
	return rows.Err()
}

func verifyManifestFile(path string, expected ManifestFile) error {
	actual, err := inspectFile(path, expected.Path)
	if err != nil {
		return err
	}
	if actual.SHA256 != expected.SHA256 || actual.Size != expected.Size {
		return ErrInvalidBackup
	}
	return nil
}

func hashRestoreRoot(path, kind string) (string, error) {
	if kind == "file" {
		file, err := inspectFile(path, filepath.Base(path))
		return file.SHA256, err
	}
	return digestTree(path)
}

func digestTree(root string) (string, error) {
	var names []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrUnsafeBackupPath
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, relative)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		if _, err := io.WriteString(hash, filepath.ToSlash(name)); err != nil {
			return "", err
		}
		hash.Write([]byte{0})
		file, err := os.Open(filepath.Join(root, name))
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncTree(root string) error {
	var directories []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func saveRestoreJournal(path string, journal RestoreJournal) error {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	return writeDurableFile(path, append(body, '\n'), 0o600)
}

func digestBytes(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func pathContains(parent, child string) bool {
	parent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	child, err = filepath.Abs(child)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
