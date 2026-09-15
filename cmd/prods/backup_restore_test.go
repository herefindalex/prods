package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/recovery"
	storesqlite "prods/internal/storage/sqlite"
)

func TestExplicitRestoreReturnsSuccessAfterVerifiedCompletion(t *testing.T) {
	root := t.TempDir()
	allowBackupTestCleanup(t, root)
	dataDir := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backups")
	databasePath := filepath.Join(dataDir, "prods.db")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir: backupDir,
		AssetDir:  filepath.Join(dataDir, "assets"),
		Kind:      "selected-restore-point",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	previousArgs := os.Args
	os.Args = []string{previousArgs[0], "--data-dir", dataDir, "--backup-dir", backupDir, "--restore-backup", selected.ID}
	t.Cleanup(func() { os.Args = previousArgs })
	if err := runWithStop(nil); err != nil {
		t.Fatalf("verified explicit restore returned an error: %v", err)
	}
}

func TestOfflineRestorePrebackupIncludesCurrentHostConfig(t *testing.T) {
	root := t.TempDir()
	allowBackupTestCleanup(t, root)
	dataDir := filepath.Join(root, "data")
	assetDir := filepath.Join(dataDir, "assets")
	backupDir := filepath.Join(root, "backups")
	configPath := filepath.Join(root, "runtime", "prods.ini")
	databasePath := filepath.Join(dataDir, "prods.db")
	for _, directory := range []string{assetDir, filepath.Dir(configPath)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	originalConfig := []byte("listen=:8080\nbase_url=http://localhost:8080\n")
	if err := os.WriteFile(configPath, originalConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	selected, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir: backupDir,
		AssetDir:  assetDir,
		Kind:      "selected-restore-point",
		Files:     map[string]string{"host-config": configPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	liveConfig := []byte("listen=:9090\nbase_url=http://localhost:9090\n")
	if err := os.WriteFile(configPath, liveConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runOfflineRestore(
		t.Context(), databasePath, dataDir, backupDir, configPath, selected.ID, false, nil, nil,
	); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(configPath); err != nil || string(body) != string(originalConfig) {
		t.Fatalf("restored host config = %q, %v", body, err)
	}

	backups, err := recovery.ListBackups(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	var preRestore *recovery.Manifest
	for index := range backups {
		if backups[index].Kind == "pre-restore" {
			preRestore = &backups[index]
			break
		}
	}
	if preRestore == nil {
		t.Fatalf("pre-restore backup missing: %+v", backups)
	}
	entry, ok := preRestore.Files["host-config"]
	if !ok || entry.Path != "files/host-config" {
		t.Fatalf("pre-restore host config entry = %+v", entry)
	}
	preRestoreBody, err := os.ReadFile(filepath.Join(backupDir, preRestore.ID, filepath.FromSlash(entry.Path)))
	if err != nil || string(preRestoreBody) != string(liveConfig) {
		t.Fatalf("pre-restore host config = %q, %v", preRestoreBody, err)
	}
}

func TestOfflineRestoreRejectsUnknownExplicitFileRoot(t *testing.T) {
	root := t.TempDir()
	allowBackupTestCleanup(t, root)
	dataDir := filepath.Join(root, "data")
	assetDir := filepath.Join(dataDir, "assets")
	backupDir := filepath.Join(root, "backups")
	databasePath := filepath.Join(dataDir, "prods.db")
	configPath := filepath.Join(root, "prods.ini")
	unknownPath := filepath.Join(root, "unknown.state")
	if err := os.MkdirAll(assetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("listen=:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unknownPath, []byte("untrusted target mapping"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := recovery.CreateBackup(t.Context(), store, recovery.BackupConfig{
		BackupDir: backupDir,
		AssetDir:  assetDir,
		Files:     map[string]string{"unknown": unknownPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	err = runOfflineRestore(t.Context(), databasePath, dataDir, backupDir, configPath, selected.ID, false, nil, nil)
	if err == nil || !strings.Contains(err.Error(), `unsupported restore file root "unknown"`) {
		t.Fatalf("unknown explicit file restore error = %v", err)
	}
	pending, pendingErr := recovery.PendingRestoreJournals(filepath.Join(dataDir, "control"))
	if pendingErr != nil || len(pending) != 0 {
		t.Fatalf("unknown file root prepared a restore: %v, %v", pending, pendingErr)
	}
}

func TestExistingBackupFilesIncludesOnlyExistingRegularConfig(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing.ini")
	if files := existingBackupFiles(missing); len(files) != 0 {
		t.Fatalf("missing config included = %v", files)
	}
	if files := existingBackupFiles(root); len(files) != 0 {
		t.Fatalf("directory config included = %v", files)
	}
	configPath := filepath.Join(root, "prods.ini")
	if err := os.WriteFile(configPath, []byte("listen=:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if files := existingBackupFiles(configPath); files["host-config"] != configPath || len(files) != 1 {
		t.Fatalf("regular config files = %v", files)
	}
}

func allowBackupTestCleanup(t *testing.T, root string) {
	t.Helper()
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			mode := os.FileMode(0o600)
			if entry.IsDir() {
				mode = 0o700
			}
			_ = os.Chmod(path, mode)
			return nil
		})
	})
}
