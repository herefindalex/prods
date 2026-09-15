package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestApplicationsFlowThroughAdminPublicationSearchAggregateAndRevocation(t *testing.T) {
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

	application := createDictionaryAPI(t, client, server.URL, csrf,
		`{"kind":"application","name":"Factory Automation","slug":"factory-automation","status":"active","revision":1}`)
	lifecycle := createDictionaryAPI(t, client, server.URL, csrf,
		`{"kind":"lifecycle","name":"Not Recommended for New Designs","slug":"nrnd","status":"active","revision":1}`)
	createdResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"APP-PUBLIC-1","name":"Application aware product","application_ids":["`+application.ID+`"],"lifecycle_id":"`+lifecycle.ID+`","status":"published"}`)
	if createdResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdResponse.StatusCode, responseBody(t, createdResponse))
	}
	var product catalog.Product
	decodeResponseJSON(t, createdResponse, &product)
	if len(product.Applications) != 1 || product.Applications[0].Name != "Factory Automation" {
		t.Fatalf("created applications = %+v", product.Applications)
	}
	if product.Lifecycle != "Not Recommended for New Designs" || product.LifecycleID != lifecycle.ID {
		t.Fatalf("created lifecycle = %q (%q)", product.Lifecycle, product.LifecycleID)
	}

	for _, path := range []string{"/products/" + product.Slug, "/products/" + product.Slug + ".json", "/products/" + product.Slug + ".md"} {
		response, body := getEventually(t, client, server.URL+path, "Factory Automation")
		if response.StatusCode != http.StatusOK || !strings.Contains(body, "Factory Automation") || !strings.Contains(body, "/applications/factory-automation") || !strings.Contains(body, "Not Recommended for New Designs") {
			t.Fatalf("representation %s status=%d body=%s", path, response.StatusCode, body)
		}
	}

	aggregatePath := "/applications/factory-automation"
	views, err := app.publisher.Views()
	if err != nil || len(views) != 1 || len(views[0].Applications) != 1 {
		t.Fatalf("active views before aggregate = %+v err=%v", views, err)
	}
	if views[0].Applications[0].URL != "http://catalog.example.test"+aggregatePath {
		t.Fatalf("application URL = %q", views[0].Applications[0].URL)
	}
	if views[0].Lifecycle != "Not Recommended for New Designs" || views[0].LifecycleID != lifecycle.ID {
		t.Fatalf("public lifecycle = %q (%q)", views[0].Lifecycle, views[0].LifecycleID)
	}
	aggregate, aggregateBody := getEventually(t, client, server.URL+aggregatePath, "APP-PUBLIC-1")
	aggregateETag := aggregate.Header.Get("ETag")
	if aggregate.StatusCode != http.StatusOK || aggregateETag == "" || !strings.Contains(aggregateBody, "APP-PUBLIC-1") {
		t.Fatalf("aggregate status=%d etag=%q body=%s", aggregate.StatusCode, aggregateETag, aggregateBody)
	}
	search, err := client.Get(server.URL + "/search?q=" + url.QueryEscape("factory automation"))
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, search); search.StatusCode != http.StatusOK || !strings.Contains(body, "APP-PUBLIC-1") {
		t.Fatalf("application search status=%d body=%s", search.StatusCode, body)
	}
	lifecycleSearch, err := client.Get(server.URL + "/search?q=" + url.QueryEscape("not recommended for new designs"))
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, lifecycleSearch); lifecycleSearch.StatusCode != http.StatusOK || !strings.Contains(body, "APP-PUBLIC-1") {
		t.Fatalf("lifecycle search status=%d body=%s", lifecycleSearch.StatusCode, body)
	}
	sitemap, err := client.Get(server.URL + "/sitemap.xml")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, sitemap); sitemap.StatusCode != http.StatusOK || !strings.Contains(body, "http://catalog.example.test"+aggregatePath) {
		t.Fatalf("application sitemap status=%d body=%s", sitemap.StatusCode, body)
	}

	hide := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/hide", csrf, `{"expected_revision":1}`)
	if body := responseBody(t, hide); hide.StatusCode != http.StatusNoContent {
		t.Fatalf("hide status=%d body=%s", hide.StatusCode, body)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+aggregatePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", aggregateETag)
	revoked, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, revoked); revoked.StatusCode == http.StatusOK || revoked.StatusCode == http.StatusNotModified || strings.Contains(body, "APP-PUBLIC-1") {
		t.Fatalf("revoked aggregate status=%d body=%s", revoked.StatusCode, body)
	}
	assertPublicAbsent(t, client, server.URL+"/search?q="+url.QueryEscape("factory automation"), "APP-PUBLIC-1")
	assertPublicAbsent(t, client, server.URL+"/sitemap.xml", aggregatePath)
}

func getEventually(t *testing.T, client *http.Client, target, content string) (*http.Response, string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err := client.Get(target)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode == http.StatusOK && strings.Contains(body, content) {
			return response, body
		}
		if time.Now().After(deadline) {
			return response, body
		}
		time.Sleep(10 * time.Millisecond)
	}
}
