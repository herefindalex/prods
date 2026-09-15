package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestPublicInterfaceLocalesQueryAndUntranslatedCanonicalMatrix(t *testing.T) {
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
	if _, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US", "zh-TW"}, TimeZone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"),
		AssetDir: filepath.Join(root, "assets"), WorkDir: filepath.Join(root, "work"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	product := createPublishedProduct(t, client, server.URL, csrf, "I18N-1")
	productPath := "/products/" + product.Slug

	englishResponse, englishBody := getEventually(t, client, server.URL+productPath, "Part number")
	englishETag := englishResponse.Header.Get("ETag")
	if englishResponse.Header.Get("Content-Language") != "en-US" || !strings.Contains(englishBody, `lang="en-US"`) {
		t.Fatalf("English representation language=%q body=%s", englishResponse.Header.Get("Content-Language"), englishBody)
	}
	chineseResponse, chineseBody := getEventually(t, client, server.URL+productPath+"?lang=zh-TW", "料號")
	if chineseResponse.Header.Get("Content-Language") != "zh-TW" || chineseResponse.Header.Get("ETag") == englishETag || !strings.Contains(chineseBody, "提出詢價") {
		t.Fatalf("Chinese representation language=%q etag=%q body=%s", chineseResponse.Header.Get("Content-Language"), chineseResponse.Header.Get("ETag"), chineseBody)
	}
	canonical := `<link rel="canonical" href="http://catalog.example.test` + productPath + `">`
	if !strings.Contains(chineseBody, canonical) || !strings.Contains(chineseBody, `rel="alternate" hreflang="x-default"`) || strings.Contains(chineseBody, `rel="alternate" hreflang="zh-TW"`) {
		t.Fatalf("UI-only canonical/hreflang matrix body=%s", chineseBody)
	}
	if !strings.Contains(chineseBody, "/rfq?lang=zh-TW&amp;product_id=") || !strings.Contains(chineseBody, "/search?lang=zh-TW") {
		t.Fatalf("localized product links did not preserve language: %s", chineseBody)
	}

	unsupported, err := client.Get(server.URL + productPath + "?lang=ja-JP")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, unsupported); unsupported.StatusCode != http.StatusBadRequest || !strings.Contains(body, "unsupported language") {
		t.Fatalf("unsupported product locale status=%d body=%s", unsupported.StatusCode, body)
	}
	noResults, err := client.Get(server.URL + "/search?q=NOT-HERE&lang=zh-TW")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, noResults); noResults.StatusCode != http.StatusOK || !strings.Contains(body, "找不到產品") || !strings.Contains(body, "lang=zh-TW") || !strings.Contains(body, "query=NOT-HERE") {
		t.Fatalf("localized no-results status=%d body=%s", noResults.StatusCode, body)
	}

	localesRequest, err := http.NewRequest(http.MethodGet, server.URL+"/api/locales", nil)
	if err != nil {
		t.Fatal(err)
	}
	localesRequest.Header.Set("Accept-Language", "zh-TW,zh;q=0.9,en;q=0.8")
	localesResponse, err := client.Do(localesRequest)
	if err != nil {
		t.Fatal(err)
	}
	var locales struct {
		Default   string   `json:"default"`
		Supported []string `json:"supported"`
		Current   string   `json:"current"`
	}
	if err := json.NewDecoder(localesResponse.Body).Decode(&locales); err != nil {
		t.Fatal(err)
	}
	_ = localesResponse.Body.Close()
	if localesResponse.StatusCode != http.StatusOK || locales.Default != "en-US" || locales.Current != "zh-TW" || len(locales.Supported) != 2 {
		t.Fatalf("locale query response = %+v status=%d", locales, localesResponse.StatusCode)
	}
	badLocales, err := client.Get(server.URL + "/api/locales?lang=ja-JP")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, badLocales); badLocales.StatusCode != http.StatusBadRequest || !strings.Contains(body, "unsupported locale") {
		t.Fatalf("unsupported locale API status=%d body=%s", badLocales.StatusCode, body)
	}
}
