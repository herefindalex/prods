package webapp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestUnsavedProductPreviewIsPrivateVersionCheckedAndNeverPublished(t *testing.T) {
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
		DefaultLocale: "zh-TW", SupportedLocales: []string{"zh-TW"}, TimeZone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), WorkDir: filepath.Join(root, "work")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	created := postAdminJSON(t, client, server.URL+"/admin/api/previews/products", csrf,
		`{"product":{"part_number":"UNSAVED-PREVIEW","name":"Never persisted","description":"Private draft text","status":"hidden"}}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("preview create status=%d body=%s", created.StatusCode, responseBody(t, created))
	}
	var receipt struct {
		URL string `json:"url"`
	}
	decodeResponseJSON(t, created, &receipt)
	if !strings.HasPrefix(receipt.URL, "/admin/previews/") || len(receipt.URL) < 50 {
		t.Fatalf("preview URL is not opaque: %q", receipt.URL)
	}
	publicAttempt, err := http.Get(server.URL + receipt.URL)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, publicAttempt); publicAttempt.StatusCode != http.StatusUnauthorized || strings.Contains(body, "UNSAVED-PREVIEW") {
		t.Fatalf("anonymous preview status=%d body=%s", publicAttempt.StatusCode, body)
	}
	preview, err := client.Get(server.URL + receipt.URL)
	if err != nil {
		t.Fatal(err)
	}
	previewBody := responseBody(t, preview)
	if preview.StatusCode != http.StatusOK || !strings.Contains(previewBody, "UNSAVED-PREVIEW") || !strings.Contains(previewBody, "Private draft text") || !strings.Contains(previewBody, `lang="zh-TW"`) {
		t.Fatalf("preview status=%d body=%s", preview.StatusCode, previewBody)
	}
	if preview.Header.Get("Cache-Control") != "private, no-store" || !strings.Contains(preview.Header.Get("X-Robots-Tag"), "noindex") || strings.Contains(previewBody, "Request this product") || strings.Contains(previewBody, `rel="canonical"`) {
		t.Fatalf("preview safety headers/body cache=%q robots=%q body=%s", preview.Header.Get("Cache-Control"), preview.Header.Get("X-Robots-Tag"), previewBody)
	}
	searchBody := publicSearchBody(t, client, server.URL, "UNSAVED-PREVIEW")
	if !strings.Contains(searchBody, "找不到產品") || strings.Contains(searchBody, "/products/preview") {
		t.Fatalf("unsaved preview entered public search: %s", searchBody)
	}

	product := createPublishedProduct(t, client, server.URL, csrf, "PREVIEW-LIVE")
	stale := postAdminJSON(t, client, server.URL+"/admin/api/previews/products", csrf,
		`{"expected_revision":99,"product":{"id":"`+product.ID+`","part_number":"PREVIEW-STALE","slug":"preview-stale","status":"published","revision":99}}`)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale preview status=%d body=%s", stale.StatusCode, responseBody(t, stale))
	}
	_ = responseBody(t, stale)
}
