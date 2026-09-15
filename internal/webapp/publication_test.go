package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestNormalPublicationUnitUpdateAndSynchronousRevocation(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), WorkDir: filepath.Join(root, "work"),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	createdResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"PUB-REV-A","name":"Publication A","description":"revision A","status":"published"}`)
	if createdResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdResponse.StatusCode, responseBody(t, createdResponse))
	}
	var product catalog.Product
	decodeResponseJSON(t, createdResponse, &product)
	productURL := server.URL + "/products/" + product.Slug
	waitForPublicBody(t, client, productURL, "revision A")

	htmlResponse, err := client.Get(productURL)
	if err != nil {
		t.Fatal(err)
	}
	etagA := htmlResponse.Header.Get("ETag")
	htmlA := responseBody(t, htmlResponse)
	if etagA == "" || !strings.Contains(htmlA, `data-public-revision="1"`) || !strings.Contains(htmlA, "application/ld+json") {
		t.Fatalf("revision A HTML etag=%q body=%s", etagA, htmlA)
	}
	if !strings.Contains(htmlA, `type="module" src="/static/public/public-islands.js"`) {
		t.Fatalf("revision A HTML does not reference the embedded island bundle: %s", htmlA)
	}
	islandBundle, err := client.Get(server.URL + "/static/public/public-islands.js")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, islandBundle); islandBundle.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("public island bundle status=%d bytes=%d", islandBundle.StatusCode, len(body))
	}
	assertPublicRepresentation(t, client, productURL+".json", []string{`"revision": 1`, "PUB-REV-A", product.ID})
	assertPublicRepresentation(t, client, productURL+".md", []string{"Public revision: `1`", "PUB-REV-A", product.ID})

	product.PartNumber = "PUB-REV-B"
	product.Name = "Publication B"
	product.Description = "revision B"
	updateBody, err := json.Marshal(map[string]any{"expected_revision": product.Revision, "product": product})
	if err != nil {
		t.Fatal(err)
	}
	updateRequest, err := http.NewRequest(http.MethodPut, server.URL+"/admin/api/products/"+product.ID, bytes.NewReader(updateBody))
	if err != nil {
		t.Fatal(err)
	}
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("X-CSRF-Token", csrf)
	updatedResponse, err := client.Do(updateRequest)
	if err != nil {
		t.Fatal(err)
	}
	if updatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updatedResponse.StatusCode, responseBody(t, updatedResponse))
	}
	decodeResponseJSON(t, updatedResponse, &product)
	waitForPublicBody(t, client, productURL, "revision B")
	assertPublicRepresentation(t, client, productURL+".json", []string{`"revision": 2`, "PUB-REV-B", "revision B"})
	assertPublicRepresentation(t, client, productURL+".md", []string{"Public revision: `2`", "PUB-REV-B", "revision B"})
	revisionBResponse, err := client.Get(productURL)
	if err != nil {
		t.Fatal(err)
	}
	etagB := revisionBResponse.Header.Get("ETag")
	_ = responseBody(t, revisionBResponse)
	staleIfRange := mustRequest(t, http.MethodGet, productURL, map[string]string{"Range": "bytes=0-20", "If-Range": etagA})
	staleIfRangeResponse, err := client.Do(staleIfRange)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, staleIfRangeResponse); staleIfRangeResponse.StatusCode != http.StatusOK || !strings.Contains(body, "revision B") {
		t.Fatalf("stale If-Range status=%d body=%s", staleIfRangeResponse.StatusCode, body)
	}
	currentRange := mustRequest(t, http.MethodGet, productURL, map[string]string{"Range": "bytes=0-20", "If-Range": etagB})
	currentRangeResponse, err := client.Do(currentRange)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, currentRangeResponse); currentRangeResponse.StatusCode != http.StatusPartialContent || len(body) != 21 {
		t.Fatalf("current Range status=%d len=%d", currentRangeResponse.StatusCode, len(body))
	}

	hideResponse := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/hide", csrf,
		`{"expected_revision":2}`)
	if body := responseBody(t, hideResponse); hideResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("hide status=%d body=%s", hideResponse.StatusCode, body)
	}
	for _, request := range []*http.Request{
		mustRequest(t, http.MethodGet, productURL, nil),
		mustRequest(t, http.MethodGet, productURL, map[string]string{"If-None-Match": etagA}),
		mustRequest(t, http.MethodGet, productURL, map[string]string{"Range": "bytes=0-20"}),
	} {
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusPartialContent || response.StatusCode == http.StatusNotModified || strings.Contains(body, "PUB-REV") {
			t.Fatalf("revoked request admitted status=%d body=%s", response.StatusCode, body)
		}
	}
	assertPublicAbsent(t, client, server.URL+"/search?q=PUB-REV-B", "/products/"+product.Slug)
	assertPublicAbsent(t, client, server.URL+"/sitemap.xml", product.ID)

	product.Status = catalog.Published
	product.Revision = 3
	product.PartNumber = "PUB-REV-C"
	republishBody, _ := json.Marshal(map[string]any{"expected_revision": int64(3), "product": product})
	republish := mustRequest(t, http.MethodPut, server.URL+"/admin/api/products/"+product.ID, nil)
	republish.Body = io.NopCloser(bytes.NewReader(republishBody))
	republish.Header.Set("Content-Type", "application/json")
	republish.Header.Set("X-CSRF-Token", csrf)
	republishResponse, err := client.Do(republish)
	if err != nil {
		t.Fatal(err)
	}
	if republishResponse.StatusCode != http.StatusOK {
		t.Fatalf("republish status=%d body=%s", republishResponse.StatusCode, responseBody(t, republishResponse))
	}
	decodeResponseJSON(t, republishResponse, &product)
	waitForPublicBody(t, client, productURL, "PUB-REV-C")

	server.Close()
	app.Close()
	app, _, err = New(store, Config{
		BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), WorkDir: filepath.Join(root, "work-2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := httptest.NewServer(app)
	restartResponse, err := http.Get(restarted.URL + "/products/" + product.Slug)
	if err != nil {
		t.Fatal(err)
	}
	etagC := restartResponse.Header.Get("ETag")
	if body := responseBody(t, restartResponse); restartResponse.StatusCode != http.StatusOK || !strings.Contains(body, "PUB-REV-C") {
		t.Fatalf("restart reconciliation status=%d body=%s", restartResponse.StatusCode, body)
	}
	adminProductResponse, err := client.Get(restarted.URL + "/admin/api/products/" + product.ID)
	if err != nil {
		t.Fatal(err)
	}
	decodeResponseJSON(t, adminProductResponse, &product)
	archiveResponse := postAdminJSON(t, client, restarted.URL+"/admin/api/products/"+product.ID+"/archive", csrf,
		`{"expected_revision":`+fmt.Sprint(product.Revision)+`}`)
	if body := responseBody(t, archiveResponse); archiveResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("archive status=%d body=%s", archiveResponse.StatusCode, body)
	}
	staleAfterArchive := mustRequest(t, http.MethodGet, restarted.URL+"/products/"+product.Slug, map[string]string{"If-None-Match": etagC})
	archivedPublic, err := client.Do(staleAfterArchive)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, archivedPublic); archivedPublic.StatusCode == http.StatusOK || archivedPublic.StatusCode == http.StatusNotModified || strings.Contains(body, "PUB-REV-C") {
		t.Fatalf("archive admitted stale public response status=%d body=%s", archivedPublic.StatusCode, body)
	}
	restarted.Close()
	app.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPublicationRestartFailsClosedAndRepairsCorruptActiveUnit(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail:       "owner@example.test",
		OwnerDisplayName: "Owner",
		PasswordHash:     passwordHash,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US"},
		TimeZone:         "UTC",
	}); err != nil {
		t.Fatal(err)
	}

	publicRoot := filepath.Join(root, "generated")
	app, _, err := New(store, Config{
		BaseURL:   "http://catalog.example.test",
		PublicDir: publicRoot,
		WorkDir:   filepath.Join(root, "work"),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf, `{"part_number":"REPAIR-1","name":"Repair me","status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.StatusCode, responseBody(t, created))
	}
	var product catalog.Product
	decodeResponseJSON(t, created, &product)
	waitForPublicBody(t, client, server.URL+"/products/"+product.Slug, "REPAIR-1")
	server.Close()
	app.Close()

	units, err := os.ReadDir(filepath.Join(publicRoot, "units"))
	if err != nil || len(units) != 1 {
		t.Fatalf("initial units=%d err=%v", len(units), err)
	}
	if err := os.WriteFile(filepath.Join(publicRoot, "units", units[0].Name(), "index.html"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}

	app, _, err = New(store, Config{
		BaseURL:   "http://catalog.example.test",
		PublicDir: publicRoot,
		WorkDir:   filepath.Join(root, "work-2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	restarted := httptest.NewServer(app)
	defer restarted.Close()
	waitForPublicBody(t, restarted.Client(), restarted.URL+"/products/"+product.Slug, "REPAIR-1")

	units, err = os.ReadDir(filepath.Join(publicRoot, "units"))
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("corrupt unit was not replaced; units=%d", len(units))
	}
}

func TestDisabledContentLanguageCannotReplayTranslatedETagOrRange(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteInstallation(t.Context(), sqlite.Installation{OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US", SupportedLocales: []string{"en-US", "zh-TW"}, TimeZone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), WorkDir: filepath.Join(root, "work")})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	defer store.Close()
	server := httptest.NewServer(app)
	defer server.Close()
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	enable := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/system/settings/content-localization", csrf, `{"expected_revision":1,"enabled":true,"supported_locales":["en-US","zh-TW"]}`)
	if body := responseBody(t, enable); enable.StatusCode != http.StatusOK {
		t.Fatalf("enable=%d %s", enable.StatusCode, body)
	}
	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf, `{"part_number":"ML-PUB-1","name":"Source name","description":"Source description","status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create=%d %s", created.StatusCode, responseBody(t, created))
	}
	var product catalog.Product
	decodeResponseJSON(t, created, &product)
	saved := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/products/"+product.ID+"/translations/zh-TW", csrf, fmt.Sprintf(`{"expected_revision":%d,"translation":{"name":"翻譯名稱","description":"翻譯說明","features":"localizedneedle"}}`, product.Revision))
	if body := responseBody(t, saved); saved.StatusCode != http.StatusOK {
		t.Fatalf("translation=%d %s", saved.StatusCode, body)
	}
	baseProductURL := server.URL + "/products/" + product.Slug
	localizedURLs := []string{baseProductURL + "?lang=zh-TW", baseProductURL + ".json?lang=zh-TW", baseProductURL + ".md?lang=zh-TW"}
	etags := make(map[string]string, len(localizedURLs))
	for _, productURL := range localizedURLs {
		waitForPublicBody(t, client, productURL, "翻譯名稱")
		translated, err := client.Get(productURL)
		if err != nil {
			t.Fatal(err)
		}
		etags[productURL] = translated.Header.Get("ETag")
		_ = responseBody(t, translated)
		if etags[productURL] == "" {
			t.Fatalf("missing translated ETag for %s", productURL)
		}
	}
	waitForPublicBody(t, client, server.URL+"/search?q=localizedneedle&lang=zh-TW", "翻譯名稱")
	disable := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/system/settings/content-localization", csrf, `{"expected_revision":2,"enabled":false,"supported_locales":["en-US","zh-TW"]}`)
	if body := responseBody(t, disable); disable.StatusCode != http.StatusOK {
		t.Fatalf("disable=%d %s", disable.StatusCode, body)
	}
	disabledSearch, err := client.Get(server.URL + "/search?q=localizedneedle&lang=zh-TW")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, disabledSearch); strings.Contains(body, "翻譯名稱") || strings.Contains(body, "/products/"+product.Slug) {
		t.Fatalf("disabled search residue status=%d body=%s", disabledSearch.StatusCode, body)
	}
	for _, productURL := range localizedURLs {
		for _, headers := range []map[string]string{{"If-None-Match": etags[productURL]}, {"Range": "bytes=0-80", "If-Range": etags[productURL]}} {
			request := mustRequest(t, http.MethodGet, productURL, headers)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body := responseBody(t, response)
			if response.StatusCode == http.StatusNotModified || strings.Contains(body, "翻譯名稱") || strings.Contains(body, "翻譯說明") {
				t.Fatalf("disabled translation status=%d body=%s", response.StatusCode, body)
			}
			if !strings.Contains(body, "Source name") && !strings.Contains(body, "Source description") {
				t.Fatalf("missing source fallback status=%d body=%s", response.StatusCode, body)
			}
		}
	}
	removeLocale := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/system/settings/content-localization", csrf, `{"expected_revision":3,"enabled":true,"supported_locales":["en-US"]}`)
	if body := responseBody(t, removeLocale); removeLocale.StatusCode != http.StatusOK {
		t.Fatalf("remove locale=%d %s", removeLocale.StatusCode, body)
	}
	stale := mustRequest(t, http.MethodGet, localizedURLs[0], map[string]string{"If-None-Match": etags[localizedURLs[0]], "Range": "bytes=0-80"})
	response, err := client.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != http.StatusBadRequest || strings.Contains(body, "翻譯") {
		t.Fatalf("removed locale status=%d body=%s", response.StatusCode, body)
	}
}

func adminJSONMethod(t *testing.T, client *http.Client, method, endpoint, csrf, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, bytes.NewBufferString(body))
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

func waitForPublicBody(t *testing.T, client *http.Client, endpoint, expected string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err := client.Get(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode == http.StatusOK && strings.Contains(body, expected) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("public endpoint did not converge: status=%d body=%s", response.StatusCode, body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertPublicRepresentation(t *testing.T, client *http.Client, endpoint string, expected []string) {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("representation status=%d body=%s", response.StatusCode, body)
	}
	for _, value := range expected {
		if !strings.Contains(body, value) {
			t.Fatalf("representation %s missing %q: %s", endpoint, value, body)
		}
	}
}

func assertPublicAbsent(t *testing.T, client *http.Client, endpoint, forbidden string) {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if strings.Contains(body, forbidden) {
		t.Fatalf("%s leaked %q: %s", endpoint, forbidden, body)
	}
}

func mustRequest(t *testing.T, method, endpoint string, headers map[string]string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request
}
