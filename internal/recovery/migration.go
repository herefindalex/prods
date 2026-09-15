package recovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MigrationPrepared   = "prepared"
	MigrationFailed     = "failed"
	MigrationSuperseded = "superseded_by_restore"
	MigrationVerified   = "verified"
)

type MigrationStep struct {
	Version       int    `json:"version"`
	Name          string `json:"name"`
	Checksum      string `json:"checksum"`
	Transactional bool   `json:"transactional"`
}

type MigrationJournal struct {
	OperationID    string          `json:"operation_id"`
	BackupPath     string          `json:"backup_path"`
	BackupID       string          `json:"backup_id"`
	ManifestSHA256 string          `json:"manifest_sha256"`
	FromVersion    int             `json:"from_version"`
	ToVersion      int             `json:"to_version"`
	Steps          []MigrationStep `json:"steps"`
	Phase          string          `json:"phase"`
	Failure        string          `json:"failure,omitempty"`
	PreparedUTC    string          `json:"prepared_utc"`
	VerifiedUTC    string          `json:"verified_utc,omitempty"`
}

func PrepareMigrationJournal(controlDir, backupPath string, fromVersion, toVersion int, steps []MigrationStep) (string, error) {
	if strings.TrimSpace(controlDir) == "" || strings.TrimSpace(backupPath) == "" || fromVersion < 1 || toVersion <= fromVersion || len(steps) == 0 {
		return "", ErrInvalidBackup
	}
	manifest, err := LoadManifest(backupPath)
	if err != nil {
		return "", err
	}
	if manifest.SchemaVersion != fromVersion {
		return "", fmt.Errorf("%w: pre-upgrade backup schema is %d, want %d", ErrInvalidBackup, manifest.SchemaVersion, fromVersion)
	}
	manifestPath := filepath.Join(backupPath, "manifest.json")
	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	for index, step := range steps {
		if step.Version != fromVersion+index+1 || strings.TrimSpace(step.Name) == "" || strings.TrimSpace(step.Checksum) == "" {
			return "", fmt.Errorf("%w: invalid migration step %d", ErrInvalidBackup, index)
		}
	}
	if steps[len(steps)-1].Version != toVersion {
		return "", fmt.Errorf("%w: migration plan ends at %d, want %d", ErrInvalidBackup, steps[len(steps)-1].Version, toVersion)
	}
	operationID, err := backupID()
	if err != nil {
		return "", err
	}
	journal := MigrationJournal{
		OperationID:    operationID,
		BackupPath:     backupPath,
		BackupID:       manifest.ID,
		ManifestSHA256: digestBytes(manifestBody),
		FromVersion:    fromVersion,
		ToVersion:      toVersion,
		Steps:          steps,
		Phase:          MigrationPrepared,
		PreparedUTC:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(controlDir, "migration-"+operationID+".json")
	if err := saveMigrationJournal(path, journal); err != nil {
		return "", err
	}
	return path, nil
}

func LoadMigrationJournal(path string) (MigrationJournal, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return MigrationJournal{}, err
	}
	var journal MigrationJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		return MigrationJournal{}, err
	}
	if journal.OperationID == "" || journal.BackupID == "" || journal.BackupPath == "" || journal.FromVersion < 1 || journal.ToVersion <= journal.FromVersion || len(journal.Steps) == 0 {
		return MigrationJournal{}, ErrInvalidBackup
	}
	return journal, nil
}

func ValidateMigrationJournalSource(journal MigrationJournal) error {
	manifest, err := LoadManifest(journal.BackupPath)
	if err != nil {
		return err
	}
	if manifest.ID != journal.BackupID || manifest.SchemaVersion != journal.FromVersion {
		return fmt.Errorf("%w: migration backup identity changed", ErrInvalidBackup)
	}
	body, err := os.ReadFile(filepath.Join(journal.BackupPath, "manifest.json"))
	if err != nil {
		return err
	}
	if digestBytes(body) != journal.ManifestSHA256 {
		return fmt.Errorf("%w: migration backup manifest changed", ErrInvalidBackup)
	}
	return nil
}

func MarkMigrationFailed(path string, cause error) error {
	journal, err := LoadMigrationJournal(path)
	if err != nil {
		return err
	}
	if journal.Phase != MigrationPrepared {
		return fmt.Errorf("%w: cannot fail migration in phase %q", ErrInvalidBackup, journal.Phase)
	}
	journal.Phase = MigrationFailed
	journal.Failure = strings.TrimSpace(cause.Error())
	return saveMigrationJournal(path, journal)
}

func CompleteMigrationJournal(path string, actualVersion int) error {
	journal, err := LoadMigrationJournal(path)
	if err != nil {
		return err
	}
	if journal.Phase == MigrationVerified {
		if actualVersion != journal.ToVersion {
			return fmt.Errorf("%w: verified migration schema changed", ErrInvalidBackup)
		}
		return nil
	}
	if journal.Phase != MigrationPrepared || actualVersion != journal.ToVersion {
		return fmt.Errorf("%w: cannot verify migration phase=%q schema=%d", ErrInvalidBackup, journal.Phase, actualVersion)
	}
	journal.Phase = MigrationVerified
	journal.VerifiedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	return saveMigrationJournal(path, journal)
}

func PendingMigrationJournals(controlDir string) ([]string, error) {
	entries, err := os.ReadDir(controlDir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "migration-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(controlDir, entry.Name())
		journal, err := LoadMigrationJournal(path)
		if err != nil {
			return nil, err
		}
		if journal.Phase != MigrationVerified && journal.Phase != MigrationSuperseded {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func SupersedeMigrationJournals(controlDir, restoreOperationID string) error {
	restoreOperationID = strings.TrimSpace(restoreOperationID)
	if restoreOperationID == "" {
		return ErrInvalidBackup
	}
	paths, err := PendingMigrationJournals(controlDir)
	if err != nil {
		return err
	}
	for _, path := range paths {
		journal, err := LoadMigrationJournal(path)
		if err != nil {
			return err
		}
		journal.Phase = MigrationSuperseded
		journal.Failure = "superseded by explicit restore " + restoreOperationID
		if err := saveMigrationJournal(path, journal); err != nil {
			return err
		}
	}
	return nil
}

func saveMigrationJournal(path string, journal MigrationJournal) error {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	return writeDurableFile(path, append(body, '\n'), 0o600)
}
