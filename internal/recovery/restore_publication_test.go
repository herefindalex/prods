package recovery

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	storesqlite "prods/internal/storage/sqlite"
)

func TestRestoreRestartReconcilesPublicIndexFromRestoredDatabase(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	databasePath := filepath.Join(dataRoot, "prods.db")
	assetRoot := filepath.Join(dataRoot, "assets")
	controlRoot := filepath.Join(dataRoot, "control")
	backupRoot := filepath.Join(root, "backups")
	publicRoot := filepath.Join(root, "generated", "public")
	if err := os.MkdirAll(assetRoot, 0o700); err != nil {
		t.Fatal(err)
	}

	store, err := storesqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail:       "owner@example.test",
		OwnerDisplayName: "Owner",
		PasswordHash:     passwordHash,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US"},
		TimeZone:         "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber:  "RESTORE-PUBLIC-1",
		Name:        "Restored revision A",
		Description: "public revision A",
		Status:      catalog.Published,
	})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := publishing.NewEngine(t.Context(), store, publishing.Config{
		Root: publicRoot, AssetRoot: assetRoot, BaseURL: "https://example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	engine.Wake()
	waitForActiveRevision(t, store, product.ID, product.Revision)

	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: backupRoot, AssetDir: assetRoot, ApplicationVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	product.Name = "Newer revision B"
	product.Description = "public revision B"
	product, err = store.UpdateProduct(t.Context(), owner.ID, product.Revision, product)
	if err != nil {
		t.Fatal(err)
	}
	engine.Wake()
	waitForActiveRevision(t, store, product.ID, product.Revision)
	engine.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	journalPath, err := PrepareRestore(t.Context(), RestoreConfig{
		BackupPath: filepath.Join(backupRoot, manifest.ID),
		ControlDir: controlRoot,
		Targets: map[string]RestoreTarget{
			"database": {Path: databasePath, Kind: "file"},
			"assets":   {Path: assetRoot, Kind: "directory"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ResumeRestore(t.Context(), journalPath); err != nil {
		t.Fatal(err)
	}

	restoredStore, err := storesqlite.OpenReady(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredStore.Close()
	restartedEngine, err := publishing.NewEngine(t.Context(), restoredStore, publishing.Config{
		Root: publicRoot, AssetRoot: assetRoot, BaseURL: "https://example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer restartedEngine.Close()
	active := waitForActiveRevision(t, restoredStore, product.ID, 1)

	request := httptest.NewRequest("GET", "https://example.test"+active.Route, nil)
	response := httptest.NewRecorder()
	if served := restartedEngine.ServePath(response, request); !served {
		t.Fatalf("restored route %s was not admitted", active.Route)
	}
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, "Restored revision A") || strings.Contains(body, "Newer revision B") {
		t.Fatalf("restored public response status=%d body=%s", response.Code, body)
	}
}

func waitForActiveRevision(t *testing.T, store *storesqlite.Store, productID string, revision int64) publishing.ActivePublication {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		active, err := store.ActivePublications(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range active {
			if item.ProductID == productID && item.SourceRevision == revision {
				return item
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("product %s revision %d did not become active", productID, revision)
	return publishing.ActivePublication{}
}
