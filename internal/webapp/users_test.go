package webapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestUserProvisioningCapabilityAndDisableFlow(t *testing.T) {
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
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

	app, _, err := New(store, Config{BaseURL: "https://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	ownerClient := newCookieClient(t)
	ownerCSRF := loginNormalAdmin(t, ownerClient, server.URL, "owner@example.test", "ownerpass1")

	roleResponse := postAdminJSON(t, ownerClient, server.URL+"/admin/api/roles", ownerCSRF,
		`{"name":"Catalog Viewer","capabilities":["admin.access","catalog.view"]}`)
	if roleResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create role status=%d body=%s", roleResponse.StatusCode, responseBody(t, roleResponse))
	}
	var role identity.Role
	decodeResponseJSON(t, roleResponse, &role)
	if role.ID == "" || len(role.Capabilities) != 2 {
		t.Fatalf("created role = %#v", role)
	}

	userResponse := postAdminJSON(t, ownerClient, server.URL+"/admin/api/users", ownerCSRF,
		`{"email":"viewer@example.test","display_name":"Viewer","role_id":"`+role.ID+`"}`)
	if userResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", userResponse.StatusCode, responseBody(t, userResponse))
	}
	var provisioned struct {
		User           identity.User `json:"user"`
		SetPasswordURL string        `json:"set_password_url"`
	}
	decodeResponseJSON(t, userResponse, &provisioned)
	setPasswordURL, err := url.Parse(provisioned.SetPasswordURL)
	if err != nil {
		t.Fatal(err)
	}
	token := setPasswordURL.Query().Get("token")
	if token == "" || provisioned.User.PasswordHash != "" {
		t.Fatalf("invalid provisioning response: %#v", provisioned)
	}

	setPage, err := ownerClient.Get(server.URL + "/set-password?token=" + url.QueryEscape(token))
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, setPage); setPage.StatusCode != http.StatusOK || !strings.Contains(body, "Set your Prods password") {
		t.Fatalf("set-password page status=%d body=%s", setPage.StatusCode, body)
	}
	setResponse, err := ownerClient.PostForm(server.URL+"/set-password", url.Values{
		"token":                 {token},
		"password":              {"viewerpass1"},
		"password_confirmation": {"viewerpass1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, setResponse); setResponse.StatusCode != http.StatusOK || !strings.Contains(body, "viewer@example.test") {
		t.Fatalf("set-password status=%d body=%s", setResponse.StatusCode, body)
	}
	replay, err := ownerClient.PostForm(server.URL+"/set-password", url.Values{
		"token":                 {token},
		"password":              {"viewerpass1"},
		"password_confirmation": {"viewerpass1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, replay); replay.StatusCode != http.StatusConflict || !strings.Contains(body, "already used") {
		t.Fatalf("set-password replay status=%d body=%s", replay.StatusCode, body)
	}

	viewerClient := newCookieClient(t)
	viewerCSRF := loginNormalAdmin(t, viewerClient, server.URL, "viewer@example.test", "viewerpass1")
	listResponse, err := viewerClient.Get(server.URL + "/admin/api/products")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, listResponse); listResponse.StatusCode != http.StatusOK {
		t.Fatalf("viewer list status=%d body=%s", listResponse.StatusCode, body)
	}
	createResponse := postAdminJSON(t, viewerClient, server.URL+"/admin/api/products", viewerCSRF,
		`{"part_number":"VIEWER-MUST-NOT-CREATE"}`)
	if body := responseBody(t, createResponse); createResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer create status=%d body=%s", createResponse.StatusCode, body)
	}

	disableResponse := postAdminJSON(t, ownerClient,
		server.URL+"/admin/api/users/"+provisioned.User.ID+"/disable", ownerCSRF, `{}`)
	if body := responseBody(t, disableResponse); disableResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("disable user status=%d body=%s", disableResponse.StatusCode, body)
	}
	denied, err := viewerClient.Get(server.URL + "/admin/api/products")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, denied); denied.StatusCode != http.StatusUnauthorized {
		t.Fatalf("disabled session status=%d body=%s", denied.StatusCode, body)
	}

	lastOwnerResponse := postAdminJSON(t, ownerClient,
		server.URL+"/admin/api/users/"+owner.ID+"/disable", ownerCSRF, `{}`)
	if body := responseBody(t, lastOwnerResponse); lastOwnerResponse.StatusCode != http.StatusConflict || !strings.Contains(body, "active Owner") {
		t.Fatalf("last Owner disable status=%d body=%s", lastOwnerResponse.StatusCode, body)
	}
}

func newCookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func loginNormalAdmin(t *testing.T, client *http.Client, baseURL, email, password string) string {
	t.Helper()
	response, err := client.PostForm(baseURL+"/admin/login", url.Values{
		"email":    {email},
		"password": {password},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d body=%s", response.StatusCode, body)
	}
	match := regexp.MustCompile(`name="csrf-token" content="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("admin response does not contain CSRF token")
	}
	return match[1]
}

func postAdminJSON(t *testing.T, client *http.Client, endpoint, csrf, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponseJSON(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
