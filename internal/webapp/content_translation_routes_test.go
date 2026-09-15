package webapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestContentTranslationRoutesRevisionGateAndEditingLifecycle(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail:       "owner@example.test",
		PasswordHash:     hash,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US"},
		TimeZone:         "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{Name: "Power", Slug: "power"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{
		ParentID: parent.ID,
		Name:     "Controllers",
		Slug:     "controllers",
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber: "CONTENT-I18N-1",
		Name:       "Source controller",
		CategoryID: child.ID,
		Status:     catalog.Published,
	})
	if err != nil {
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
	defer server.Close()
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	localization := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/website/localization", csrf,
		`{"expected_working_revision":1,"localization":{"default_locale":"en-US","enabled_locales":["en-US","de-DE"],"content_editing_enabled":true}}`)
	if body := responseBody(t, localization); localization.StatusCode != http.StatusOK {
		t.Fatalf("enable editing status=%d body=%s", localization.StatusCode, body)
	}

	productSource := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/products/"+product.ID+"/content/source-locales", csrf,
		`{"expected_revision":1,"source_locale":"en-US","source_locales":{"name":"en-US","description":"en-US","features":"en-US","specification":"en-US"}}`)
	if productSource.StatusCode != http.StatusOK {
		t.Fatalf("Product source locale status=%d body=%s", productSource.StatusCode, responseBody(t, productSource))
	}
	var productContent catalog.ProductContent
	decodeResponseJSON(t, productSource, &productContent)
	if productContent.ProductRevision != 2 || productContent.SourceLocales["name"] != "en-US" {
		t.Fatalf("Product content=%+v", productContent)
	}
	staleProduct := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/products/"+product.ID+"/content/source-locales", csrf,
		`{"expected_revision":1,"source_locale":"de-DE","source_locales":{}}`)
	if body := responseBody(t, staleProduct); staleProduct.StatusCode != http.StatusConflict {
		t.Fatalf("stale Product source locale status=%d body=%s", staleProduct.StatusCode, body)
	}
	translation := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/products/"+product.ID+"/translations/de-DE", csrf,
		`{"expected_revision":2,"translation":{"name":"Übersetzter Regler","description":"Produktbeschreibung"}}`)
	if body := responseBody(t, translation); translation.StatusCode != http.StatusOK {
		t.Fatalf("Product translation status=%d body=%s", translation.StatusCode, body)
	}

	parentSource := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/taxonomy/category/"+parent.ID+"/content/source-locales", csrf,
		`{"expected_revision":1,"source_locale":"en-US","source_locales":{"name":"en-US","description":"en-US"}}`)
	if parentSource.StatusCode != http.StatusOK {
		t.Fatalf("taxonomy source locale status=%d body=%s", parentSource.StatusCode, responseBody(t, parentSource))
	}
	var taxonomyContent catalog.TaxonomyContent
	decodeResponseJSON(t, parentSource, &taxonomyContent)
	if taxonomyContent.SubjectRevision != 2 || taxonomyContent.SourceLocales["description"] != "en-US" {
		t.Fatalf("taxonomy content=%+v", taxonomyContent)
	}
	taxonomyTranslation := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/taxonomy/category/"+parent.ID+"/translations/de-DE", csrf,
		`{"expected_revision":2,"translation":{"name":"Leistung"}}`)
	if body := responseBody(t, taxonomyTranslation); taxonomyTranslation.StatusCode != http.StatusOK {
		t.Fatalf("taxonomy translation status=%d body=%s", taxonomyTranslation.StatusCode, body)
	}

	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", published.StatusCode, body)
	}
	baseProductURL := server.URL + "/products/" + product.Slug
	productURL := baseProductURL + "?lang=de-DE"
	waitForPublicBody(t, client, productURL, "Übersetzter Regler")
	waitForPublicBody(t, client, baseProductURL+".json?lang=de-DE", "Leistung")

	// Updating an ancestor taxonomy translation must requeue descendant Products.
	taxonomyTranslation = adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/taxonomy/category/"+parent.ID+"/translations/de-DE", csrf,
		`{"expected_revision":3,"translation":{"name":"Energie"}}`)
	if body := responseBody(t, taxonomyTranslation); taxonomyTranslation.StatusCode != http.StatusOK {
		t.Fatalf("updated taxonomy translation status=%d body=%s", taxonomyTranslation.StatusCode, body)
	}
	waitForPublicBody(t, client, baseProductURL+".json?lang=de-DE", "Energie")

	disabled := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/website/localization", csrf,
		`{"expected_working_revision":2,"localization":{"default_locale":"en-US","enabled_locales":["en-US","de-DE"],"content_editing_enabled":false}}`)
	if body := responseBody(t, disabled); disabled.StatusCode != http.StatusOK {
		t.Fatalf("disable editing status=%d body=%s", disabled.StatusCode, body)
	}
	currentProduct, err := store.Product(t.Context(), product.ID)
	if err != nil {
		t.Fatal(err)
	}
	currentParent, err := store.Category(t.Context(), parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	blockedProduct := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/products/"+product.ID+"/translations/de-DE", csrf,
		fmt.Sprintf(`{"expected_revision":%d,"translation":{"name":"Must not save"}}`, currentProduct.Revision))
	if body := responseBody(t, blockedProduct); blockedProduct.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("disabled Product editing status=%d body=%s", blockedProduct.StatusCode, body)
	}
	blockedTaxonomy := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/taxonomy/category/"+parent.ID+"/translations/de-DE", csrf,
		fmt.Sprintf(`{"expected_revision":%d,"translation":{"name":"Must not save"}}`, currentParent.Revision))
	if body := responseBody(t, blockedTaxonomy); blockedTaxonomy.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("disabled taxonomy editing status=%d body=%s", blockedTaxonomy.StatusCode, body)
	}

	// Editing off is an authoring gate only. It must not unpublish stored content.
	waitForPublicBody(t, client, productURL, "Übersetzter Regler")
	published = postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":2,"expected_working_revision":3,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish editing-off Website status=%d body=%s", published.StatusCode, body)
	}
	waitForPublicBody(t, client, productURL, "Übersetzter Regler")
	waitForPublicBody(t, client, baseProductURL+".json?lang=de-DE", "Energie")
}
