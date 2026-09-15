package webapp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func TestSiteRoutePublishSwitchesEveryProductAtOneEpochAndRejectsInvalidCandidate(t *testing.T) {
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
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	config := Config{BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), AssetDir: filepath.Join(root, "assets"), WorkDir: filepath.Join(root, "work")}
	app, _, err := New(store, config)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	makerA := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"manufacturer","name":"Maker A","slug":"maker-a","status":"active","revision":1}`)
	sharedBrand := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"brand","name":"Shared Brand","slug":"shared-brand","status":"active","revision":1}`)
	firstResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"SWITCH-A","slug":"switch-item","manufacturer_id":"`+makerA.ID+`","brand_id":"`+sharedBrand.ID+`","status":"published"}`)
	if firstResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create first status=%d body=%s", firstResponse.StatusCode, responseBody(t, firstResponse))
	}
	var first catalog.Product
	decodeResponseJSON(t, firstResponse, &first)
	oldURL := server.URL + "/products/switch-item"
	waitForPublicBody(t, client, oldURL, "SWITCH-A")
	oldResponse, err := client.Get(oldURL)
	if err != nil {
		t.Fatal(err)
	}
	oldETag := oldResponse.Header.Get("ETag")
	_ = responseBody(t, oldResponse)

	previewResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/preview", csrf,
		`{"product_prefix":"/catalog","url_pattern":"manufacturer"}`)
	if previewResponse.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.StatusCode, responseBody(t, previewResponse))
	}
	var preview publishing.SiteRoutePreview
	decodeResponseJSON(t, previewResponse, &preview)
	if preview.CurrentEpoch != 1 || preview.CandidateEpoch != 2 || preview.Affected != 1 || len(preview.Missing) != 0 || len(preview.Conflicts) != 0 {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	publishResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":1,"expected_working_revision":1,"config":{"product_prefix":"/catalog","url_pattern":"manufacturer"}}`)
	if publishResponse.StatusCode != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", publishResponse.StatusCode, responseBody(t, publishResponse))
	}
	_ = responseBody(t, publishResponse)
	newURL := server.URL + "/catalog/maker-a/switch-item"
	newResponse, err := client.Get(newURL)
	if err != nil {
		t.Fatal(err)
	}
	newBody := responseBody(t, newResponse)
	if newResponse.StatusCode != http.StatusOK || !strings.Contains(newBody, "SWITCH-A") || !strings.Contains(newBody, `data-site-epoch="2"`) || !strings.Contains(newBody, "http://catalog.example.test/catalog/maker-a/switch-item") {
		t.Fatalf("new route status=%d body=%s", newResponse.StatusCode, newBody)
	}
	staleRequest, err := http.NewRequest(http.MethodGet, oldURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	staleRequest.Header.Set("If-None-Match", oldETag)
	noRedirectClient := *client
	noRedirectClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	staleResponse, err := noRedirectClient.Do(staleRequest)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, staleResponse); staleResponse.StatusCode != http.StatusPermanentRedirect || staleResponse.Header.Get("Location") != "/catalog/maker-a/switch-item" || strings.Contains(body, "SWITCH-A") {
		t.Fatalf("old route history status=%d location=%q body=%s", staleResponse.StatusCode, staleResponse.Header.Get("Location"), body)
	}
	assertPublicAbsent(t, client, server.URL+"/sitemap.xml", "/products/switch-item")
	assertPublicRepresentation(t, client, server.URL+"/sitemap.xml", []string{"/catalog/maker-a/switch-item"})
	assertPublicRepresentation(t, client, server.URL+"/catalog/products-000001.json", []string{
		`"site_epoch": 2`,
		`"canonical_url": "http://catalog.example.test/catalog/maker-a/switch-item"`,
		`"json_url": "http://catalog.example.test/catalog/maker-a/switch-item.json"`,
	})
	assertPublicAbsent(t, client, server.URL+"/catalog/products-000001.json", "/products/switch-item")

	makerB := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"manufacturer","name":"Maker B","slug":"maker-b","status":"active","revision":1}`)
	secondResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"SWITCH-B","slug":"switch-item","manufacturer_id":"`+makerB.ID+`","brand_id":"`+sharedBrand.ID+`","status":"published"}`)
	if secondResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create second status=%d body=%s", secondResponse.StatusCode, responseBody(t, secondResponse))
	}
	var second catalog.Product
	decodeResponseJSON(t, secondResponse, &second)
	waitForPublicBody(t, client, server.URL+"/catalog/maker-b/switch-item", "SWITCH-B")

	conflictPreview := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/preview", csrf,
		`{"product_prefix":"/parts","url_pattern":"brand"}`)
	if conflictPreview.StatusCode != http.StatusOK {
		t.Fatalf("conflict preview status=%d body=%s", conflictPreview.StatusCode, responseBody(t, conflictPreview))
	}
	var conflicted publishing.SiteRoutePreview
	decodeResponseJSON(t, conflictPreview, &conflicted)
	if len(conflicted.Conflicts) < 2 {
		t.Fatalf("expected route conflict: %+v", conflicted)
	}
	rejected := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":2,"expected_working_revision":1,"config":{"product_prefix":"/parts","url_pattern":"brand"}}`)
	if rejected.StatusCode != http.StatusConflict {
		t.Fatalf("invalid publish status=%d body=%s", rejected.StatusCode, responseBody(t, rejected))
	}
	_ = responseBody(t, rejected)
	waitForPublicBody(t, client, newURL, "SWITCH-A")
	waitForPublicBody(t, client, server.URL+"/catalog/maker-b/switch-item", "SWITCH-B")

	server.Close()
	app.Close()
	app, _, err = New(store, config)
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	restarted := httptest.NewServer(app)
	defer restarted.Close()
	waitForPublicBody(t, restarted.Client(), restarted.URL+"/catalog/maker-a/switch-item", "SWITCH-A")
	assertPublicRepresentation(t, restarted.Client(), restarted.URL+"/catalog/products-000001.json", []string{
		`"canonical_url": "http://catalog.example.test/catalog/maker-a/switch-item"`,
	})
}

func TestCustomProductPathHistoryArchiveAndReusePrecedence(t *testing.T) {
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
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
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

	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"PATH-A","slug":"path-a","status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.StatusCode, responseBody(t, created))
	}
	var product catalog.Product
	decodeResponseJSON(t, created, &product)
	waitForPublicBody(t, client, server.URL+"/products/path-a", "PATH-A")

	updated := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/url", csrf,
		`{"expected_revision":1,"slug":"path-b","custom_path":"/featured/path-a"}`)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("URL update status=%d body=%s", updated.StatusCode, responseBody(t, updated))
	}
	decodeResponseJSON(t, updated, &product)
	waitForPublicBody(t, client, server.URL+"/featured/path-a", "PATH-A")

	noRedirectClient := *client
	noRedirectClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	oldResponse, err := noRedirectClient.Get(server.URL + "/products/path-a")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, oldResponse); oldResponse.StatusCode != http.StatusPermanentRedirect || oldResponse.Header.Get("Location") != "/featured/path-a" || strings.Contains(body, "PATH-A") {
		t.Fatalf("old route status=%d location=%q body=%s", oldResponse.StatusCode, oldResponse.Header.Get("Location"), body)
	}

	archive := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/archive", csrf, `{"expected_revision":2}`)
	if body := responseBody(t, archive); archive.StatusCode != http.StatusNoContent {
		t.Fatalf("archive status=%d body=%s", archive.StatusCode, body)
	}
	gone, err := noRedirectClient.Get(server.URL + "/featured/path-a")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, gone); gone.StatusCode != http.StatusGone || strings.Contains(body, "PATH-A") {
		t.Fatalf("archived route status=%d body=%s", gone.StatusCode, body)
	}

	replacement := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"PATH-REUSED","slug":"replacement","custom_path":"/featured/path-a","status":"published"}`)
	if replacement.StatusCode != http.StatusCreated {
		t.Fatalf("replacement status=%d body=%s", replacement.StatusCode, responseBody(t, replacement))
	}
	waitForPublicBody(t, client, server.URL+"/featured/path-a", "PATH-REUSED")
}

func TestWebsiteWorkingPreviewPublishAndRestoreVersion(t *testing.T) {
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
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
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

	configurationResponse, err := client.Get(server.URL + "/admin/api/website/configuration")
	if err != nil {
		t.Fatal(err)
	}
	var initial site.State
	decodeResponseJSON(t, configurationResponse, &initial)
	if initial.WorkingRevision != 1 || initial.ActiveEpoch != 1 || initial.Active.Organization.DisplayName != "Product Catalog" {
		t.Fatalf("initial website state=%+v", initial)
	}

	saveBody := `{"expected_revision":1,"configuration":{"organization":{"display_name":"Example Components","legal_name":"Example Components LLC","official_website":"https://example.test","privacy_url":"https://example.test/privacy","contact_links":[{"id":"sales","label":"Sales","url":"https://example.test/sales","sort_order":1}]},"navigation":[{"id":"catalog","label":"Parts","url":"/search","sort_order":10,"visible":true},{"id":"support","label":"Support","url":"https://example.test/support","sort_order":20,"open_new_window":true,"visible":true}],"theme":{"primary_color":"#123456","secondary_color":"#654321","font_family":"Arial, sans-serif","content_width_px":960,"custom_css":"h1{letter-spacing:.01em}","custom_css_enabled":true},"seo":{"default_title":"Example Components Catalog","default_description":"Electronic component catalog"}}}`
	savedResponse := adminJSONRequest(t, client, http.MethodPut, server.URL+"/admin/api/website/configuration", csrf, saveBody)
	if savedResponse.StatusCode != http.StatusOK {
		t.Fatalf("save status=%d body=%s", savedResponse.StatusCode, responseBody(t, savedResponse))
	}
	var saved site.State
	decodeResponseJSON(t, savedResponse, &saved)
	if saved.WorkingRevision != 2 || saved.Working.Organization.DisplayName != "Example Components" || saved.Active.Organization.DisplayName != "Product Catalog" {
		t.Fatalf("working save leaked into active state: %+v", saved)
	}

	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf, `{"part_number":"WEBSITE-VERSION","slug":"website-version","status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", created.StatusCode, responseBody(t, created))
	}
	waitForPublicBody(t, client, server.URL+"/products/website-version", "Product Catalog")

	previewResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/preview", csrf, `{"expected_working_revision":2}`)
	if previewResponse.StatusCode != http.StatusCreated {
		t.Fatalf("website preview status=%d body=%s", previewResponse.StatusCode, responseBody(t, previewResponse))
	}
	var previewReceipt struct {
		URL string `json:"url"`
	}
	decodeResponseJSON(t, previewResponse, &previewReceipt)
	publicPreview, err := http.Get(server.URL + previewReceipt.URL)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, publicPreview); publicPreview.StatusCode != http.StatusUnauthorized || strings.Contains(body, "Example Components") {
		t.Fatalf("anonymous preview status=%d body=%s", publicPreview.StatusCode, body)
	}
	privatePreview, err := client.Get(server.URL + previewReceipt.URL)
	if err != nil {
		t.Fatal(err)
	}
	privatePreviewBody := responseBody(t, privatePreview)
	if privatePreview.StatusCode != http.StatusOK || !strings.Contains(privatePreviewBody, "Example Components") || !strings.Contains(privatePreviewBody, "letter-spacing") || privatePreview.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("private preview status=%d cache=%q body=%s", privatePreview.StatusCode, privatePreview.Header.Get("Cache-Control"), privatePreviewBody)
	}

	routePreviewResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/preview", csrf, `{"product_prefix":"/products","url_pattern":"compact"}`)
	if routePreviewResponse.StatusCode != http.StatusOK {
		t.Fatalf("route preview status=%d body=%s", routePreviewResponse.StatusCode, responseBody(t, routePreviewResponse))
	}
	var routePreview publishing.SiteRoutePreview
	decodeResponseJSON(t, routePreviewResponse, &routePreview)
	if routePreview.WorkingRevision != 2 {
		t.Fatalf("route preview working revision=%d", routePreview.WorkingRevision)
	}
	publishResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if publishResponse.StatusCode != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", publishResponse.StatusCode, responseBody(t, publishResponse))
	}
	waitForPublicBody(t, client, server.URL+"/products/website-version", "Example Components")
	for _, publicURL := range []string{"/search", "/rfq"} {
		response, err := client.Get(server.URL + publicURL)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode != http.StatusOK || !strings.Contains(body, "Example Components") || !strings.Contains(body, `data-site-epoch="2"`) {
			t.Fatalf("dynamic public page %s mixed site epoch status=%d body=%s", publicURL, response.StatusCode, body)
		}
	}
	jsonResponse, err := client.Get(server.URL + "/products/website-version.json")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, jsonResponse); jsonResponse.StatusCode != http.StatusOK || !strings.Contains(body, `"display_name": "Example Components"`) || !strings.Contains(body, `"site_epoch": 2`) {
		t.Fatalf("public JSON did not share Website version status=%d body=%s", jsonResponse.StatusCode, body)
	}
	cssResponse, err := client.Get(server.URL + "/site.css?v=2")
	if err != nil {
		t.Fatal(err)
	}
	cssETag := cssResponse.Header.Get("ETag")
	if body := responseBody(t, cssResponse); cssResponse.StatusCode != http.StatusOK || !strings.Contains(body, "letter-spacing") || cssETag == "" || cssResponse.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("active custom CSS status=%d etag=%q cache=%q body=%s", cssResponse.StatusCode, cssETag, cssResponse.Header.Get("Cache-Control"), body)
	}
	disableCSS := postAdminJSON(t, client, server.URL+"/admin/api/website/custom-css", csrf, `{"disabled":true}`)
	if disableCSS.StatusCode != http.StatusOK {
		t.Fatalf("disable custom CSS status=%d body=%s", disableCSS.StatusCode, responseBody(t, disableCSS))
	}
	staleCSSRequest, err := http.NewRequest(http.MethodGet, server.URL+"/site.css?v=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	staleCSSRequest.Header.Set("If-None-Match", cssETag)
	staleCSSResponse, err := client.Do(staleCSSRequest)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, staleCSSResponse); staleCSSResponse.StatusCode != http.StatusNotFound || strings.Contains(body, "letter-spacing") {
		t.Fatalf("Safe Mode stale CSS status=%d body=%s", staleCSSResponse.StatusCode, body)
	}
	adminPage, err := client.Get(server.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, adminPage); adminPage.StatusCode != http.StatusOK || strings.Contains(body, "/site.css") || strings.Contains(body, "letter-spacing") {
		t.Fatalf("Admin inherited Custom CSS status=%d body=%s", adminPage.StatusCode, body)
	}
	enableCSS := postAdminJSON(t, client, server.URL+"/admin/api/website/custom-css", csrf, `{"disabled":false}`)
	if enableCSS.StatusCode != http.StatusOK {
		t.Fatalf("re-enable custom CSS status=%d body=%s", enableCSS.StatusCode, responseBody(t, enableCSS))
	}
	cssResponse, err = client.Get(server.URL + "/site.css?v=2")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, cssResponse); cssResponse.StatusCode != http.StatusOK || !strings.Contains(body, "letter-spacing") {
		t.Fatalf("re-enabled custom CSS status=%d body=%s", cssResponse.StatusCode, body)
	}

	versionsResponse, err := client.Get(server.URL + "/admin/api/website/versions")
	if err != nil {
		t.Fatal(err)
	}
	var versions []site.Version
	decodeResponseJSON(t, versionsResponse, &versions)
	if len(versions) != 2 || versions[0].SiteEpoch != 2 || versions[0].Configuration.Organization.DisplayName != "Example Components" {
		t.Fatalf("website versions=%+v", versions)
	}

	restoreResponse := postAdminJSON(t, client, server.URL+"/admin/api/website/versions/restore", csrf,
		`{"expected_working_revision":2,"version":1}`)
	if restoreResponse.StatusCode != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restoreResponse.StatusCode, responseBody(t, restoreResponse))
	}
	var restored site.State
	decodeResponseJSON(t, restoreResponse, &restored)
	if restored.WorkingRevision != 3 || restored.Working.Organization.DisplayName != "Product Catalog" || restored.Active.Organization.DisplayName != "Example Components" {
		t.Fatalf("restore must create an unpublished working copy: %+v", restored)
	}
}

func adminJSONRequest(t *testing.T, client *http.Client, method, endpoint, csrf, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, strings.NewReader(body))
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
