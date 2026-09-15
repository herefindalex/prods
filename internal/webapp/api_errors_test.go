package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	storesqlite "prods/internal/storage/sqlite"
)

func TestWriteAPIErrorUsesStableCodeAndConfiguredLocale(t *testing.T) {
	server := newAPIErrorTestServer(t, []string{"en-US", "zh-TW"})
	tests := []struct {
		name           string
		acceptLanguage string
		wantLocale     string
		wantMessage    string
	}{
		{name: "traditional Chinese", acceptLanguage: "zh-TW", wantLocale: "zh-TW", wantMessage: "請求資料格式無效。"},
		{name: "English", acceptLanguage: "en-US", wantLocale: "en-US", wantMessage: "The request data is invalid."},
		{name: "unsupported falls back to site default", acceptLanguage: "fr-FR", wantLocale: "en-US", wantMessage: "The request data is invalid."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/admin/api/products", nil)
			request.Header.Set("Accept-Language", test.acceptLanguage)
			response := httptest.NewRecorder()

			server.writeAPIError(response, request, http.StatusBadRequest, apiCodeInvalidJSON)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if got := response.Header().Get("Content-Language"); got != test.wantLocale {
				t.Fatalf("Content-Language = %q, want %q", got, test.wantLocale)
			}
			if got := response.Header().Get("Vary"); got != "Accept-Language" {
				t.Fatalf("Vary = %q, want Accept-Language", got)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type = %q", got)
			}
			var body apiErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != apiCodeInvalidJSON || body.Error != test.wantMessage {
				t.Fatalf("error response = %+v, want code %q and message %q", body, apiCodeInvalidJSON, test.wantMessage)
			}
		})
	}
}

func TestWriteAPIErrorRejectsUnconfiguredEmbeddedLocale(t *testing.T) {
	server := newAPIErrorTestServer(t, []string{"en-US"})
	request := httptest.NewRequest(http.MethodGet, "/admin/api/example", nil)
	request.Header.Set("Accept-Language", "zh-TW")
	response := httptest.NewRecorder()

	server.writeAPIError(response, request, http.StatusForbidden, apiCodeForbidden)

	var body apiErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := response.Header().Get("Content-Language"); got != "en-US" {
		t.Fatalf("Content-Language = %q, want en-US", got)
	}
	if body.Code != apiCodeForbidden || body.Error != "You do not have permission to perform this action." {
		t.Fatalf("error response = %+v", body)
	}
}

func TestRequireCapabilityReturnsLocalizedStableUnauthorizedError(t *testing.T) {
	server := newAPIErrorTestServer(t, []string{"en-US", "zh-TW"})
	request := httptest.NewRequest(http.MethodGet, "/admin/api/products", nil)
	request.Header.Set("Accept-Language", "zh-TW")
	response := httptest.NewRecorder()

	if _, ok := server.requireCapability(response, request, "catalog.view", false); ok {
		t.Fatal("unauthenticated request unexpectedly admitted")
	}

	var body apiErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnauthorized || body.Code != apiCodeUnauthorized || body.Error != "登入工作階段不存在或已過期，請重新登入。" {
		t.Fatalf("status=%d error response=%+v", response.Code, body)
	}
}

func TestWriteAPIErrorDoesNotExposeUnknownFallback(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "/admin/api/example", nil)
	response := httptest.NewRecorder()

	server.writeAPIError(response, request, http.StatusInternalServerError, "database_driver_secret")

	var body apiErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != apiCodeInternal || body.Error != "An internal error occurred." {
		t.Fatalf("error response = %+v", body)
	}
}

func newAPIErrorTestServer(t *testing.T, supportedLocales []string) *Server {
	t.Helper()
	store, err := storesqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail:       "owner@example.test",
		OwnerDisplayName: "Owner",
		PasswordHash:     "test-password-hash",
		DefaultLocale:    "en-US",
		SupportedLocales: supportedLocales,
		TimeZone:         "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	return &Server{store: store}
}
