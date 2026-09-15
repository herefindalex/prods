package webapp

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"prods/internal/identity"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func TestManualMaintenancePausesPublicAndRFQButKeepsAdminSystemEntrypoints(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	before, err := http.Get(fixture.server.URL + "/search")
	if err != nil {
		t.Fatal(err)
	}
	before.Body.Close()
	if before.StatusCode != http.StatusOK {
		t.Fatalf("public before maintenance=%d", before.StatusCode)
	}

	endpoint := fixture.server.URL + "/admin/api/system/maintenance"
	withoutCSRF := putAdminJSON(t, fixture.client, endpoint, "", `{"expected_revision":1,"active":true,"message":"planned"}`)
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("maintenance without CSRF=%d", withoutCSRF.StatusCode)
	}
	withoutCSRF.Body.Close()
	activatedResponse := putAdminJSON(t, fixture.client, endpoint, fixture.csrf,
		`{"expected_revision":1,"active":true,"message":"Planned <script>alert(1)</script>"}`)
	if activatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("activate maintenance=%d body=%s", activatedResponse.StatusCode, responseBody(t, activatedResponse))
	}
	var activated site.Maintenance
	decodeResponseJSON(t, activatedResponse, &activated)
	if !activated.Active || activated.Revision != 2 {
		t.Fatalf("activated maintenance=%+v", activated)
	}

	for _, path := range []string{"/search", "/rfq", "/products/missing.html", "/assets/missing"} {
		response, err := http.Get(fixture.server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusServiceUnavailable || response.Header.Get("Retry-After") != "300" ||
			!strings.Contains(string(body), `data-mode="maintenance"`) ||
			!strings.Contains(string(body), `/static/system/system.css`) ||
			!strings.Contains(string(body), `/static/system/system.js`) ||
			strings.Contains(string(body), "Planned") || strings.Contains(string(body), "/static/admin/") ||
			strings.Contains(string(body), "/static/public/") || strings.Contains(string(body), "/site.css") ||
			!strings.Contains(response.Header.Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatalf("maintenance %s status=%d headers=%v body=%s", path, response.StatusCode, response.Header, body)
		}
	}
	stateResponse, err := http.Get(fixture.server.URL + "/maintenance/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var publicState struct {
		Active            bool   `json:"active"`
		Message           string `json:"message"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	}
	decodeResponseJSON(t, stateResponse, &publicState)
	if stateResponse.StatusCode != http.StatusOK || !publicState.Active ||
		publicState.Message != "Planned <script>alert(1)</script>" || publicState.RetryAfterSeconds != 300 {
		t.Fatalf("public maintenance state status=%d state=%+v", stateResponse.StatusCode, publicState)
	}
	rfq, err := http.Post(fixture.server.URL+"/api/rfqs", "application/json", strings.NewReader(`{"submission_key":"sub_123456789012345678901234567890","submission":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	rfq.Body.Close()
	if rfq.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("RFQ during maintenance=%d", rfq.StatusCode)
	}

	live, err := http.Get(fixture.server.URL + "/health/live")
	if err != nil {
		t.Fatal(err)
	}
	live.Body.Close()
	if live.StatusCode != http.StatusOK {
		t.Fatalf("liveness during maintenance=%d", live.StatusCode)
	}
	ready, err := http.Get(fixture.server.URL + "/health/ready")
	if err != nil {
		t.Fatal(err)
	}
	ready.Body.Close()
	if ready.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readiness during maintenance=%d", ready.StatusCode)
	}
	adminState, err := fixture.client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	adminState.Body.Close()
	if adminState.StatusCode != http.StatusOK {
		t.Fatalf("Admin maintenance status during maintenance=%d", adminState.StatusCode)
	}
	setPassword, err := http.Get(fixture.server.URL + "/set-password")
	if err != nil {
		t.Fatal(err)
	}
	setPassword.Body.Close()
	if setPassword.StatusCode != http.StatusBadRequest {
		t.Fatalf("set-password entrypoint during maintenance=%d", setPassword.StatusCode)
	}
	staticAsset, err := http.Get(fixture.server.URL + "/static/system/system.css")
	if err != nil {
		t.Fatal(err)
	}
	staticAsset.Body.Close()
	if staticAsset.StatusCode != http.StatusOK {
		t.Fatalf("system asset during maintenance=%d", staticAsset.StatusCode)
	}

	deactivated := putAdminJSON(t, fixture.client, endpoint, fixture.csrf,
		`{"expected_revision":2,"active":false,"message":"Planned maintenance complete"}`)
	if deactivated.StatusCode != http.StatusOK {
		t.Fatalf("deactivate maintenance=%d body=%s", deactivated.StatusCode, responseBody(t, deactivated))
	}
	deactivated.Body.Close()
	after, err := http.Get(fixture.server.URL + "/search")
	if err != nil {
		t.Fatal(err)
	}
	after.Body.Close()
	if after.StatusCode != http.StatusOK {
		t.Fatalf("public after maintenance=%d", after.StatusCode)
	}
}

func TestMaintenanceActivationWaitsForAlreadyAdmittedPublicMutation(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	fixture.app.mux.HandleFunc("POST /test-public-mutation", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	})

	publicDone := make(chan error, 1)
	go func() {
		response, err := http.Post(fixture.server.URL+"/test-public-mutation", "application/json", nil)
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				err = errors.New("unexpected public mutation response")
			}
		}
		publicDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("public mutation did not enter")
	}

	maintenanceDone := make(chan error, 1)
	go func() {
		request, err := http.NewRequest(http.MethodPut, fixture.server.URL+"/admin/api/system/maintenance",
			strings.NewReader(`{"expected_revision":1,"active":true,"message":"draining"}`))
		if err == nil {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", fixture.csrf)
			var response *http.Response
			response, err = fixture.client.Do(request)
			if err == nil {
				response.Body.Close()
				if response.StatusCode != http.StatusOK {
					err = errors.New("unexpected maintenance response")
				}
			}
		}
		maintenanceDone <- err
	}()
	select {
	case err := <-maintenanceDone:
		t.Fatalf("maintenance activated before admitted mutation drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-publicDone; err != nil {
		t.Fatal(err)
	}
	if err := <-maintenanceDone; err != nil {
		t.Fatal(err)
	}

	response, err := http.Post(fixture.server.URL+"/test-public-mutation", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatalf("new mutation after activation status=%d handler calls=%d", response.StatusCode, calls.Load())
	}
}

func TestDurableMaintenanceIsLoadedBeforeNewServerAdmitsPublicRequests(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateSiteMaintenance(t.Context(), owner.ID, 1, true, "Durable maintenance"); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", WorkDir: filepath.Join(root, "work"),
		PublicDir: filepath.Join(root, "public"), AssetDir: filepath.Join(root, "assets"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	response, err := http.Get(server.URL + "/search")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("public request after maintenance restart=%d", response.StatusCode)
	}
}
