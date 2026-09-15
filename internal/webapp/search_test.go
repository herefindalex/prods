package webapp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestPublicSearchProjectionUsesPinnedUnicodeFoldLiteralWildcardsAndActiveRevalidation(t *testing.T) {
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

	products := []string{
		`{"part_number":"100%_REAL\\PART","slug":"literal-wildcards","status":"published"}`,
		`{"part_number":"ORDINARY","slug":"ordinary","status":"published"}`,
		`{"part_number":"ÉCHO","slug":"accent-upper","manufacturer":"Maker","status":"published"}`,
		`{"part_number":"ABC123","slug":"case-upper","manufacturer":"Maker","status":"published"}`,
		`{"part_number":"abc123","slug":"case-lower","manufacturer":"Maker","status":"published"}`,
	}
	created := make([]catalog.Product, 0, len(products))
	for _, body := range products {
		response := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf, body)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("create status=%d body=%s", response.StatusCode, responseBody(t, response))
		}
		var product catalog.Product
		decodeResponseJSON(t, response, &product)
		created = append(created, product)
		waitForPublicBody(t, client, server.URL+"/products/"+product.Slug, product.PartNumber)
	}

	literalBody := publicSearchBody(t, client, server.URL, "%_")
	if !strings.Contains(literalBody, "100%_REAL") || strings.Contains(literalBody, "ORDINARY") {
		t.Fatalf("literal wildcard query leaked wildcard semantics: %s", literalBody)
	}
	accentBody := publicSearchBody(t, client, server.URL, "écho")
	if !strings.Contains(accentBody, "ÉCHO") {
		t.Fatalf("Unicode case fold missed accent variant: %s", accentBody)
	}
	caseBody := publicSearchBody(t, client, server.URL, "AbC123")
	if !strings.Contains(caseBody, "ABC123") || !strings.Contains(caseBody, "abc123") {
		t.Fatalf("case-insensitive search changed identity behavior: %s", caseBody)
	}

	hide := postAdminJSON(t, client, server.URL+"/admin/api/products/"+created[2].ID+"/hide", csrf, `{"expected_revision":1}`)
	if body := responseBody(t, hide); hide.StatusCode != http.StatusNoContent {
		t.Fatalf("hide status=%d body=%s", hide.StatusCode, body)
	}
	assertPublicAbsent(t, client, server.URL+"/search?q="+url.QueryEscape("écho"), "ÉCHO")
}

func TestPublicSearchFiltersUseActiveCategoryTrailAndDoNotIndexDescription(t *testing.T) {
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
	parent := createCategoryAPI(t, client, server.URL, csrf, `{"name":"Power","slug":"power","status":"active","revision":1}`)
	child := createCategoryAPI(t, client, server.URL, csrf, `{"parent_id":"`+parent.ID+`","name":"Regulators","slug":"regulators","status":"active","revision":1}`)
	other := createCategoryAPI(t, client, server.URL, csrf, `{"name":"Connectors","slug":"connectors","status":"active","revision":1}`)
	makerA := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"manufacturer","name":"Maker A","slug":"maker-a","status":"active","revision":1}`)
	makerB := createDictionaryAPI(t, client, server.URL, csrf, `{"kind":"manufacturer","name":"Maker B","slug":"maker-b","status":"active","revision":1}`)

	childResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"FILTER-CHILD","name":"Shared Filter Name","description":"description-only-needle","category_id":"`+child.ID+`","manufacturer_id":"`+makerA.ID+`","status":"published"}`)
	otherResponse := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"FILTER-OTHER","name":"Shared Filter Name","category_id":"`+other.ID+`","manufacturer_id":"`+makerB.ID+`","status":"published"}`)
	if childResponse.StatusCode != http.StatusCreated || otherResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create filtered products status=%d/%d", childResponse.StatusCode, otherResponse.StatusCode)
	}
	var childProduct, otherProduct catalog.Product
	decodeResponseJSON(t, childResponse, &childProduct)
	decodeResponseJSON(t, otherResponse, &otherProduct)
	waitForPublicBody(t, client, server.URL+"/products/"+childProduct.Slug, childProduct.PartNumber)
	waitForPublicBody(t, client, server.URL+"/products/"+otherProduct.Slug, otherProduct.PartNumber)

	filtered, err := client.Get(server.URL + "/search?q=" + url.QueryEscape("Shared Filter") + "&category_id=" + url.QueryEscape(parent.ID))
	if err != nil {
		t.Fatal(err)
	}
	filteredBody := responseBody(t, filtered)
	if filtered.StatusCode != http.StatusOK || !strings.Contains(filteredBody, childProduct.PartNumber) || strings.Contains(filteredBody, otherProduct.PartNumber) ||
		!strings.Contains(filteredBody, `value="`+parent.ID+`" selected`) {
		t.Fatalf("parent category filter status=%d body=%s", filtered.StatusCode, filteredBody)
	}
	byMaker, err := client.Get(server.URL + "/search?q=" + url.QueryEscape("Shared Filter") + "&manufacturer_id=" + url.QueryEscape(makerB.ID))
	if err != nil {
		t.Fatal(err)
	}
	byMakerBody := responseBody(t, byMaker)
	if !strings.Contains(byMakerBody, otherProduct.PartNumber) || strings.Contains(byMakerBody, childProduct.PartNumber) {
		t.Fatalf("manufacturer filter body=%s", byMakerBody)
	}
	descriptionOnly := publicSearchBody(t, client, server.URL, "description-only-needle")
	if strings.Contains(descriptionOnly, childProduct.PartNumber) {
		t.Fatalf("Description unexpectedly entered public full-text search: %s", descriptionOnly)
	}
}

func TestSearchURLPreservesFiltersAcrossPagination(t *testing.T) {
	got := searchURL("query", 2, "zh-TW", publicSearchFilters{
		ManufacturerID: "maker-1", BrandID: "brand-1", CategoryID: "category-1",
	})
	for _, expected := range []string{"q=query", "page=2", "lang=zh-TW", "manufacturer_id=maker-1", "brand_id=brand-1", "category_id=category-1"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("pagination URL %q missing %q", got, expected)
		}
	}
}

func publicSearchBody(t *testing.T, client *http.Client, baseURL, query string) string {
	t.Helper()
	response, err := client.Get(baseURL + "/search?q=" + url.QueryEscape(query))
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("search status=%d body=%s", response.StatusCode, body)
	}
	return body
}
