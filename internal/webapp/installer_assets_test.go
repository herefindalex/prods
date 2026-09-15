package webapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	storesqlite "prods/internal/storage/sqlite"
)

func TestInstallerUsesOnlyItsDedicatedStaticStyles(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.Create(filepath.Join(dataRoot, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	installer, _, err := NewInstaller(store, InstallerConfig{
		BootstrapToken: "bootstrap-token",
		DataDir:        dataRoot,
		BackupDir:      filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}

	page := installerAssetRequest(installer, "/install?token=bootstrap-token")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `/static/system/system.css`) ||
		!strings.Contains(page.Body.String(), `/static/system/system.js`) ||
		strings.Contains(page.Body.String(), `/static/public/public.css`) {
		t.Fatalf("installer page status=%d body=%s", page.Code, page.Body.String())
	}
	stylesheet := installerAssetRequest(installer, "/static/system/system.css")
	if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), "font-family") {
		t.Fatalf("installer stylesheet status=%d body=%s", stylesheet.Code, stylesheet.Body.String())
	}
	script := installerAssetRequest(installer, "/static/system/system.js")
	if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), "system-root") {
		t.Fatalf("installer script status=%d body=%s", script.Code, script.Body.String())
	}
	publicAsset := installerAssetRequest(installer, "/static/public/public.css")
	if publicAsset.Code != http.StatusNotFound {
		t.Fatalf("public asset exposed by installer: status=%d", publicAsset.Code)
	}
}

func installerAssetRequest(handler http.Handler, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
