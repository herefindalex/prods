package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	storesqlite "prods/internal/storage/sqlite"
)

func TestCreateBackupIncludesExplicitHostConfigFile(t *testing.T) {
	root := t.TempDir()
	store, assetRoot, backupRoot := newFileBackupTestStore(t, root)
	configPath := filepath.Join(root, "binary", "prods.ini")
	configBody := []byte("listen=:8080\nbase_url=http://localhost:8080\n")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir:            backupRoot,
		AssetDir:             assetRoot,
		Files:                map[string]string{"host-config": configPath},
		ExternalRequirements: []string{"environment:PRODS_SMTP_PASSWORD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SQLiteVersion == "" || !manifest.ContainsSensitiveData {
		t.Fatalf("backup compatibility and sensitivity metadata = %+v", manifest)
	}
	if len(manifest.ExternalRequirements) != 1 || manifest.ExternalRequirements[0] != "environment:PRODS_SMTP_PASSWORD" {
		t.Fatalf("external requirements = %#v", manifest.ExternalRequirements)
	}
	entry, ok := manifest.Files["host-config"]
	if !ok {
		t.Fatalf("host-config missing from manifest: %+v", manifest.Files)
	}
	wantHash := sha256.Sum256(configBody)
	if entry.Path != "files/host-config" || entry.Size != int64(len(configBody)) || entry.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("host-config manifest entry = %+v", entry)
	}
	backupPath := filepath.Join(backupRoot, manifest.ID)
	if copied, err := os.ReadFile(filepath.Join(backupPath, "files", "host-config")); err != nil || string(copied) != string(configBody) {
		t.Fatalf("backed up host config = %q, %v", copied, err)
	}
	loaded, err := LoadManifest(backupPath)
	if err != nil || loaded.Files["host-config"] != entry {
		t.Fatalf("loaded host config entry = %+v, %v", loaded.Files["host-config"], err)
	}
	if len(loaded.ExternalRequirements) != 1 || loaded.ExternalRequirements[0] != "environment:PRODS_SMTP_PASSWORD" {
		t.Fatalf("loaded external requirements = %#v", loaded.ExternalRequirements)
	}
}

func TestLoadManifestAcceptsVersionOneWithoutExplicitFiles(t *testing.T) {
	root := t.TempDir()
	store, assetRoot, backupRoot := newFileBackupTestStore(t, root)
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{BackupDir: backupRoot, AssetDir: assetRoot})
	if err != nil {
		t.Fatal(err)
	}
	manifest.ManifestVersion = 1
	manifest.Files = nil
	manifest.SQLiteVersion = ""
	manifest.ContainsSensitiveData = false
	backupPath := filepath.Join(backupRoot, manifest.ID)
	writeBackupTestManifest(t, backupPath, manifest)

	loaded, err := LoadManifest(backupPath)
	if err != nil || loaded.ManifestVersion != 1 || len(loaded.Files) != 0 {
		t.Fatalf("version one manifest = %+v, %v", loaded, err)
	}
}

func TestLoadManifestRejectsUnsafeFilePathsSeparatelyFromInvalidMetadata(t *testing.T) {
	root := t.TempDir()
	store, assetRoot, backupRoot := newFileBackupTestStore(t, root)
	configPath := filepath.Join(root, "prods.ini")
	if err := os.WriteFile(configPath, []byte("listen=:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: backupRoot,
		AssetDir:  assetRoot,
		Files:     map[string]string{"host-config": configPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(backupRoot, manifest.ID)
	original := manifest.Files["host-config"]

	tests := []struct {
		name string
		file ManifestFile
		want error
	}{
		{name: "parent traversal", file: ManifestFile{Path: "files/../host-config", SHA256: original.SHA256, Size: original.Size}, want: ErrUnsafeBackupPath},
		{name: "portable backslash", file: ManifestFile{Path: `files\host-config`, SHA256: original.SHA256, Size: original.Size}, want: ErrUnsafeBackupPath},
		{name: "wrong fixed path", file: ManifestFile{Path: "files/other", SHA256: original.SHA256, Size: original.Size}, want: ErrUnsafeBackupPath},
		{name: "invalid hash", file: ManifestFile{Path: original.Path, SHA256: "not-a-sha256", Size: original.Size}, want: ErrInvalidBackup},
		{name: "negative size", file: ManifestFile{Path: original.Path, SHA256: original.SHA256, Size: -1}, want: ErrInvalidBackup},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := manifest
			candidate.Files = map[string]ManifestFile{"host-config": test.file}
			writeBackupTestManifest(t, backupPath, candidate)
			_, err := LoadManifest(backupPath)
			if !errors.Is(err, test.want) {
				t.Fatalf("LoadManifest error = %v, want %v", err, test.want)
			}
		})
	}

	manifest.ManifestVersion = 1
	writeBackupTestManifest(t, backupPath, manifest)
	if _, err := LoadManifest(backupPath); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("version one manifest accepted explicit files: %v", err)
	}
}

func TestCreateBackupRejectsBackupAndSourcePathOverlapBeforeSnapshot(t *testing.T) {
	t.Run("backup inside root", func(t *testing.T) {
		root := t.TempDir()
		assetRoot := filepath.Join(root, "assets")
		configRoot := filepath.Join(root, "config")
		backupRoot := filepath.Join(configRoot, "backups")
		for _, directory := range []string{assetRoot, configRoot} {
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		assertBackupTopologyRejectedBeforeSnapshot(t, BackupConfig{
			BackupDir: backupRoot, AssetDir: assetRoot, Roots: map[string]string{"config": configRoot},
		})
	})

	t.Run("root inside backup", func(t *testing.T) {
		root := t.TempDir()
		assetRoot := filepath.Join(root, "assets")
		backupRoot := filepath.Join(root, "backups")
		configRoot := filepath.Join(backupRoot, "config")
		for _, directory := range []string{assetRoot, configRoot} {
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		assertBackupTopologyRejectedBeforeSnapshot(t, BackupConfig{
			BackupDir: backupRoot, AssetDir: assetRoot, Roots: map[string]string{"config": configRoot},
		})
	})

	t.Run("explicit file inside backup", func(t *testing.T) {
		root := t.TempDir()
		assetRoot := filepath.Join(root, "assets")
		backupRoot := filepath.Join(root, "backups")
		if err := os.MkdirAll(assetRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		assertBackupTopologyRejectedBeforeSnapshot(t, BackupConfig{
			BackupDir: backupRoot, AssetDir: assetRoot,
			Files: map[string]string{"host-config": filepath.Join(backupRoot, "prods.ini")},
		})
	})

	t.Run("symlink alias", func(t *testing.T) {
		root := t.TempDir()
		realRoot := filepath.Join(root, "real-config")
		assetRoot := filepath.Join(root, "assets")
		for _, directory := range []string{realRoot, assetRoot} {
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		alias := filepath.Join(root, "config-alias")
		if err := os.Symlink(realRoot, alias); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		assertBackupTopologyRejectedBeforeSnapshot(t, BackupConfig{
			BackupDir: filepath.Join(alias, "backups"), AssetDir: assetRoot,
			Roots: map[string]string{"config": realRoot},
		})
	})
}

func assertBackupTopologyRejectedBeforeSnapshot(t *testing.T, config BackupConfig) {
	t.Helper()
	snapshotter := &failingSnapshotter{}
	_, err := CreateBackup(t.Context(), snapshotter, config)
	if !errors.Is(err, ErrUnsafeBackupPath) {
		t.Fatalf("overlapping backup topology error = %v", err)
	}
	if snapshotter.called {
		t.Fatal("snapshot ran before overlapping backup topology was rejected")
	}
}

func newFileBackupTestStore(t *testing.T, root string) (*storesqlite.Store, string, string) {
	t.Helper()
	dataRoot := filepath.Join(root, "data")
	assetRoot := filepath.Join(dataRoot, "assets")
	backupRoot := filepath.Join(root, "backups")
	if err := os.MkdirAll(assetRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(filepath.Join(dataRoot, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, assetRoot, backupRoot
}

func writeBackupTestManifest(t *testing.T, backupPath string, manifest Manifest) {
	t.Helper()
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupPath, "manifest.json"), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
