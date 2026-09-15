package recovery

import (
	"os"
	"path/filepath"
	"testing"

	"prods/internal/platform"
)

func TestPreparedRestoreActivatesHostConfigAsOneFileAndReconcilesCrash(t *testing.T) {
	root := t.TempDir()
	store, assetRoot, backupRoot := newFileBackupTestStore(t, root)
	databasePath := filepath.Join(root, "data", "prods.db")
	backupConfigPath := filepath.Join(root, "backup-input", "prods.ini")
	backupConfig := []byte("listen=:8080\nbase_url=http://localhost:8080\n")
	if err := os.MkdirAll(filepath.Dir(backupConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupConfigPath, backupConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: backupRoot,
		AssetDir:  assetRoot,
		Files:     map[string]string{"host-config": backupConfigPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	runtimeRoot := filepath.Join(root, "runtime")
	liveConfigPath := filepath.Join(runtimeRoot, "prods.ini")
	siblingPath := filepath.Join(runtimeRoot, "prods")
	liveConfig := []byte("listen=:9090\nbase_url=http://localhost:9090\n")
	siblingBody := []byte("executable-sibling-must-survive")
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveConfigPath, liveConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(siblingPath, siblingBody, 0o700); err != nil {
		t.Fatal(err)
	}

	controlRoot := filepath.Join(root, "data", "control")
	var probedPaths []string
	journalPath, err := PrepareRestore(t.Context(), RestoreConfig{
		BackupPath: filepath.Join(backupRoot, manifest.ID),
		ControlDir: controlRoot,
		Targets: map[string]RestoreTarget{
			"database":    {Path: databasePath, Kind: "file"},
			"assets":      {Path: assetRoot, Kind: "directory"},
			"host-config": {Path: liveConfigPath, Kind: "file"},
		},
		ResourceGate: platform.NewResourceGate(func(path string) (platform.ResourceStats, error) {
			probedPaths = append(probedPaths, path)
			return platform.ResourceStats{VolumeID: path, TotalBytes: 1 << 30, FreeBytes: 1 << 30, FreeInodes: 1 << 20}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(liveConfigPath); err != nil || string(body) != string(liveConfig) {
		t.Fatalf("prepare changed live host config = %q, %v", body, err)
	}
	if body, err := os.ReadFile(siblingPath); err != nil || string(body) != string(siblingBody) {
		t.Fatalf("prepare changed sibling = %q, %v", body, err)
	}
	if !containsRestoreTestPath(probedPaths, liveConfigPath) {
		t.Fatalf("host config target was not resource-probed: %v", probedPaths)
	}

	journal, err := LoadRestoreJournal(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"database", "assets", "host-config"}
	if len(journal.Order) != len(wantOrder) {
		t.Fatalf("restore order = %v", journal.Order)
	}
	for index, name := range wantOrder {
		if journal.Order[index] != name {
			t.Fatalf("restore order = %v", journal.Order)
		}
	}

	// Simulate fixed-order activation where database and assets have durable
	// completion evidence, then the process crashes after replacing host-config
	// but before recording that final root.
	for _, name := range []string{"database", "assets"} {
		if err := activateRestoreRoot(journal, name); err != nil {
			t.Fatal(err)
		}
		journal.Completed[name] = journal.RootSHA256[name]
	}
	if err := saveRestoreJournal(journalPath, journal); err != nil {
		t.Fatal(err)
	}
	if err := activateRestoreRoot(journal, "host-config"); err != nil {
		t.Fatal(err)
	}
	if err := ResumeRestore(t.Context(), journalPath); err != nil {
		t.Fatal(err)
	}

	journal, err = LoadRestoreJournal(journalPath)
	if err != nil || journal.Phase != RestoreVerified || journal.Completed["host-config"] == "" {
		t.Fatalf("reconciled restore journal = %+v, %v", journal, err)
	}
	if body, err := os.ReadFile(liveConfigPath); err != nil || string(body) != string(backupConfig) {
		t.Fatalf("restored host config = %q, %v", body, err)
	}
	if body, err := os.ReadFile(siblingPath); err != nil || string(body) != string(siblingBody) {
		t.Fatalf("host config activation replaced sibling = %q, %v", body, err)
	}
	if body, err := os.ReadFile(liveConfigPath + ".pre-restore-" + journal.OperationID); err != nil || string(body) != string(liveConfig) {
		t.Fatalf("preserved pre-restore host config = %q, %v", body, err)
	}
}

func containsRestoreTestPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
