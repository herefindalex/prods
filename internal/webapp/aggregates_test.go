package webapp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestPublicAggregatesAndSitemapUseOnlyActiveSnapshots(t *testing.T) {
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
		OwnerEmail:       "owner@example.test",
		OwnerDisplayName: "Owner",
		PasswordHash:     passwordHash,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US"},
		TimeZone:         "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		BaseURL:   "http://catalog.example.test",
		PublicDir: filepath.Join(root, "generated"),
		AssetDir:  filepath.Join(root, "assets"),
		WorkDir:   filepath.Join(root, "work"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	parent := createCategoryAPI(t, client, server.URL, csrf, `{"name":"Power","slug":"power","status":"active","revision":1}`)
	child := createCategoryAPI(t, client, server.URL, csrf, `{"parent_id":"`+parent.ID+`","name":"Regulators","slug":"regulators","status":"active","revision":1}`)
	manufacturer := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"manufacturer","name":"Acme Components","slug":"acme","status":"active","revision":1}`)
	brand := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"brand","name":"Volt Star","slug":"volt-star","status":"active","revision":1}`)

	productResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"AGG-1","name":"Aggregate regulator","category_id":"`+child.ID+`","manufacturer_id":"`+manufacturer.ID+`","brand_id":"`+brand.ID+`","status":"published"}`)
	if productResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", productResponse.StatusCode, responseBody(t, productResponse))
	}
	var product catalog.Product
	decodeResponseJSON(t, productResponse, &product)
	waitForPublicBody(t, client, server.URL+"/products/"+product.Slug, "AGG-1")

	aggregates := []string{
		"/categories/power",
		"/categories/power/regulators",
		"/manufacturers/acme",
		"/brands/volt-star",
	}
	etags := make(map[string]string)
	for _, route := range aggregates {
		response, err := client.Get(server.URL + route)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode != http.StatusOK || !strings.Contains(body, "AGG-1") {
			t.Fatalf("aggregate %s status=%d body=%s", route, response.StatusCode, body)
		}
		etags[route] = response.Header.Get("ETag")
	}
	sitemap, err := client.Get(server.URL + "/sitemap.xml")
	if err != nil {
		t.Fatal(err)
	}
	sitemapBody := responseBody(t, sitemap)
	for _, expected := range append(aggregates, "/products/"+product.Slug) {
		if !strings.Contains(sitemapBody, "http://catalog.example.test"+expected) {
			t.Fatalf("sitemap missing %s: %s", expected, sitemapBody)
		}
	}
	machineExpectations := map[string][]string{
		"/robots.txt":                   {"Sitemap: http://catalog.example.test/sitemap.xml", "Disallow: /admin/"},
		"/llms.txt":                     {"Catalog manifest", "http://catalog.example.test/catalog/manifest.json"},
		"/catalog/manifest.json":        {`"format_version": "prods.catalog-manifest.v1"`, "/catalog/products-000001.json", `"product_count": 1`},
		"/catalog/products-000001.json": {`"format_version": "prods.catalog-shard.v1"`, product.ID, "AGG-1", "/products/" + product.Slug + ".json"},
	}
	for path, expected := range machineExpectations {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode != http.StatusOK || response.Header.Get("ETag") == "" {
			t.Fatalf("machine output %s status=%d etag=%q body=%s", path, response.StatusCode, response.Header.Get("ETag"), body)
		}
		for _, value := range expected {
			if !strings.Contains(body, value) {
				t.Errorf("machine output %s missing %q: %s", path, value, body)
			}
		}
	}
	shardResponse, err := client.Get(server.URL + "/catalog/products-000001.json")
	if err != nil {
		t.Fatal(err)
	}
	shardETag := shardResponse.Header.Get("ETag")
	_ = responseBody(t, shardResponse)

	hide := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/hide", csrf, `{"expected_revision":1}`)
	if body := responseBody(t, hide); hide.StatusCode != http.StatusNoContent {
		t.Fatalf("hide status=%d body=%s", hide.StatusCode, body)
	}
	for _, route := range aggregates {
		request, err := http.NewRequest(http.MethodGet, server.URL+route, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("If-None-Match", etags[route])
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNotModified || strings.Contains(body, "AGG-1") {
			t.Fatalf("revoked aggregate %s status=%d body=%s", route, response.StatusCode, body)
		}
	}
	assertPublicAbsent(t, client, server.URL+"/sitemap.xml", product.ID)
	assertPublicAbsent(t, client, server.URL+"/catalog/manifest.json", "/catalog/products-000001.json")
	staleShard, err := http.NewRequest(http.MethodGet, server.URL+"/catalog/products-000001.json", nil)
	if err != nil {
		t.Fatal(err)
	}
	staleShard.Header.Set("If-None-Match", shardETag)
	staleShard.Header.Set("If-Range", shardETag)
	staleShard.Header.Set("Range", "bytes=0-40")
	staleShardResponse, err := client.Do(staleShard)
	if err != nil {
		t.Fatal(err)
	}
	staleShardBody := responseBody(t, staleShardResponse)
	if staleShardResponse.StatusCode == http.StatusOK || staleShardResponse.StatusCode == http.StatusPartialContent || staleShardResponse.StatusCode == http.StatusNotModified || strings.Contains(staleShardBody, product.ID) {
		t.Fatalf("revoked shard admitted status=%d body=%s", staleShardResponse.StatusCode, staleShardBody)
	}
}

func createCategoryAPI(t *testing.T, client *http.Client, baseURL, csrf, body string) catalog.Category {
	t.Helper()
	response := postAdminJSON(t, client, baseURL+"/admin/api/categories", csrf, body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create category status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var category catalog.Category
	decodeResponseJSON(t, response, &category)
	return category
}

func createDictionaryAPI(t *testing.T, client *http.Client, baseURL, csrf, body string) catalog.DictionaryEntry {
	t.Helper()
	response := postAdminJSON(t, client, baseURL+"/admin/api/dictionaries", csrf, body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create dictionary status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var entry catalog.DictionaryEntry
	decodeResponseJSON(t, response, &entry)
	return entry
}
