package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/distribution"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func testServer(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	server, client, _ := testServerWithStore(t)
	return server, client
}

func testServerWithStore(t *testing.T) (*httptest.Server, *http.Client, *sqlite.Store) {
	t.Helper()
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	app, generated, err := New(store, Config{BaseURL: "https://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	if generated != "" {
		t.Fatal("configured admin token should not be replaced")
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return server, &http.Client{Jar: jar}, store
}

func responseBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestWriteJSONUsesEmptyArrayForNilSlice(t *testing.T) {
	response := httptest.NewRecorder()
	var values []string
	writeJSON(response, http.StatusOK, values)
	if body := strings.TrimSpace(response.Body.String()); body != "[]" {
		t.Fatalf("nil list JSON = %s", body)
	}
}

func loginAdmin(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	response, err := client.PostForm(baseURL+"/admin/login", url.Values{"token": {"test-admin-token"}})
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin login status=%d body=%s", response.StatusCode, body)
	}
	match := regexp.MustCompile(`name="csrf-token" content="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("admin CSRF missing from %s", body)
	}
	return match[1]
}

func TestAdminDocumentsSetSecurityHeaders(t *testing.T) {
	server, client := testServer(t)
	loginAdmin(t, client, server.URL)

	response, err := client.Get(server.URL + "/admin/catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin document status=%d", response.StatusCode)
	}
	policy := response.Header.Get("Content-Security-Policy")
	for _, required := range []string{"default-src 'none'", "script-src 'self'", "connect-src 'self'", "form-action 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
		if !strings.Contains(policy, required) {
			t.Fatalf("admin CSP missing %q: %s", required, policy)
		}
	}
	if response.Header.Get("Referrer-Policy") != "same-origin" ||
		response.Header.Get("X-Content-Type-Options") != "nosniff" ||
		response.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("admin security headers incomplete: %v", response.Header)
	}
}

func TestSystemUpdateIsAuthenticatedAndUnavailableDoesNotFailHealth(t *testing.T) {
	server, client := testServer(t)
	response, err := client.Get(server.URL + "/admin/api/system/update")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous update status=%d", response.StatusCode)
	}
	loginAdmin(t, client, server.URL)
	response, err = client.Get(server.URL + "/admin/api/system/update")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var status struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || status.Status != "unavailable" {
		t.Fatalf("update response status=%d body=%+v", response.StatusCode, status)
	}
}

func TestSystemUpdateReturnsValidatedPlatformDownload(t *testing.T) {
	assetName := "prods-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	var releaseServer *httptest.Server
	releaseServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v2.0.0","html_url":%q,"assets":[{"name":%q,"browser_download_url":%q}]}`,
			releaseServer.URL+"/herefindalex/prods/releases/tag/v2.0.0", assetName, releaseServer.URL+"/download")
	}))
	defer releaseServer.Close()
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		ApplicationVersion: "v1.0.0",
		DistributionClient: &distribution.Client{HTTPClient: releaseServer.Client(), APIBase: releaseServer.URL, AllowTestHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	loginAdmin(t, client, server.URL)
	response, err := client.Get(server.URL + "/admin/api/system/update")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var status distribution.UpdateStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || status.Status != "update_available" ||
		status.LatestVersion != "v2.0.0" || status.DownloadURL != releaseServer.URL+"/download" {
		t.Fatalf("update response status=%d body=%+v", response.StatusCode, status)
	}
}

func TestRuntimeProductUsesOnePublicViewAcrossFormats(t *testing.T) {
	server, client := testServer(t)
	csrf := loginAdmin(t, client, server.URL)
	payload := `{"id":"runtime-product","part_number":"RUNTIME-3V3","name":"Runtime Product","manufacturer":"Example Components","description":"Created after startup","specification":"3.0–3.6V, 4.5–5.5V","document_url":"https://example.com/runtime.pdf","status":"published"}`
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/api/products", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, response); response.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.StatusCode, body)
	}

	formats := map[string][]string{
		"/products/runtime-product":      {"RUNTIME-3V3", "3.0–3.6V, 4.5–5.5V", "runtime.pdf", "application/ld+json", "data-revision=\"1\"", `type="application/json"`, `type="text/markdown"`},
		"/products/runtime-product.json": {`"id": "runtime-product"`, `"revision": 1`, `"part_number": "RUNTIME-3V3"`, `"canonical_url": "https://catalog.example.test/products/runtime-product"`},
		"/products/runtime-product.md":   {"Product ID: `runtime-product`", "Public revision: `1`", "RUNTIME-3V3", "runtime.pdf"},
		"/sitemap.xml":                   {"https://catalog.example.test/products/runtime-product"},
		"/robots.txt":                    {"Sitemap: https://catalog.example.test/sitemap.xml"},
		"/llms.txt":                      {"Catalog manifest", "https://catalog.example.test/catalog/manifest.json"},
		"/catalog/manifest.json":         {`"format_version": "prods.catalog-manifest.v1"`, "/catalog/products-000001.json"},
		"/catalog/products-000001.json":  {`"format_version": "prods.catalog-shard.v1"`, "runtime-product", "RUNTIME-3V3"},
	}
	for path, expected := range formats {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, response.StatusCode, body)
		}
		for _, text := range expected {
			if !strings.Contains(body, text) {
				t.Errorf("%s missing %q", path, text)
			}
		}
	}
}

func TestNoResultsPaginationAndHiddenBoundary(t *testing.T) {
	server, client := testServer(t)
	for _, test := range []struct {
		path    string
		present string
		absent  string
	}{
		{"/search?page=1&page_size=3", "Next page", "SECRET-POC-01"},
		{"/search?page=2&page_size=3", "Previous page", "SECRET-POC-01"},
		{"/search?q=TI+TPS54331DR", "Request this part", "SECRET-POC-01"},
		{"/products/synthetic-hidden.json", "404 page not found", "SECRET-POC-01"},
	} {
		response, err := client.Get(server.URL + test.path)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if !strings.Contains(body, test.present) {
			t.Errorf("%s missing %q: %s", test.path, test.present, body)
		}
		if strings.Contains(body, test.absent) {
			t.Errorf("%s leaked %q", test.path, test.absent)
		}
	}
}

func TestJSONRFQReplaysReceiptAndConflictsOnEdit(t *testing.T) {
	server, client := testServer(t)
	response, err := client.Get(server.URL + "/rfq?query=TI+TPS54331DR")
	if err != nil {
		t.Fatal(err)
	}
	page := responseBody(t, response)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(page)
	keyMatch := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page)
	if len(csrfMatch) != 2 || len(keyMatch) != 2 {
		t.Fatal("RFQ form did not provide separate CSRF and submission keys")
	}
	if csrfMatch[1] == keyMatch[1] {
		t.Fatal("CSRF and idempotency keys must be separate")
	}
	firstJSON := `{"submission_key":"` + keyMatch[1] + `","submission":{"name":"Ada","email":"ada@example.test","items":[{"kind":"requested","requested":"TI TPS54331DR","raw_query":"TI TPS54331DR"}]}}`
	secondJSON := `{"submission":{"items":[{"raw_query":"TI TPS54331DR","requested":"TI TPS54331DR","kind":"requested"}],"email":"ada@example.test","name":"Ada"},"submission_key":"` + keyMatch[1] + `"}`
	first := postRFQJSON(t, client, server.URL, csrfMatch[1], firstJSON)
	second := postRFQJSON(t, client, server.URL, csrfMatch[1], secondJSON)
	if first.RFQID == "" || first.RFQID != second.RFQID || !second.Replay {
		t.Fatalf("receipt replay mismatch: %#v %#v", first, second)
	}
	changed := strings.Replace(secondJSON, `"requested":"TI TPS54331DR"`, `"requested":"TI TPS54331DR changed"`, 1)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/rfqs", strings.NewReader(changed))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrfMatch[1])
	conflict, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, conflict)
	if conflict.StatusCode != http.StatusConflict || !strings.Contains(body, "different payload") {
		t.Fatalf("conflict status=%d body=%s", conflict.StatusCode, body)
	}
}

func TestHTMLRFQCatalogSubmissionUsesSupportedLocale(t *testing.T) {
	server, client := testServer(t)
	response, err := client.Get(server.URL + "/rfq?product_id=synthetic-page-1")
	if err != nil {
		t.Fatal(err)
	}
	page := responseBody(t, response)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(page)
	keyMatch := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page)
	actionMatch := regexp.MustCompile(`<form method="post" action="([^"]+)"`).FindStringSubmatch(page)
	if len(csrfMatch) != 2 || len(keyMatch) != 2 || len(actionMatch) != 2 {
		t.Fatal("RFQ form did not contain CSRF, submission key, and action")
	}

	form := url.Values{
		"csrf_token":     {csrfMatch[1]},
		"submission_key": {keyMatch[1]},
		"kind":           {"catalog"},
		"product_id":     {"synthetic-page-1"},
		"name":           {"Release Smoke"},
		"email":          {"smoke@example.test"},
		"notes":          {"Packaged binary RFQ"},
	}
	action := strings.ReplaceAll(actionMatch[1], "&amp;", "&")
	response, err = client.PostForm(server.URL+action, form)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, "RFQ received") {
		t.Fatalf("RFQ status=%d body=%s", response.StatusCode, body)
	}
	referenceMatch := regexp.MustCompile(`Reference: <strong>([^<]+)</strong>`).FindStringSubmatch(body)
	if len(referenceMatch) != 2 {
		t.Fatalf("RFQ receipt missing reference: %s", body)
	}

	response, err = client.PostForm(server.URL+action, form)
	if err != nil {
		t.Fatal(err)
	}
	body = responseBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, referenceMatch[1]) ||
		!strings.Contains(body, "no duplicate RFQ was created") {
		t.Fatalf("RFQ replay status=%d body=%s", response.StatusCode, body)
	}

	form.Set("notes", "Edited after completed submission")
	response, err = client.PostForm(server.URL+action, form)
	if err != nil {
		t.Fatal(err)
	}
	body = responseBody(t, response)
	if response.StatusCode != http.StatusConflict || !strings.Contains(body, "Your edits were not overwritten") ||
		!strings.Contains(body, "Edited after completed submission") ||
		!strings.Contains(body, `name="kind" value="catalog"`) ||
		!strings.Contains(body, `name="product_id" value="synthetic-page-1"`) {
		t.Fatalf("RFQ conflict status=%d body=%s", response.StatusCode, body)
	}
}

func TestHTMLRFQMultipleCatalogProductsAreDeduplicatedAndPersistedTogether(t *testing.T) {
	server, client, store := testServerWithStore(t)
	csrf := loginAdmin(t, client, server.URL)
	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"id":"rfq-second-product","part_number":"RFQ-SECOND","status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.StatusCode, responseBody(t, created))
	}
	_ = responseBody(t, created)
	waitForPublicBody(t, client, server.URL+"/products/rfq-second-product", "RFQ-SECOND")

	response, err := client.Get(server.URL + "/rfq?product_id=synthetic-page-1&product_id=rfq-second-product&product_id=synthetic-page-1")
	if err != nil {
		t.Fatal(err)
	}
	page := responseBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("RFQ form status=%d body=%s", response.StatusCode, page)
	}
	if strings.Count(page, `name="product_id" value="synthetic-page-1"`) != 1 ||
		strings.Count(page, `name="product_id" value="rfq-second-product"`) != 1 {
		t.Fatalf("RFQ form did not render two deduplicated catalog products: %s", page)
	}
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(page)
	keyMatch := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page)
	actionMatch := regexp.MustCompile(`<form method="post" action="([^"]+)"`).FindStringSubmatch(page)
	if len(csrfMatch) != 2 || len(keyMatch) != 2 || len(actionMatch) != 2 {
		t.Fatal("RFQ form did not contain CSRF, submission key, and action")
	}

	form := url.Values{
		"csrf_token":     {csrfMatch[1]},
		"submission_key": {keyMatch[1]},
		"kind":           {"catalog"},
		"product_id":     {"synthetic-page-1", "rfq-second-product", "synthetic-page-1"},
		"name":           {"Multi Product Buyer"},
		"email":          {"multi@example.test"},
	}
	action := strings.ReplaceAll(actionMatch[1], "&amp;", "&")
	response, err = client.PostForm(server.URL+action, form)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, "RFQ received") {
		t.Fatalf("RFQ submission status=%d body=%s", response.StatusCode, body)
	}

	rfqs, err := store.ListRFQs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rfqs) != 1 || len(rfqs[0].Items) != 2 {
		t.Fatalf("stored RFQs=%#v", rfqs)
	}
	if rfqs[0].Items[0].ProductID != "synthetic-page-1" || rfqs[0].Items[1].ProductID != "rfq-second-product" {
		t.Fatalf("stored RFQ item order=%#v", rfqs[0].Items)
	}
}

func TestHTMLRFQWriteFailurePreservesEnteredContent(t *testing.T) {
	server, client, store := testServerWithStore(t)
	response, err := client.Get(server.URL + "/rfq?query=TI+TPS54331DR")
	if err != nil {
		t.Fatal(err)
	}
	page := responseBody(t, response)
	csrfMatch := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(page)
	keyMatch := regexp.MustCompile(`name="submission_key" value="([^"]+)"`).FindStringSubmatch(page)
	if len(csrfMatch) != 2 || len(keyMatch) != 2 {
		t.Fatal("RFQ form did not provide submission credentials")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	entered := url.Values{
		"csrf_token":     {csrfMatch[1]},
		"submission_key": {keyMatch[1]},
		"kind":           {"requested"},
		"requested":      {"TI TPS54331DR edited"},
		"raw_query":      {"TI TPS54331DR"},
		"name":           {"Ada Lovelace"},
		"email":          {"ada@example.test"},
		"quantity":       {""},
		"notes":          {"Keep this text for retry"},
	}
	failed, err := client.PostForm(server.URL+"/rfq", entered)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, failed)
	if failed.StatusCode != http.StatusBadRequest {
		t.Fatalf("write failure status=%d body=%s", failed.StatusCode, body)
	}
	if contentType := failed.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("write failure content type=%q", contentType)
	}
	for _, want := range []string{"Could not save the RFQ", "TI TPS54331DR edited", "Ada Lovelace", "ada@example.test", "Keep this text for retry"} {
		if !strings.Contains(body, want) {
			t.Errorf("write failure page missing %q", want)
		}
	}
	if strings.Contains(body, "RFQ received") {
		t.Fatal("write failure must not render success")
	}
}

type receipt struct {
	RFQID  string `json:"rfq_id"`
	Replay bool   `json:"replay"`
}

func postRFQJSON(t *testing.T, client *http.Client, baseURL, csrf, body string) receipt {
	t.Helper()
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/rfqs", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded := responseBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("RFQ status=%d body=%s", response.StatusCode, encoded)
	}
	var result receipt
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHideSmokeChecksVisibilityBeforeStaleETag(t *testing.T) {
	server, client := testServer(t)
	response, err := client.Get(server.URL + "/products/synthetic-published")
	if err != nil {
		t.Fatal(err)
	}
	etag := response.Header.Get("ETag")
	_ = responseBody(t, response)
	csrf := loginAdmin(t, client, server.URL)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/api/products/synthetic-published/hide", nil)
	request.Header.Set("X-CSRF-Token", csrf)
	hidden, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = responseBody(t, hidden)
	if hidden.StatusCode != http.StatusNoContent {
		t.Fatalf("hide status=%d", hidden.StatusCode)
	}
	stale, _ := http.NewRequest(http.MethodGet, server.URL+"/products/synthetic-published", nil)
	stale.Header.Set("If-None-Match", etag)
	after, err := client.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, after)
	if after.StatusCode == http.StatusNotModified || strings.Contains(body, "SYNTH-3V3-01") {
		t.Fatalf("stale representation admitted after hide: status=%d body=%s", after.StatusCode, body)
	}
}

func TestConfiguredHostOriginAndSecureCookieBoundary(t *testing.T) {
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", AdminToken: "test-token", EnablePOCAdmin: true,
		SecureCookies: true, EnforceHost: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	badHost := httptest.NewRequest(http.MethodGet, "/search", nil)
	badHost.Host = "evil.example.test"
	badHostResponse := httptest.NewRecorder()
	app.ServeHTTP(badHostResponse, badHost)
	if badHostResponse.Code != http.StatusMisdirectedRequest {
		t.Fatalf("bad Host status=%d", badHostResponse.Code)
	}

	badOrigin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("token=test-token"))
	badOrigin.Host = "catalog.example.test"
	badOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badOrigin.Header.Set("Origin", "https://evil.example.test")
	badOriginResponse := httptest.NewRecorder()
	app.ServeHTTP(badOriginResponse, badOrigin)
	if badOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("bad Origin status=%d", badOriginResponse.Code)
	}
	nullOrigin := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("token=test-token"))
	nullOrigin.Host = "catalog.example.test"
	nullOrigin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	nullOrigin.Header.Set("Origin", "null")
	nullOriginResponse := httptest.NewRecorder()
	app.ServeHTTP(nullOriginResponse, nullOrigin)
	if nullOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("null Origin status=%d", nullOriginResponse.Code)
	}

	login := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("token=test-token"))
	login.Host = "catalog.example.test"
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login.Header.Set("Origin", "https://catalog.example.test")
	loginResponse := httptest.NewRecorder()
	app.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusSeeOther {
		t.Fatalf("valid authority login status=%d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	cookie := cookieNamed(loginResponse.Result(), "prods_admin")
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("Admin cookie = %+v", cookie)
	}
}

func TestLoopbackAliasesShareConfiguredHostAndOriginBoundary(t *testing.T) {
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app, _, err := New(store, Config{
		BaseURL:        "http://localhost:8080",
		AdminToken:     "test-token",
		EnablePOCAdmin: true,
		EnforceHost:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	page := httptest.NewRequest(http.MethodGet, "/admin/login?lang=en-US", nil)
	page.Host = "127.0.0.1:8080"
	pageResponse := httptest.NewRecorder()
	app.ServeHTTP(pageResponse, page)
	if pageResponse.Code != http.StatusOK || pageResponse.Header().Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("loopback login page status=%d referrer-policy=%q", pageResponse.Code, pageResponse.Header().Get("Referrer-Policy"))
	}

	login := httptest.NewRequest(http.MethodPost, "/admin/login?lang=en-US", strings.NewReader("token=test-token"))
	login.Host = "127.0.0.1:8080"
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login.Header.Set("Origin", "http://127.0.0.1:8080")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, login)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin" {
		t.Fatalf("loopback alias login status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
}

func TestNormalModeRedirectsStaleInstallerAndAdminTrailingSlash(t *testing.T) {
	server, client := testServer(t)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for _, test := range []struct {
		path     string
		location string
	}{
		{path: "/install?token=must-not-leak", location: "/"},
		{path: "/install/", location: "/"},
		{path: "/admin/?lang=zh-TW", location: "/admin?lang=zh-TW"},
	} {
		response, err := client.Get(server.URL + test.path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != test.location {
			t.Errorf("%s status=%d location=%q", test.path, response.StatusCode, response.Header.Get("Location"))
		}
	}

	response, err := client.Get(server.URL + "/install/api/state")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("stale installer API status=%d", response.StatusCode)
	}
}

func TestNormalHealthSeparatesLivenessAndReadiness(t *testing.T) {
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", AdminToken: "test-token", EnablePOCAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/live", nil); response.Code != http.StatusOK {
		t.Fatalf("normal liveness status=%d", response.Code)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/ready", nil); response.Code != http.StatusOK {
		t.Fatalf("normal readiness status=%d", response.Code)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/live", nil); response.Code != http.StatusOK {
		t.Fatalf("closed-store liveness status=%d", response.Code)
	}
	if response := requestWithCookies(app, http.MethodGet, "/health/ready", nil); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed-store readiness status=%d", response.Code)
	}
}

func TestNormalCatalogAPIUsesRevisionArchiveAndAuditContracts(t *testing.T) {
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	login, err := client.PostForm(server.URL+"/admin/login", url.Values{
		"email": {"owner@example.test"}, "password": {"ownerpass1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	loginBody := responseBody(t, login)
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.StatusCode, loginBody)
	}
	csrfMatch := regexp.MustCompile(`name="csrf-token" content="([^"]+)"`).FindStringSubmatch(loginBody)
	if len(csrfMatch) != 2 {
		t.Fatal("normal Admin CSRF token missing")
	}
	csrf := csrfMatch[1]

	create, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/api/products", strings.NewReader(`{"part_number":"  M2-ABC  "}`))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("X-CSRF-Token", csrf)
	createdResponse, err := client.Do(create)
	if err != nil {
		t.Fatal(err)
	}
	createdBody := responseBody(t, createdResponse)
	if createdResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdResponse.StatusCode, createdBody)
	}
	var created catalog.Product
	if err := json.Unmarshal([]byte(createdBody), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.PartNumber != "  M2-ABC  " || created.RecordState != catalog.RecordCurrent || created.Status != catalog.Hidden {
		t.Fatalf("created = %+v", created)
	}

	created.Name = "updated"
	updateBody, _ := json.Marshal(map[string]any{"expected_revision": created.Revision, "product": created})
	update, _ := http.NewRequest(http.MethodPut, server.URL+"/admin/api/products/"+created.ID, bytes.NewReader(updateBody))
	update.Header.Set("Content-Type", "application/json")
	update.Header.Set("X-CSRF-Token", csrf)
	updatedResponse, err := client.Do(update)
	if err != nil {
		t.Fatal(err)
	}
	var updated catalog.Product
	if body := responseBody(t, updatedResponse); updatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updatedResponse.StatusCode, body)
	} else if err := json.Unmarshal([]byte(body), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Name != "updated" {
		t.Fatalf("updated = %+v", updated)
	}

	stale, _ := http.NewRequest(http.MethodPut, server.URL+"/admin/api/products/"+created.ID, bytes.NewReader(updateBody))
	stale.Header.Set("Content-Type", "application/json")
	stale.Header.Set("X-CSRF-Token", csrf)
	staleResponse, err := client.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, staleResponse); staleResponse.StatusCode != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", staleResponse.StatusCode, body)
	}

	archiveBody := strings.NewReader(`{"expected_revision":2}`)
	archive, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/api/products/"+created.ID+"/archive", archiveBody)
	archive.Header.Set("Content-Type", "application/json")
	archive.Header.Set("X-CSRF-Token", csrf)
	archiveResponse, err := client.Do(archive)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, archiveResponse); archiveResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("archive status=%d body=%s", archiveResponse.StatusCode, body)
	}

	auditResponse, err := client.Get(server.URL + "/admin/api/audit")
	if err != nil {
		t.Fatal(err)
	}
	auditBody := responseBody(t, auditResponse)
	if auditResponse.StatusCode != http.StatusOK || !strings.Contains(auditBody, "product.created") ||
		!strings.Contains(auditBody, "product.updated") || !strings.Contains(auditBody, "product.archived") {
		t.Fatalf("audit status=%d body=%s", auditResponse.StatusCode, auditBody)
	}
}
