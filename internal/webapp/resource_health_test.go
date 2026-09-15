package webapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/platform"
	"prods/internal/storage/sqlite"
)

func TestApplyRuntimeHealthSurfacesLatestManagerFailure(t *testing.T) {
	failureTime := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	component := systemComponentHealth{Component: "backup", Status: "Normal", Summary: "ok"}
	applyRuntimeHealth(&component, platform.RuntimeHealthSnapshot{
		Running: true, LastAttemptUTC: failureTime, LastFailureUTC: failureTime,
		ConsecutiveFailures: 2, FailureStage: "reconcile",
	}, "latest runtime cycle failed")
	if component.Status != "Warning" || component.Summary != "latest runtime cycle failed" ||
		component.RuntimeRunning == nil || !*component.RuntimeRunning ||
		component.RuntimeFailureStage != "reconcile" || component.ConsecutiveFailures != 2 ||
		component.LastRuntimeFailureUTC != failureTime.Format(time.RFC3339Nano) {
		t.Fatalf("runtime health component=%+v", component)
	}
}

func TestBackupResourceFailureIsVisibleButDoesNotStopReadiness(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	backupPath := filepath.Join(root, "unavailable-backups")
	store, err := sqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	gate := platform.NewResourceGate(func(path string) (platform.ResourceStats, error) {
		if filepath.Clean(path) == filepath.Clean(backupPath) {
			return platform.ResourceStats{VolumeID: "backup", TotalBytes: 1 << 30, FreeBytes: 0}, nil
		}
		return platform.ResourceStats{
			VolumeID: "data", TotalBytes: 10 << 30, FreeBytes: 5 << 30,
			FreeInodes: 1_000_000, SupportsInodes: true,
		}, nil
	})
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		DatabasePath: databasePath, BackupDir: backupPath, WorkDir: filepath.Join(root, "work"),
		AssetDir: filepath.Join(root, "assets"), PublicDir: filepath.Join(root, "generated"), ResourceGate: gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginAdmin(t, client, server.URL)

	ready, err := client.Get(server.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	if ready.StatusCode != http.StatusOK {
		t.Fatalf("backup-only failure changed readiness to %d", ready.StatusCode)
	}
	_ = ready.Body.Close()

	response, err := client.Get(server.URL + "/admin/api/system/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}
	var body struct {
		Status    string                 `json:"status"`
		Resources []systemResourceHealth `json:"resources"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "Critical" {
		t.Fatalf("aggregate health = %q", body.Status)
	}
	foundBackup := false
	for _, resource := range body.Resources {
		if resource.Resource == "backups" {
			foundBackup = true
			if resource.Status != "Critical" || resource.Reason == "" {
				t.Fatalf("backup health = %+v", resource)
			}
		}
	}
	if !foundBackup {
		t.Fatal("backup resource missing from health response")
	}
}

func TestWebsiteAssetResourceRejectionPrecedesUploadWrite(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	assetPath := filepath.Join(root, "assets")
	store, err := sqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	gate := platform.NewResourceGate(func(path string) (platform.ResourceStats, error) {
		if filepath.Clean(path) == filepath.Clean(assetPath) {
			return platform.ResourceStats{VolumeID: "assets", TotalBytes: 1 << 30, FreeBytes: 0}, nil
		}
		return platform.ResourceStats{VolumeID: "data", TotalBytes: 10 << 30, FreeBytes: 5 << 30}, nil
	})
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		DatabasePath: databasePath, AssetDir: assetPath, WorkDir: filepath.Join(root, "work"),
		PublicDir: filepath.Join(root, "generated"), BackupDir: filepath.Join(root, "backups"), ResourceGate: gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	csrf := loginAdmin(t, client, server.URL)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("not read because admission must reject first"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/admin/api/website/assets", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusInsufficientStorage {
		t.Fatalf("upload status = %d", response.StatusCode)
	}
	if _, err := os.Stat(assetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("asset root was touched before admission: %v", err)
	}
}

func TestDatabaseResourceHardStopChangesReadinessButNotLiveness(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	store, err := sqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	gate := platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{VolumeID: "database", TotalBytes: 1 << 30, FreeBytes: 0}, nil
	})
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		DatabasePath: databasePath, WorkDir: filepath.Join(root, "work"), ResourceGate: gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/live", nil); response.Code != http.StatusOK {
		t.Fatalf("liveness = %d", response.Code)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/ready", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d", response.Code)
	}
}

func TestResourceHealthWarningDoesNotBlockReadiness(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	store, err := sqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	gate := platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{
			VolumeID:       "shared-warning-volume",
			TotalBytes:     10 << 30,
			FreeBytes:      400 << 20,
			FreeInodes:     900,
			SupportsInodes: true,
		}, nil
	})
	app, _, err := New(store, Config{
		BaseURL:        "http://catalog.example.test",
		AdminToken:     "test-admin-token",
		EnablePOCAdmin: true,
		DatabasePath:   databasePath,
		AssetDir:       filepath.Join(root, "assets"),
		WorkDir:        filepath.Join(root, "work"),
		PublicDir:      filepath.Join(root, "generated"),
		BackupDir:      filepath.Join(root, "backups"),
		ResourceGate:   gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)

	ready, err := http.Get(server.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	_ = ready.Body.Close()
	if ready.StatusCode != http.StatusOK {
		t.Fatalf("warning readiness status=%d", ready.StatusCode)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	_ = loginAdmin(t, client, server.URL)
	response, err := client.Get(server.URL + "/admin/api/system/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", response.StatusCode)
	}
	var body systemHealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "Warning" {
		t.Fatalf("health status=%q, want Warning", body.Status)
	}
	if len(body.Resources) == 0 {
		t.Fatal("health response has no resources")
	}
	for _, resource := range body.Resources {
		if resource.Status != "Warning" {
			t.Fatalf("resource %s status=%q, want Warning", resource.Resource, resource.Status)
		}
		if resource.AdmissionFloorBytes != resourceByteHeadroom {
			t.Fatalf("resource %s admission floor=%d", resource.Resource, resource.AdmissionFloorBytes)
		}
		if resource.WarningFloorBytes != resourceWarningBytes {
			t.Fatalf("resource %s warning floor=%d", resource.Resource, resource.WarningFloorBytes)
		}
		if resource.RecommendedAction == "" {
			t.Fatalf("resource %s missing remediation", resource.Resource)
		}
	}
}

func TestResourceWarningAccountsForReservationsAndInodes(t *testing.T) {
	if !resourceWarning(platform.ResourceSnapshot{ResourceStats: platform.ResourceStats{TotalBytes: 20 << 30, FreeBytes: 2 << 30}, ReservedBytes: 1200 << 20}) {
		t.Fatal("reserved bytes should cross the warning floor")
	}
	if !resourceWarning(platform.ResourceSnapshot{ResourceStats: platform.ResourceStats{TotalBytes: 20 << 30, FreeBytes: 4 << 30, FreeInodes: 1200, SupportsInodes: true}, ReservedInodes: 300}) {
		t.Fatal("reserved inodes should cross the warning floor")
	}
	if resourceWarning(platform.ResourceSnapshot{ResourceStats: platform.ResourceStats{TotalBytes: 20 << 30, FreeBytes: 4 << 30, FreeInodes: 10_000, SupportsInodes: true}}) {
		t.Fatal("healthy headroom should not warn")
	}
}

func TestDatabaseHardStopPrecedesRiskyAdminCommitValidation(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	store, err := sqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	gate := platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
		return platform.ResourceStats{VolumeID: "database-critical", TotalBytes: 1 << 30, FreeBytes: 0}, nil
	})
	app, _, err := New(store, Config{
		BaseURL:        "http://catalog.example.test",
		AdminToken:     "test-admin-token",
		EnablePOCAdmin: true,
		DatabasePath:   databasePath,
		AssetDir:       filepath.Join(root, "assets"),
		WorkDir:        filepath.Join(root, "work"),
		PublicDir:      filepath.Join(root, "generated"),
		BackupDir:      filepath.Join(root, "backups"),
		ResourceGate:   gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	csrf := loginAdmin(t, client, server.URL)

	for _, path := range []string{
		"/admin/api/imports/not-a-job/commit",
		"/admin/api/product-bulk/not-a-run/execute",
		"/admin/api/website/public-copy/import/commit",
	} {
		request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewBufferString("{}"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", csrf)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusInsufficientStorage {
			t.Fatalf("%s status=%d, want %d", path, response.StatusCode, http.StatusInsufficientStorage)
		}
	}
}
