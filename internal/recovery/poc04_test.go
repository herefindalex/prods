//go:build poc

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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

var errPOC04Crash = errors.New("simulated process crash")

type poc04Manifest struct {
	ID         string            `json:"id"`
	CreatedUTC string            `json:"created_utc"`
	DBFile     string            `json:"db_file"`
	DBHash     string            `json:"db_hash"`
	Assets     map[string]string `json:"assets"`
	Roots      map[string]string `json:"roots"`
}

type poc04Journal struct {
	OperationID  string            `json:"operation_id"`
	ManifestPath string            `json:"manifest_path"`
	ManifestHash string            `json:"manifest_hash"`
	Phase        string            `json:"phase"`
	Order        []string          `json:"order"`
	Stage        map[string]string `json:"stage"`
	RootHashes   map[string]string `json:"root_hashes"`
	Completed    map[string]string `json:"completed"`
}

type poc04Fixture struct {
	root       string
	dbPath     string
	db         *sql.DB
	assets     string
	config     string
	other      string
	backups    string
	control    string
	assetGate  sync.Mutex
	pinsMu     sync.Mutex
	pinned     map[string]int
	pinReady   chan struct{}
	copyResume chan struct{}
}

func newPOC04Fixture(t *testing.T) *poc04Fixture {
	t.Helper()
	root := t.TempDir()
	f := &poc04Fixture{
		root:    root,
		dbPath:  filepath.Join(root, "live-source", "db.sqlite"),
		assets:  filepath.Join(root, "live-source", "assets"),
		config:  filepath.Join(root, "live-source", "config"),
		other:   filepath.Join(root, "live-source", "public-state"),
		backups: filepath.Join(root, "backups"),
		control: filepath.Join(root, "control"),
		pinned:  make(map[string]int),
	}
	for _, dir := range []string{filepath.Dir(f.dbPath), f.assets, f.config, f.other, f.backups, f.control} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	f.db, err = sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// One application writer connection matches the bounded SQLite writer model;
	// concurrent callers queue here instead of surfacing connection-local BUSY.
	f.db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA busy_timeout=5000",
		`CREATE TABLE rfqs(id INTEGER PRIMARY KEY, note TEXT NOT NULL)`,
		`CREATE TABLE asset_refs(name TEXT PRIMARY KEY, sha256 TEXT NOT NULL)`,
		`CREATE TABLE sessions(id TEXT PRIMARY KEY, account TEXT NOT NULL)`,
		`CREATE TABLE smtp_outbox(id TEXT PRIMARY KEY, status TEXT NOT NULL)`,
	} {
		if _, err := f.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec(`INSERT INTO sessions VALUES('old-session','admin'); INSERT INTO smtp_outbox VALUES('mail-1','pending')`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.config, "secrets.ini"), []byte("smtp_password=backup-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.other, "epoch"), []byte("42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.addAsset(t, "product-a.txt", []byte("immutable asset A"))
	t.Cleanup(func() { _ = f.db.Close() })
	return f
}

func (f *poc04Fixture) addAsset(t *testing.T, name string, body []byte) {
	t.Helper()
	f.assetGate.Lock()
	defer f.assetGate.Unlock()
	hash := hashBytes(body)
	file, err := os.OpenFile(filepath.Join(f.assets, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO asset_refs(name,sha256) VALUES(?,?)`, name, hash); err != nil {
		t.Fatal(err)
	}
}

func hashBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(body), nil
}

func hashTree(root string) (string, error) {
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, name)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func durableWrite(path string, body []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func copyFile(source, destination string) error {
	body, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return durableWrite(destination, body, 0o600)
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		return copyFile(path, target)
	})
}

func (f *poc04Fixture) pin(names []string) {
	f.pinsMu.Lock()
	defer f.pinsMu.Unlock()
	for _, name := range names {
		f.pinned[name]++
	}
}

func (f *poc04Fixture) unpin(names []string) {
	f.pinsMu.Lock()
	defer f.pinsMu.Unlock()
	for _, name := range names {
		f.pinned[name]--
		if f.pinned[name] == 0 {
			delete(f.pinned, name)
		}
	}
}

func (f *poc04Fixture) gc(name string) error {
	f.pinsMu.Lock()
	pinned := f.pinned[name] > 0
	f.pinsMu.Unlock()
	if pinned {
		return nil
	}
	return os.Remove(filepath.Join(f.assets, name))
}

func (f *poc04Fixture) backup(t *testing.T, id string) (poc04Manifest, error) {
	return f.backupWithManifestWriter(t, id, durableWrite)
}

func (f *poc04Fixture) backupWithManifestWriter(t *testing.T, id string, writeManifest func(string, []byte, fs.FileMode) error) (poc04Manifest, error) {
	t.Helper()
	destination := filepath.Join(f.backups, id)
	if err := os.MkdirAll(filepath.Join(destination, "assets"), 0o700); err != nil {
		return poc04Manifest{}, err
	}
	dbSnapshot := filepath.Join(destination, "db.sqlite")

	// Asset replacement is paused only across DB snapshot + pin registration.
	// RFQ writes continue. Immutable pinned files are copied after the gate opens.
	f.assetGate.Lock()
	if _, err := f.db.Exec(`VACUUM INTO ?`, dbSnapshot); err != nil {
		f.assetGate.Unlock()
		return poc04Manifest{}, err
	}
	snapshotDB, err := sql.Open("sqlite", dbSnapshot)
	if err != nil {
		f.assetGate.Unlock()
		return poc04Manifest{}, err
	}
	rows, err := snapshotDB.Query(`SELECT name,sha256 FROM asset_refs ORDER BY name`)
	if err != nil {
		_ = snapshotDB.Close()
		f.assetGate.Unlock()
		return poc04Manifest{}, err
	}
	assets := make(map[string]string)
	var names []string
	for rows.Next() {
		var name, hash string
		if err := rows.Scan(&name, &hash); err != nil {
			_ = rows.Close()
			_ = snapshotDB.Close()
			f.assetGate.Unlock()
			return poc04Manifest{}, err
		}
		assets[name] = hash
		names = append(names, name)
	}
	_ = rows.Close()
	_ = snapshotDB.Close()
	f.pin(names)
	f.assetGate.Unlock()
	defer f.unpin(names)
	if f.pinReady != nil {
		close(f.pinReady)
		<-f.copyResume
	}

	for _, name := range names {
		if err := copyFile(filepath.Join(f.assets, name), filepath.Join(destination, "assets", name)); err != nil {
			return poc04Manifest{}, err
		}
		got, err := hashFile(filepath.Join(destination, "assets", name))
		if err != nil || got != assets[name] {
			return poc04Manifest{}, fmt.Errorf("asset %s hash mismatch", name)
		}
	}
	for rootName, source := range map[string]string{"config": f.config, "other": f.other} {
		if err := copyTree(source, filepath.Join(destination, rootName)); err != nil {
			return poc04Manifest{}, err
		}
	}
	dbHash, err := hashFile(dbSnapshot)
	if err != nil {
		return poc04Manifest{}, err
	}
	configHash, err := hashTree(filepath.Join(destination, "config"))
	if err != nil {
		return poc04Manifest{}, err
	}
	otherHash, err := hashTree(filepath.Join(destination, "other"))
	if err != nil {
		return poc04Manifest{}, err
	}
	manifest := poc04Manifest{
		ID:         id,
		CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		DBFile:     "db.sqlite",
		DBHash:     dbHash,
		Assets:     assets,
		Roots:      map[string]string{"config": configHash, "other": otherHash},
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return poc04Manifest{}, err
	}
	if err := writeManifest(filepath.Join(destination, "manifest.json"), body, 0o600); err != nil {
		return poc04Manifest{}, err
	}
	return manifest, nil
}

func scanBackupManifests(root string) ([]poc04Manifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var result []poc04Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, entry.Name(), "manifest.json"))
		if err != nil {
			continue
		}
		var manifest poc04Manifest
		if json.Unmarshal(body, &manifest) == nil {
			result = append(result, manifest)
		}
	}
	return result, nil
}

func preparePOC04Restore(root, control, backup, operationID string, beforePrepared bool) (string, error) {
	manifestPath := filepath.Join(backup, "manifest.json")
	manifestBody, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}
	var manifest poc04Manifest
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		return "", err
	}
	stageBase := filepath.Join(root, "restore-stage", operationID)
	stages := map[string]string{
		"db":     filepath.Join(stageBase, "db"),
		"assets": filepath.Join(stageBase, "assets"),
		"config": filepath.Join(stageBase, "config"),
		"other":  filepath.Join(stageBase, "other"),
	}
	for _, dir := range stages {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
	}
	if err := copyFile(filepath.Join(backup, manifest.DBFile), filepath.Join(stages["db"], "db.sqlite")); err != nil {
		return "", err
	}
	if got, _ := hashFile(filepath.Join(stages["db"], "db.sqlite")); got != manifest.DBHash {
		return "", errors.New("staged DB hash mismatch")
	}
	if err := copyTree(filepath.Join(backup, "assets"), stages["assets"]); err != nil {
		return "", err
	}
	if err := copyTree(filepath.Join(backup, "config"), stages["config"]); err != nil {
		return "", err
	}
	if err := copyTree(filepath.Join(backup, "other"), stages["other"]); err != nil {
		return "", err
	}

	// Recovery policy is part of staging: old sessions are revoked and uncertain
	// SMTP work becomes explicit "unknown" rather than being blindly resent.
	stagedDB, err := sql.Open("sqlite", filepath.Join(stages["db"], "db.sqlite"))
	if err != nil {
		return "", err
	}
	if _, err := stagedDB.Exec(`DELETE FROM sessions; UPDATE smtp_outbox SET status='unknown'`); err != nil {
		_ = stagedDB.Close()
		return "", err
	}
	if err := stagedDB.Close(); err != nil {
		return "", err
	}

	rootHashes := make(map[string]string)
	for _, name := range []string{"db", "assets", "config", "other"} {
		hash, err := hashTree(stages[name])
		if err != nil {
			return "", err
		}
		rootHashes[name] = hash
	}
	if beforePrepared {
		return "", errPOC04Crash
	}
	journal := poc04Journal{
		OperationID:  operationID,
		ManifestPath: manifestPath,
		ManifestHash: hashBytes(manifestBody),
		Phase:        "prepared",
		Order:        []string{"db", "assets", "config", "other"},
		Stage:        stages,
		RootHashes:   rootHashes,
		Completed:    make(map[string]string),
	}
	journalPath := filepath.Join(control, operationID+".json")
	if err := savePOC04Journal(journalPath, journal); err != nil {
		return "", err
	}
	return journalPath, nil
}

func savePOC04Journal(path string, journal poc04Journal) error {
	body, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	return durableWrite(path, body, 0o600)
}

func loadPOC04Journal(path string) (poc04Journal, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return poc04Journal{}, err
	}
	var journal poc04Journal
	err = json.Unmarshal(body, &journal)
	return journal, err
}

func activeRoot(base, name string) (string, string, error) {
	body, err := os.ReadFile(filepath.Join(base, "live", name, "CURRENT"))
	if err != nil {
		return "", "", err
	}
	parts := strings.Split(strings.TrimSpace(string(body)), "|")
	if len(parts) != 2 {
		return "", "", errors.New("invalid root pointer")
	}
	return filepath.Join(base, "live", name, "generations", parts[0]), parts[1], nil
}

func activatePOC04Root(base string, journal poc04Journal, name string) error {
	target := filepath.Join(base, "live", name, "generations", journal.OperationID)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	if err := copyTree(journal.Stage[name], target); err != nil {
		return err
	}
	hash, err := hashTree(target)
	if err != nil {
		return err
	}
	if hash != journal.RootHashes[name] {
		return fmt.Errorf("activated %s root hash mismatch", name)
	}
	return durableWrite(filepath.Join(base, "live", name, "CURRENT"), []byte(journal.OperationID+"|"+hash+"\n"), 0o600)
}

func rollForwardPOC04(base, journalPath, fault string) error {
	journal, err := loadPOC04Journal(journalPath)
	if err != nil {
		return err
	}
	if journal.Phase == "verified" {
		return verifyPOC04Restore(base, journal)
	}
	if journal.Phase != "prepared" && journal.Phase != "activating" {
		return errors.New("restore is not prepared")
	}
	if fault == "after-prepared" {
		return errPOC04Crash
	}
	journal.Phase = "activating"
	if err := savePOC04Journal(journalPath, journal); err != nil {
		return err
	}
	for _, name := range journal.Order {
		if journal.Completed[name] == journal.RootHashes[name] {
			continue
		}
		active, hash, activeErr := activeRoot(base, name)
		if activeErr == nil && hash == journal.RootHashes[name] {
			actual, err := hashTree(active)
			if err != nil || actual != hash {
				return fmt.Errorf("active root %s evidence mismatch", name)
			}
			journal.Completed[name] = hash
			if err := savePOC04Journal(journalPath, journal); err != nil {
				return err
			}
			continue
		}
		if err := activatePOC04Root(base, journal, name); err != nil {
			return err
		}
		if fault == "after-"+name+"-activation-before-journal" {
			return errPOC04Crash
		}
		journal.Completed[name] = journal.RootHashes[name]
		if err := savePOC04Journal(journalPath, journal); err != nil {
			return err
		}
		if fault == "after-"+name {
			return errPOC04Crash
		}
	}
	if fault == "before-final-verification" {
		return errPOC04Crash
	}
	if err := verifyPOC04Restore(base, journal); err != nil {
		return err
	}
	journal.Phase = "verified"
	return savePOC04Journal(journalPath, journal)
}

func verifyPOC04Restore(base string, journal poc04Journal) error {
	for _, name := range journal.Order {
		active, evidence, err := activeRoot(base, name)
		if err != nil {
			return err
		}
		actual, err := hashTree(active)
		if err != nil || actual != journal.RootHashes[name] || evidence != actual {
			return fmt.Errorf("final %s root verification failed", name)
		}
	}
	dbRoot, _, _ := activeRoot(base, "db")
	assetsRoot, _, _ := activeRoot(base, "assets")
	db, err := sql.Open("sqlite", filepath.Join(dbRoot, "db.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT name,sha256 FROM asset_refs`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, expected string
		if err := rows.Scan(&name, &expected); err != nil {
			return err
		}
		actual, err := hashFile(filepath.Join(assetsRoot, name))
		if err != nil || actual != expected {
			return fmt.Errorf("DB/asset cross-root mismatch for %s", name)
		}
	}
	var sessions int
	if err := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 0 {
		return errors.New("old sessions were restored")
	}
	var smtpStatus string
	if err := db.QueryRow(`SELECT status FROM smtp_outbox WHERE id='mail-1'`).Scan(&smtpStatus); err != nil || smtpStatus != "unknown" {
		return errors.New("SMTP outcome was blindly replayable")
	}
	return nil
}

func TestPOC04OnlineBackupPinsSnapshotAssetsAndSurvivesPrimaryLoss(t *testing.T) {
	f := newPOC04Fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rfqWrites atomic.Int64
	var writerWG sync.WaitGroup
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				if _, err := f.db.Exec(`INSERT INTO rfqs(note) VALUES(?)`, fmt.Sprintf("rfq-%d", i)); err == nil {
					rfqWrites.Add(1)
				}
			}
		}
	}()

	f.pinReady = make(chan struct{})
	f.copyResume = make(chan struct{})
	type backupResult struct {
		manifest poc04Manifest
		err      error
	}
	result := make(chan backupResult, 1)
	go func() {
		manifest, err := f.backup(t, "backup-001")
		result <- backupResult{manifest: manifest, err: err}
	}()
	<-f.pinReady

	// Asset replacement and GC progress while the DB snapshot's immutable asset
	// set is pinned. GC cannot remove a snapshot dependency.
	f.addAsset(t, "product-b.txt", []byte("later immutable asset B"))
	if err := os.WriteFile(filepath.Join(f.assets, "expired.tmp"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.gc("expired.tmp"); err != nil {
		t.Fatal(err)
	}
	if err := f.gc("product-a.txt"); err != nil {
		t.Fatal(err)
	}
	close(f.copyResume)
	backup := <-result
	if backup.err != nil {
		t.Fatal(backup.err)
	}
	manifest := backup.manifest
	cancel()
	writerWG.Wait()
	if rfqWrites.Load() == 0 {
		t.Fatal("online RFQ writer made no progress")
	}
	if len(manifest.Assets) != 1 {
		t.Fatalf("snapshot assets = %d, want 1", len(manifest.Assets))
	}

	// Concurrent later assets cannot change the snapshot's exact set.
	if _, err := os.Stat(filepath.Join(f.backups, "backup-001", "assets", "product-b.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("backup incorrectly included a post-snapshot asset")
	}

	// The independent manifest remains enumerable even if the primary DB is unreadable.
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.dbPath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(f.dbPath + "-wal")
	_ = os.Remove(f.dbPath + "-shm")
	broken, err := sql.Open("sqlite", f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	var tableCount int
	err = broken.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tableCount)
	if err == nil {
		t.Fatal("corrupted primary unexpectedly remained queryable")
	}
	manifests, err := scanBackupManifests(f.backups)
	if err != nil || len(manifests) != 1 || manifests[0].ID != "backup-001" {
		t.Fatalf("manifest recovery failed: %#v, %v", manifests, err)
	}
}

func TestPOC04DestinationFailuresNeverCreateCompleteManifest(t *testing.T) {
	for _, failure := range []struct {
		name string
		err  error
	}{
		{"destination-disconnected", errors.New("destination disconnected")},
		{"out-of-space", syscallENOSPC{}},
		{"read-only", fs.ErrPermission},
	} {
		t.Run(failure.name, func(t *testing.T) {
			f := newPOC04Fixture(t)
			writer := func(string, []byte, fs.FileMode) error { return failure.err }
			if _, err := f.backupWithManifestWriter(t, "failed-backup", writer); err == nil {
				t.Fatal("fault did not surface")
			}
			if _, err := os.Stat(filepath.Join(f.backups, "failed-backup", "manifest.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed backup advertised a complete manifest")
			}
			manifests, err := scanBackupManifests(f.backups)
			if err != nil || len(manifests) != 0 {
				t.Fatalf("failed backup became selectable: %#v, %v", manifests, err)
			}
		})
	}
}

type syscallENOSPC struct{}

func (syscallENOSPC) Error() string { return "no space left on device" }

func TestPOC04PreparedRestoreAlwaysRollsForward(t *testing.T) {
	faults := []string{
		"after-prepared",
		"after-db",
		"after-assets",
		"after-config",
		"after-other",
		"before-final-verification",
		"after-db-activation-before-journal",
	}
	for _, fault := range faults {
		t.Run(fault, func(t *testing.T) {
			f := newPOC04Fixture(t)
			if _, err := f.backup(t, "restore-source"); err != nil {
				t.Fatal(err)
			}
			journalPath, err := preparePOC04Restore(f.root, f.control, filepath.Join(f.backups, "restore-source"), "restore-001", false)
			if err != nil {
				t.Fatal(err)
			}
			if err := rollForwardPOC04(f.root, journalPath, fault); !errors.Is(err, errPOC04Crash) {
				t.Fatalf("fault %s = %v, want simulated crash", fault, err)
			}
			journal, err := loadPOC04Journal(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			if journal.Phase == "verified" {
				t.Fatal("crashed restore entered Normal/verified")
			}
			if err := rollForwardPOC04(f.root, journalPath, ""); err != nil {
				t.Fatal(err)
			}
			journal, err = loadPOC04Journal(journalPath)
			if err != nil || journal.Phase != "verified" {
				t.Fatalf("restart did not finish the same restore: %#v, %v", journal, err)
			}
		})
	}
}

func TestPOC04BeforePreparedCanBeAbandoned(t *testing.T) {
	f := newPOC04Fixture(t)
	if _, err := f.backup(t, "restore-source"); err != nil {
		t.Fatal(err)
	}
	journalPath, err := preparePOC04Restore(f.root, f.control, filepath.Join(f.backups, "restore-source"), "abandoned", true)
	if !errors.Is(err, errPOC04Crash) || journalPath != "" {
		t.Fatalf("before prepared = (%q, %v)", journalPath, err)
	}
	entries, err := os.ReadDir(f.control)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("before-prepared staging created durable restore authority")
	}
	if _, _, err := activeRoot(f.root, "db"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("before-prepared crash activated a root")
	}
}
