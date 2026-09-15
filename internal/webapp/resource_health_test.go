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
