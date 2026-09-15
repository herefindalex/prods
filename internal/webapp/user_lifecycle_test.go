package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestUserRoleGrantAndReactivationFlow(t *testing.T) {
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
	ownerCSRF := loginNormalAdmin(t, ownerClient, server.URL, owner.Email, "ownerpass1")

	roleResponse := postAdminJSON(t, ownerClient, server.URL+"/admin/api/roles", ownerCSRF,
		`{"name":"Catalog Viewer","capabilities":["admin.access","catalog.view"]}`)
	if roleResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create role status=%d body=%s", roleResponse.StatusCode, responseBody(t, roleResponse))
	}
	var viewerRole identity.Role
	decodeResponseJSON(t, roleResponse, &viewerRole)

	userResponse := postAdminJSON(t, ownerClient, server.URL+"/admin/api/users", ownerCSRF,
		`{"email":"viewer@example.test","display_name":"Viewer","role_id":"`+viewerRole.ID+`"}`)
	if userResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create user status=%d body=%s", userResponse.StatusCode, responseBody(t, userResponse))
	}
	var firstGrant setPasswordGrantPayload
	decodeResponseJSON(t, userResponse, &firstGrant)
	firstToken := grantToken(t, firstGrant.SetPasswordURL)

	reissueResponse := postAdminJSON(t, ownerClient,
		server.URL+"/admin/api/users/"+firstGrant.User.ID+"/set-password-grant", ownerCSRF, `{}`)
	if reissueResponse.StatusCode != http.StatusCreated {
		t.Fatalf("reissue grant status=%d body=%s", reissueResponse.StatusCode, responseBody(t, reissueResponse))
	}
	var secondGrant setPasswordGrantPayload
	decodeResponseJSON(t, reissueResponse, &secondGrant)
	secondToken := grantToken(t, secondGrant.SetPasswordURL)
	if firstToken == secondToken {
		t.Fatal("reissued grant reused the previous bearer token")
	}

	staleGrant := setPasswordRequest(t, ownerClient, server.URL, firstToken, "viewerpass1")
	if staleGrant.StatusCode != http.StatusConflict {
		t.Fatalf("stale grant status=%d body=%s", staleGrant.StatusCode, responseBody(t, staleGrant))
	}
	_ = responseBody(t, staleGrant)
	setPassword := setPasswordRequest(t, ownerClient, server.URL, secondToken, "viewerpass1")
	if setPassword.StatusCode != http.StatusOK {
		t.Fatalf("set password status=%d body=%s", setPassword.StatusCode, responseBody(t, setPassword))
	}
	_ = responseBody(t, setPassword)

	viewerClient := newCookieClient(t)
	_ = loginNormalAdmin(t, viewerClient, server.URL, secondGrant.User.Email, "viewerpass1")
	roleChange := putAdminJSON(t, ownerClient, server.URL+"/admin/api/users/"+secondGrant.User.ID+"/role", ownerCSRF,
		`{"role_id":"`+identity.RoleOwner+`"}`)
	if roleChange.StatusCode != http.StatusOK {
		t.Fatalf("promote user status=%d body=%s", roleChange.StatusCode, responseBody(t, roleChange))
	}
	var promoted identity.User
	decodeResponseJSON(t, roleChange, &promoted)
	if promoted.Role != identity.RoleOwner {
		t.Fatalf("promoted role=%q", promoted.Role)
	}
	denied, err := viewerClient.Get(server.URL + "/admin/api/products")
	if err != nil {
		t.Fatal(err)
	}
	if denied.StatusCode != http.StatusUnauthorized {
		t.Fatalf("role change did not revoke old session: status=%d body=%s", denied.StatusCode, responseBody(t, denied))
	}
	_ = responseBody(t, denied)

	disable := postAdminJSON(t, ownerClient, server.URL+"/admin/api/users/"+promoted.ID+"/disable", ownerCSRF, `{}`)
	if disable.StatusCode != http.StatusNoContent {
		t.Fatalf("disable promoted user status=%d body=%s", disable.StatusCode, responseBody(t, disable))
	}
	disable.Body.Close()

	reactivate := postAdminJSON(t, ownerClient, server.URL+"/admin/api/users/"+promoted.ID+"/reactivate", ownerCSRF, `{}`)
	if reactivate.StatusCode != http.StatusOK {
		t.Fatalf("reactivate status=%d body=%s", reactivate.StatusCode, responseBody(t, reactivate))
	}
	var reactivationGrant setPasswordGrantPayload
	decodeResponseJSON(t, reactivate, &reactivationGrant)
	if reactivationGrant.User.Status != identity.UserActive {
		t.Fatalf("reactivated status=%q", reactivationGrant.User.Status)
	}

	oldPasswordClient := newCookieClient(t)
	oldLogin, err := oldPasswordClient.PostForm(server.URL+"/admin/login", url.Values{
		"email":    {reactivationGrant.User.Email},
		"password": {"viewerpass1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if oldLogin.StatusCode == http.StatusOK {
		t.Fatalf("old password became usable after reactivation: body=%s", responseBody(t, oldLogin))
	}
	_ = responseBody(t, oldLogin)

	newPassword := setPasswordRequest(t, ownerClient, server.URL, grantToken(t, reactivationGrant.SetPasswordURL), "viewerpass2")
	if newPassword.StatusCode != http.StatusOK {
		t.Fatalf("reactivation password status=%d body=%s", newPassword.StatusCode, responseBody(t, newPassword))
	}
	_ = responseBody(t, newPassword)

	secondOwnerClient := newCookieClient(t)
	secondOwnerCSRF := loginNormalAdmin(t, secondOwnerClient, server.URL, reactivationGrant.User.Email, "viewerpass2")
	demoteOriginal := putAdminJSON(t, secondOwnerClient, server.URL+"/admin/api/users/"+owner.ID+"/role", secondOwnerCSRF,
		`{"role_id":"`+viewerRole.ID+`"}`)
	if demoteOriginal.StatusCode != http.StatusOK {
		t.Fatalf("demote original owner status=%d body=%s", demoteOriginal.StatusCode, responseBody(t, demoteOriginal))
	}
	_ = responseBody(t, demoteOriginal)

	demoteLast := putAdminJSON(t, secondOwnerClient, server.URL+"/admin/api/users/"+promoted.ID+"/role", secondOwnerCSRF,
		`{"role_id":"`+viewerRole.ID+`"}`)
	if demoteLast.StatusCode != http.StatusConflict {
		t.Fatalf("demote last owner status=%d body=%s", demoteLast.StatusCode, responseBody(t, demoteLast))
	}
	_ = responseBody(t, demoteLast)
}

type setPasswordGrantPayload struct {
	User           identity.User `json:"user"`
	SetPasswordURL string        `json:"set_password_url"`
	ExpiresAt      string        `json:"expires_at"`
}

func grantToken(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	token := parsed.Query().Get("token")
	if token == "" {
		t.Fatal("set-password URL does not contain a token")
	}
	return token
}

func setPasswordRequest(t *testing.T, client *http.Client, baseURL, token, password string) *http.Response {
	t.Helper()
	response, err := client.PostForm(baseURL+"/set-password", url.Values{
		"token":                 {token},
		"password":              {password},
		"password_confirmation": {password},
	})
	if err != nil {
		t.Fatal(err)
	}
	return response
}
