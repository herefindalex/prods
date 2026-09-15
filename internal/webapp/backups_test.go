package webapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"prods/internal/identity"
	"prods/internal/platform"
	"prods/internal/recovery"
	"prods/internal/storage/sqlite"
)

type backupWebFixture struct {
	root         string
	databasePath string
	assetPath    string
	backupPath   string
	configPath   string
	store        *sqlite.Store
	owner        identity.User
	app          *Server
	server       *httptest.Server
	client       *http.Client
	csrf         string
}

func newBackupWebFixture(t *testing.T, gate *platform.ResourceGate) backupWebFixture {
	t.Helper()
	root := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return os.Chmod(path, 0o600)
		})
	})
	databasePath := filepath.Join(root, "data", "prods.db")
	assetPath := filepath.Join(root, "data", "assets")
	backupPath := filepath.Join(root, "backups")
	configPath := filepath.Join(root, "runtime", "prods.ini")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("listen=:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
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
	settings, err := store.BackupSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = false
	if _, err := store.UpdateBackupSettings(t.Context(), owner.ID, settings.Version, settings); err != nil {
		t.Fatal(err)
	}
	if gate == nil {
		gate = platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
			return platform.ResourceStats{
				VolumeID:       "test-volume",
				TotalBytes:     1 << 40,
				FreeBytes:      1 << 39,
				FreeInodes:     1_000_000,
				SupportsInodes: true,
			}, nil
		})
	}
	app, _, err := New(store, Config{
		BaseURL:               "http://catalog.example.test",
		DatabasePath:          databasePath,
		AssetDir:              assetPath,
		BackupDir:             backupPath,
		WorkDir:               filepath.Join(root, "work"),
		PublicDir:             filepath.Join(root, "generated"),
		ResourceGate:          gate,
		EnableBackupScheduler: true,
		BackupFiles:           map[string]string{"host-config": configPath},
		ApplicationVersion:    "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	return backupWebFixture{
		root:         root,
		databasePath: databasePath,
		assetPath:    assetPath,
		backupPath:   backupPath,
		configPath:   configPath,
		store:        store,
		owner:        owner,
		app:          app,
		server:       server,
		client:       client,
		csrf:         csrf,
	}
}

func TestBackupAPIRequiresSystemManageAndRejectsStaleSettingsRevision(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)

	unauthorized, err := http.Get(fixture.server.URL + "/admin/api/backups")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, unauthorized); unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized backup status=%d body=%s", unauthorized.StatusCode, body)
	}

	role, err := fixture.store.CreateRole(t.Context(), fixture.owner.ID, identity.Role{
		Name:         "Admin without system management",
		Capabilities: []identity.Capability{identity.CapabilityAdminAccess},
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := fixture.store.CreateUser(t.Context(), fixture.owner.ID, "viewer@example.test", "Viewer", role.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	viewerHash, err := identity.HashPassword("viewerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.SetUserPassword(t.Context(), grant.Token, viewerHash, time.Now()); err != nil {
		t.Fatal(err)
	}
	viewer := newCookieClient(t)
	_ = loginNormalAdmin(t, viewer, fixture.server.URL, "viewer@example.test", "viewerpass1")
	forbidden, err := viewer.Get(fixture.server.URL + "/admin/api/backups")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, forbidden); forbidden.StatusCode != http.StatusForbidden {
		t.Fatalf("backup status without system.manage=%d body=%s", forbidden.StatusCode, body)
	}

	status, err := fixture.client.Get(fixture.server.URL + "/admin/api/backups")
	if err != nil {
		t.Fatal(err)
	}
	if status.StatusCode != http.StatusOK {
		t.Fatalf("owner backup status=%d body=%s", status.StatusCode, responseBody(t, status))
	}
	var current backupStatusResponse
	decodeResponseJSON(t, status, &current)
	if !current.ContainsSensitiveData {
		t.Fatal("backup status did not mark restore points as sensitive")
	}
	updated := current.Settings
	updated.LocalTime = "04:15"
	accepted := putBackupSettings(t, fixture, current.Settings.Version, updated)
	if accepted.StatusCode != http.StatusOK {
		t.Fatalf("update backup settings=%d body=%s", accepted.StatusCode, responseBody(t, accepted))
	}
	var saved sqlite.BackupSettings
	decodeResponseJSON(t, accepted, &saved)
	if saved.Version != current.Settings.Version+1 || saved.LocalTime != "04:15" {
		t.Fatalf("saved backup settings=%+v", saved)
	}

	stale := putBackupSettings(t, fixture, current.Settings.Version, updated)
	if body := responseBody(t, stale); stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale backup settings=%d body=%s", stale.StatusCode, body)
	}
}

func TestManualBackupAPIReturnsDurableVerifiedReceipt(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	before := backupHealthComponent(t, fixture)
	if before.Status != "Warning" || before.LastSuccessUTC != "" {
		t.Fatalf("backup health before first success=%+v", before)
	}
	response := postAdminJSON(t, fixture.client, fixture.server.URL+"/admin/api/backups/run", fixture.csrf, `{}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("manual backup=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var run sqlite.BackupRun
	decodeResponseJSON(t, response, &run)
	if run.ID == "" || run.Status != "succeeded" || run.BackupID == "" || run.SizeBytes <= 0 ||
		!run.ContentVerified || !run.ReadOnlyApplied || run.CompletedAt == "" {
		t.Fatalf("manual backup receipt=%+v", run)
	}
	manifest, err := recovery.LoadManifest(filepath.Join(fixture.backupPath, run.BackupID))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != run.ID || manifest.Kind != "manual" || !manifest.ContentVerified || !manifest.ReadOnlyApplied ||
		manifest.SQLiteVersion == "" || !manifest.ContainsSensitiveData {
		t.Fatalf("manual backup manifest=%+v", manifest)
	}
	entry, ok := manifest.Files["host-config"]
	if !ok || entry.Path != "files/host-config" {
		t.Fatalf("manual backup host config entry=%+v", entry)
	}
	if body, err := os.ReadFile(filepath.Join(fixture.backupPath, run.BackupID, filepath.FromSlash(entry.Path))); err != nil || string(body) != "listen=:8080\n" {
		t.Fatalf("manual backup host config=%q err=%v", body, err)
	}
	runs, err := fixture.store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].Status != "succeeded" || runs[0].BackupID != run.BackupID {
		t.Fatalf("durable backup runs=%+v", runs)
	}
	after := backupHealthComponent(t, fixture)
	if after.Status != "Normal" || after.LastSuccessUTC == "" || after.LastRunStatus != "succeeded" {
		t.Fatalf("backup health after success=%+v", after)
	}
}

func TestBackupDestinationFailureLeavesPublicRFQAndReadinessAvailable(t *testing.T) {
	var backupPath string
	gate := platform.NewResourceGate(func(path string) (platform.ResourceStats, error) {
		if backupPath != "" && filepath.Clean(path) == filepath.Clean(backupPath) {
			return platform.ResourceStats{VolumeID: "backup-volume", TotalBytes: 1 << 30, FreeBytes: 0}, nil
		}
		return platform.ResourceStats{
			VolumeID:       "data-volume",
			TotalBytes:     1 << 40,
			FreeBytes:      1 << 39,
			FreeInodes:     1_000_000,
			SupportsInodes: true,
		}, nil
	})
	fixture := newBackupWebFixture(t, gate)
	backupPath = fixture.backupPath

	failed := postAdminJSON(t, fixture.client, fixture.server.URL+"/admin/api/backups/run", fixture.csrf, `{}`)
	if body := responseBody(t, failed); failed.StatusCode != http.StatusInsufficientStorage {
		t.Fatalf("failed backup=%d body=%s", failed.StatusCode, body)
	}
	runs, err := fixture.store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" || runs[0].ErrorMessage == "" || runs[0].BackupID != "" {
		t.Fatalf("failed backup receipt=%+v", runs)
	}
	if _, err := os.Stat(fixture.backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup destination touched before resource admission: %v", err)
	}

	ready, err := fixture.client.Get(fixture.server.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, ready); ready.StatusCode != http.StatusOK {
		t.Fatalf("readiness after backup failure=%d body=%s", ready.StatusCode, body)
	}
	public, err := fixture.client.Get(fixture.server.URL + "/search")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, public); public.StatusCode != http.StatusOK {
		t.Fatalf("public after backup failure=%d body=%s", public.StatusCode, body)
	}

	publicClient := newCookieClient(t)
	form, err := publicClient.Get(fixture.server.URL + "/rfq?query=BACKUP-FAILURE-PART")
	if err != nil {
		t.Fatal(err)
	}
	page := responseBody(t, form)
	csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(page)
	key := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page)
	if len(csrf) != 2 || len(key) != 2 {
		t.Fatalf("RFQ form keys missing after backup failure: %s", page)
	}
	receipt, err := publicClient.PostForm(fixture.server.URL+"/rfq", url.Values{
		"csrf_token":     {csrf[1]},
		"submission_key": {key[1]},
		"kind":           {"requested"},
		"raw_query":      {"BACKUP-FAILURE-PART"},
		"requested":      {"BACKUP-FAILURE-PART"},
		"name":           {"Ada"},
		"email":          {"ada@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, receipt); receipt.StatusCode != http.StatusOK {
		t.Fatalf("RFQ after backup failure=%d body=%s", receipt.StatusCode, body)
	}
}

func putBackupSettings(t *testing.T, fixture backupWebFixture, expectedVersion int64, settings sqlite.BackupSettings) *http.Response {
	t.Helper()
	body, err := json.Marshal(struct {
		ExpectedVersion int64                 `json:"expected_version"`
		Settings        sqlite.BackupSettings `json:"settings"`
	}{ExpectedVersion: expectedVersion, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPut, fixture.server.URL+"/admin/api/backups/settings", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", fixture.csrf)
	response, err := fixture.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func backupHealthComponent(t *testing.T, fixture backupWebFixture) systemComponentHealth {
	t.Helper()
	response, err := fixture.client.Get(fixture.server.URL + "/admin/api/system/health")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("system health=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var health systemHealthResponse
	decodeResponseJSON(t, response, &health)
	for _, component := range health.Components {
		if component.Component == "backup" {
			return component
		}
	}
	t.Fatal("backup component missing from system health")
	return systemComponentHealth{}
}
