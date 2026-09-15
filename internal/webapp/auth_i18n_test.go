package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/identity"
	storesqlite "prods/internal/storage/sqlite"
)

func TestLoginAndSetPasswordRespectConfiguredInterfaceLanguage(t *testing.T) {
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
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteInstallation(t.Context(), storesqlite.Installation{
		OwnerEmail:       "owner@example.test",
		OwnerDisplayName: "Owner",
		PasswordHash:     passwordHash,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US", "zh-TW"},
		TimeZone:         "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		PublicDir: filepath.Join(root, "generated", "public"),
		AssetDir:  filepath.Join(dataRoot, "assets"), WorkDir: filepath.Join(root, "work"),
		BaseURL: "https://example.test", SecureCookies: true, EnforceHost: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	login := authPageRequest(app, http.MethodGet, "/admin/login?lang=zh-TW", nil, "")
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), "Prods 管理後台") ||
		!strings.Contains(login.Body.String(), "data-email-label=\"電子郵件\"") ||
		!strings.Contains(login.Body.String(), "id=\"admin-login-root\"") ||
		!strings.Contains(login.Body.String(), "/static/admin/admin.js") ||
		!strings.Contains(login.Body.String(), "/admin/login?lang=zh-TW") {
		t.Fatalf("localized login status=%d body=%s", login.Code, login.Body.String())
	}
	negotiated := authPageRequest(app, http.MethodGet, "/admin/login", nil, "zh-TW, en;q=0.8")
	if negotiated.Code != http.StatusOK || !strings.Contains(negotiated.Body.String(), "Prods 管理後台") {
		t.Fatalf("negotiated login status=%d body=%s", negotiated.Code, negotiated.Body.String())
	}

	badLogin := authPageRequest(app, http.MethodPost, "/admin/login?lang=zh-TW", url.Values{
		"email": {"owner@example.test"}, "password": {"wrongpass1"},
	}, "")
	if badLogin.Code != http.StatusUnauthorized || !strings.Contains(badLogin.Body.String(), "電子郵件或密碼不正確") {
		t.Fatalf("localized login error status=%d body=%s", badLogin.Code, badLogin.Body.String())
	}

	setPassword := authPageRequest(app, http.MethodGet, "/set-password?token=invite-token&lang=zh-TW", nil, "")
	if setPassword.Code != http.StatusOK || !strings.Contains(setPassword.Body.String(), "設定 Prods 密碼") ||
		!strings.Contains(setPassword.Body.String(), "/set-password?lang=zh-TW") {
		t.Fatalf("localized set-password status=%d body=%s", setPassword.Code, setPassword.Body.String())
	}
	mismatch := authPageRequest(app, http.MethodPost, "/set-password?lang=zh-TW", url.Values{
		"token": {"invite-token"}, "password": {"ownerpass1"}, "password_confirmation": {"different1"},
	}, "")
	if mismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(mismatch.Body.String(), "兩次輸入的密碼不一致") {
		t.Fatalf("localized set-password error status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
}

func authPageRequest(handler http.Handler, method, target string, form url.Values, acceptLanguage string) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form == nil {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(form.Encode())
	}
	request := httptest.NewRequest(method, "https://example.test"+target, body)
	request.Host = "example.test"
	request.Header.Set("Origin", "https://example.test")
	request.Header.Set("Accept-Language", acceptLanguage)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
