package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func TestCategoryListingProfileActivatesOnlyWithWebsitePublishAndKeepsEmptyCategory(t *testing.T) {
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
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), AssetDir: filepath.Join(root, "assets"), WorkDir: filepath.Join(root, "work")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	category := createCategoryAPI(t, client, server.URL, csrf, `{"id":"cat_listing","name":"Listing","slug":"listing","status":"active","revision":1}`)
	empty := createCategoryAPI(t, client, server.URL, csrf, `{"id":"cat_empty","name":"Empty","slug":"empty","status":"active","revision":1}`)
	for _, payload := range []string{
		`{"id":"spec_input","name":"Input voltage","preferred_unit":"V","filterable":true,"semantic_version":1,"status":"active","revision":1}`,
		`{"id":"spec_output","name":"Output current","preferred_unit":"A","filterable":true,"semantic_version":1,"status":"active","revision":1}`,
	} {
		response := postAdminJSON(t, client, server.URL+"/admin/api/specs", csrf, payload)
		if body := responseBody(t, response); response.StatusCode != http.StatusCreated {
			t.Fatalf("create spec status=%d body=%s", response.StatusCode, body)
		}
	}
	setResponse := postAdminJSON(t, client, server.URL+"/admin/api/spec-sets", csrf, `{"id":"set_listing","name":"Listing","status":"active","revision":1,"spec_ids":["spec_input","spec_output"]}`)
	if body := responseBody(t, setResponse); setResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create spec set status=%d body=%s", setResponse.StatusCode, body)
	}
	assignment := postAdminJSON(t, client, server.URL+"/admin/api/categories/"+category.ID+"/spec-set", csrf, `{"expected_revision":1,"spec_set_id":"set_listing"}`)
	if body := responseBody(t, assignment); assignment.StatusCode != http.StatusNoContent {
		t.Fatalf("assign spec set status=%d body=%s", assignment.StatusCode, body)
	}
	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf, `{"part_number":"LIST-1","name":"Listing product","category_id":"`+category.ID+`","status":"published"}`)
	if body := responseBody(t, created); created.StatusCode != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", created.StatusCode, body)
	}
	waitForPublicBody(t, client, server.URL+"/categories/listing", "LIST-1")
	before, err := client.Get(server.URL + "/categories/listing")
	if err != nil {
		t.Fatal(err)
	}
	beforeBody := responseBody(t, before)
	if strings.Index(beforeBody, "Input voltage") > strings.Index(beforeBody, "Output current") {
		t.Fatalf("default Common order unexpected: %s", beforeBody)
	}

	state, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state.Working.CategoryListingProfiles = map[string]site.CategoryListingProfile{
		category.ID: {
			VisibleColumns: []string{"part_number", "spec:spec_output", "spec:spec_input", "rfq"},
			DefaultSort:    "part_number", DefaultSortDirection: "asc", MobileKeySpecs: []string{"spec:spec_output"},
		},
	}
	state, err = store.SaveWebsiteWorking(t.Context(), owner.ID, state.WorkingRevision, state.Working)
	if err != nil {
		t.Fatal(err)
	}
	unpublished, err := client.Get(server.URL + "/categories/listing")
	if err != nil {
		t.Fatal(err)
	}
	unpublishedBody := responseBody(t, unpublished)
	if strings.Index(unpublishedBody, "Input voltage") > strings.Index(unpublishedBody, "Output current") {
		t.Fatal("working listing profile leaked before Website Publish")
	}
	publishPayload, err := json.Marshal(map[string]any{
		"expected_epoch": 1, "expected_working_revision": state.WorkingRevision,
		"config": map[string]any{"product_prefix": "/products", "url_pattern": "compact"},
	})
	if err != nil {
		t.Fatal(err)
	}
	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf, string(publishPayload))
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish Website status=%d body=%s", published.StatusCode, body)
	}
	after, err := client.Get(server.URL + "/categories/listing")
	if err != nil {
		t.Fatal(err)
	}
	afterBody := responseBody(t, after)
	if strings.Index(afterBody, "Output current") < 0 || strings.Index(afterBody, "Input voltage") < 0 || strings.Index(afterBody, "Output current") > strings.Index(afterBody, "Input voltage") {
		t.Fatalf("published listing profile not active: %s", afterBody)
	}
	if strings.Contains(afterBody, ">Name</th>") {
		t.Fatal("published profile retained a removed column")
	}
	emptyResponse, err := client.Get(server.URL + "/categories/" + empty.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, emptyResponse); emptyResponse.StatusCode != http.StatusOK || !strings.Contains(body, "no published products") {
		t.Fatalf("empty Category status=%d body=%s", emptyResponse.StatusCode, body)
	}
}

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
		categoryRoute := strings.HasPrefix(route, "/categories/")
		if response.StatusCode == http.StatusNotModified || strings.Contains(body, "AGG-1") || (response.StatusCode == http.StatusOK && !categoryRoute) {
			t.Fatalf("revoked aggregate %s status=%d body=%s", route, response.StatusCode, body)
		}
		if categoryRoute && response.StatusCode != http.StatusOK {
			t.Fatalf("empty active category %s status=%d body=%s", route, response.StatusCode, body)
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
