package webapp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	storesqlite "prods/internal/storage/sqlite"
)

func TestBeginDrainKeepsLivenessAndRejectsNewAdmissions(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(filepath.Join(root, "data", "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, _, err := New(store, Config{
		PublicDir:      filepath.Join(root, "generated", "public"),
		AssetDir:       filepath.Join(root, "data", "assets"),
		WorkDir:        filepath.Join(root, "work"),
		BaseURL:        "https://example.test",
		EnablePOCAdmin: true,
		SecureCookies:  true,
		EnforceHost:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	if response := drainRequest(app, http.MethodGet, "/health/ready"); response.Code != http.StatusOK {
		t.Fatalf("ready before drain = %d", response.Code)
	}
	app.BeginDrain()
	app.importMu.Lock()
	closing := app.closing
	app.importMu.Unlock()
	if !closing {
		t.Fatal("drain did not stop new import dispatch")
	}
	if response := drainRequest(app, http.MethodGet, "/health/live"); response.Code != http.StatusOK {
		t.Fatalf("liveness during drain = %d", response.Code)
	}
	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health/ready"},
		{http.MethodGet, "/search?q=part"},
		{http.MethodPost, "/rfq"},
		{http.MethodGet, "/admin"},
	} {
		response := drainRequest(app, request.method, request.path)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s during drain = %d", request.method, request.path, response.Code)
		}
	}
}

func drainRequest(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://example.test"+target, nil)
	request.Host = "example.test"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
