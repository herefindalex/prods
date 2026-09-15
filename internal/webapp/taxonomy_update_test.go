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

func TestDictionaryUpdatePreviewRepublishesAffectedProductAndAggregates(t *testing.T) {
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
		`{"kind":"application","name":"Legacy Motion","slug":"legacy-motion","status":"active","revision":1}`)
	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"TAXONOMY-1","application_ids":["`+application.ID+`"],"status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.StatusCode, responseBody(t, created))
	}
	var product catalog.Product
	decodeResponseJSON(t, created, &product)
	productURL := server.URL + "/products/" + product.Slug
	before, beforeBody := getEventually(t, client, productURL, "Legacy Motion")
	if before.StatusCode != http.StatusOK {
		t.Fatalf("initial product status=%d body=%s", before.StatusCode, beforeBody)
	}
	oldETag := before.Header.Get("ETag")

	previewResponse := postAdminJSON(t, client, server.URL+"/admin/api/dictionaries/"+application.ID+"/update-preview", csrf,
		`{"expected_revision":1,"name":"Motion Control","slug":"motion-control"}`)
	if previewResponse.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.StatusCode, responseBody(t, previewResponse))
	}
	var preview catalog.TaxonomyImpact
	decodeResponseJSON(t, previewResponse, &preview)
	if len(preview.AffectedProducts) != 1 || preview.AffectedProducts[0].ProductID != product.ID {
		t.Fatalf("preview = %+v", preview)
	}
	unchanged, err := store.Product(t.Context(), product.ID)
	if err != nil || unchanged.Revision != product.Revision || len(unchanged.Applications) != 1 || unchanged.Applications[0].Name != "Legacy Motion" {
		t.Fatalf("preview mutation = %+v, %v", unchanged, err)
	}

	updateResponse := postAdminJSON(t, client, server.URL+"/admin/api/dictionaries/"+application.ID+"/update", csrf,
		`{"expected_revision":1,"name":"Motion Control","slug":"motion-control"}`)
	if updateResponse.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updateResponse.StatusCode, responseBody(t, updateResponse))
	}
	_ = responseBody(t, updateResponse)
	updatedProduct, updatedBody := getEventually(t, client, productURL, "Motion Control")
	if updatedProduct.StatusCode != http.StatusOK || strings.Contains(updatedBody, "Legacy Motion") {
		t.Fatalf("updated product status=%d body=%s", updatedProduct.StatusCode, updatedBody)
	}

	request, err := http.NewRequest(http.MethodGet, productURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", oldETag)
	after, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	afterBody := responseBody(t, after)
	if after.StatusCode != http.StatusOK || !strings.Contains(afterBody, "Motion Control") || strings.Contains(afterBody, "Legacy Motion") {
		t.Fatalf("updated product status=%d body=%s", after.StatusCode, afterBody)
	}

	newAggregate := "/applications/motion-control"
	aggregate, aggregateBody := getEventually(t, client, server.URL+newAggregate, "TAXONOMY-1")
	if aggregate.StatusCode != http.StatusOK || !strings.Contains(aggregateBody, "TAXONOMY-1") {
		t.Fatalf("new aggregate status=%d body=%s", aggregate.StatusCode, aggregateBody)
	}
	assertPublicAbsent(t, client, server.URL+"/applications/legacy-motion", "TAXONOMY-1")
	assertPublicAbsent(t, client, server.URL+"/search?q="+url.QueryEscape("legacy motion"), "TAXONOMY-1")
	search, searchBody := getEventually(t, client, server.URL+"/search?q="+url.QueryEscape("motion control"), "TAXONOMY-1")
	if search.StatusCode != http.StatusOK || !strings.Contains(searchBody, "TAXONOMY-1") {
		t.Fatalf("new search status=%d body=%s", search.StatusCode, searchBody)
	}
	sitemap, sitemapBody := getEventually(t, client, server.URL+"/sitemap.xml", "http://catalog.example.test"+newAggregate)
	if sitemap.StatusCode != http.StatusOK || strings.Contains(sitemapBody, "/applications/legacy-motion") {
		t.Fatalf("sitemap status=%d body=%s", sitemap.StatusCode, sitemapBody)
	}
	changed, err := store.Product(t.Context(), product.ID)
	if err != nil || changed.Revision != product.Revision+1 {
		t.Fatalf("product revision = %+v, %v", changed, err)
	}
	if owner.ID == "" {
		t.Fatal("owner ID missing")
	}
}
