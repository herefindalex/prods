package webapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/publishing"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func TestWebsiteLocaleChangesRequirePublishAndDisabledLocaleLeavesNoPublicResidue(t *testing.T) {
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
	if _, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
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
	defer server.Close()
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	created := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"WEB-LANG-1","name":"Source name","status":"published"}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create=%d %s", created.StatusCode, responseBody(t, created))
	}
	var product catalog.Product
	decodeResponseJSON(t, created, &product)
	productURL := server.URL + "/products/" + product.Slug
	waitForPublicBody(t, client, productURL, "WEB-LANG-1")
	if response, err := client.Get(productURL + "?lang=zh-TW"); err != nil || response.StatusCode != http.StatusBadRequest {
		if err == nil {
			response.Body.Close()
		}
		t.Fatalf("unpublished locale status=%v err=%v", response.StatusCode, err)
	}

	working := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/website/localization", csrf,
		`{"expected_working_revision":1,"localization":{"default_locale":"en-US","enabled_locales":["en-US","zh-TW"],"content_editing_enabled":true}}`)
	if working.StatusCode != http.StatusOK {
		t.Fatalf("save localization=%d %s", working.StatusCode, responseBody(t, working))
	}
	working.Body.Close()
	translated := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/products/"+product.ID+"/translations/zh-TW", csrf,
		fmt.Sprintf(`{"expected_revision":%d,"translation":{"name":"公開翻譯","features":"hidden-locale-needle"}}`, product.Revision))
	if translated.StatusCode != http.StatusOK {
		t.Fatalf("translation=%d %s", translated.StatusCode, responseBody(t, translated))
	}
	translated.Body.Close()
	if response, err := client.Get(productURL + "?lang=zh-TW"); err != nil || response.StatusCode != http.StatusBadRequest {
		if err == nil {
			response.Body.Close()
		}
		t.Fatalf("working locale leaked before publish status=%v err=%v", response.StatusCode, err)
	}

	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if published.StatusCode != http.StatusOK {
		t.Fatalf("publish locale=%d %s", published.StatusCode, responseBody(t, published))
	}
	published.Body.Close()
	localizedResponse, err := client.Get(productURL + "?lang=zh-TW")
	if err != nil {
		t.Fatal(err)
	}
	localizedETag := localizedResponse.Header.Get("ETag")
	if body := responseBody(t, localizedResponse); localizedResponse.StatusCode != http.StatusOK || !strings.Contains(body, "公開翻譯") {
		t.Fatalf("published locale=%d body=%s", localizedResponse.StatusCode, body)
	}

	disabled := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/website/localization", csrf,
		`{"expected_working_revision":2,"localization":{"default_locale":"en-US","enabled_locales":["en-US"],"content_editing_enabled":false}}`)
	if disabled.StatusCode != http.StatusOK {
		t.Fatalf("disable working locale=%d %s", disabled.StatusCode, responseBody(t, disabled))
	}
	disabled.Body.Close()
	published = postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":2,"expected_working_revision":3,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if published.StatusCode != http.StatusOK {
		t.Fatalf("publish disabled locale=%d %s", published.StatusCode, responseBody(t, published))
	}
	published.Body.Close()

	request, err := http.NewRequest(http.MethodGet, productURL+"?lang=zh-TW", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", localizedETag)
	request.Header.Set("Range", "bytes=0-40")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("disabled locale stale validator/range status=%d", response.StatusCode)
	}
	search, err := client.Get(server.URL + "/search?q=hidden-locale-needle")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, search); strings.Contains(body, product.Slug) {
		t.Fatalf("disabled locale search residue: %s", body)
	}
}

func TestPublishedTaxonomyUsesSameLocaleResolverAsProduct(t *testing.T) {
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
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	website, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveWebsiteLocalization(t.Context(), owner.ID, website.WorkingRevision, site.WebsiteLocalization{DefaultLocale: "en-US", EnabledLocales: []string{"en-US", "de-DE"}, ContentEditingEnabled: true}); err != nil {
		t.Fatal(err)
	}
	category, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{Name: "Power", Slug: "power"})
	if err != nil {
		t.Fatal(err)
	}
	translated, err := store.SaveTaxonomyTranslation(t.Context(), owner.ID, "category", category.ID, category.Revision, catalog.TaxonomyTranslation{Locale: "de-DE", Name: "Stromversorgung"})
	if err != nil || translated.SubjectRevision != category.Revision+1 {
		t.Fatalf("taxonomy translation=%+v err=%v", translated, err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "TAX-I18N-1", Name: "Controller", CategoryID: category.ID, Status: catalog.Published})
	if err != nil {
		t.Fatal(err)
	}

	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), AssetDir: filepath.Join(root, "assets"), WorkDir: filepath.Join(root, "work")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	defer server.Close()
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf, `{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish=%d %s", published.StatusCode, body)
	}
	waitForPublicBody(t, client, server.URL+"/products/"+product.Slug+"?lang=de-DE", "Stromversorgung")
}

func TestPublicCopyOverrideRequiresWebsitePublishAndReachesHTMLJSONAndMarkdown(t *testing.T) {
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
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "COPY-1", Name: "Copy product", Status: catalog.Published})
	if err != nil {
		t.Fatal(err)
	}
	website, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	overrides := localization.PublicCopyOverrideMap{
		"catalog.title":         {"en-US": {Key: "catalog.title", Locale: "en-US", Value: "Component Library", DefinitionVersion: 1}},
		"product.request_quote": {"en-US": {Key: "product.request_quote", Locale: "en-US", Value: "Talk to sales", DefinitionVersion: 1}},
	}
	if _, err := store.SaveWebsiteLocalization(t.Context(), owner.ID, website.WorkingRevision, site.WebsiteLocalization{DefaultLocale: "en-US", EnabledLocales: []string{"en-US"}, PublicCopyOverrides: overrides}); err != nil {
		t.Fatal(err)
	}
	preview, sources, err := store.PrepareSiteRoutes(t.Context(), publishing.SiteRouteConfig{ProductPrefix: "/products", Pattern: publishing.RouteCompact})
	if err != nil || len(sources) != 1 || sources[0].PublicCopyOverrides["catalog.title"]["en-US"].Value != "Component Library" || len(sources[0].PublicCopyDefaults) == 0 {
		t.Fatalf("prepared public copy preview=%+v sources=%+v err=%v", preview, sources, err)
	}

	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), AssetDir: filepath.Join(root, "assets"), WorkDir: filepath.Join(root, "work")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	defer server.Close()
	client := newCookieClient(t)
	productURL := server.URL + "/products/" + product.Slug
	waitForPublicBody(t, client, productURL, "Catalog")
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf, `{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish=%d %s", published.StatusCode, body)
	}
	waitForPublicBody(t, client, productURL, "Talk to sales")
	waitForPublicBody(t, client, server.URL+"/search", "Component Library")
	waitForPublicBody(t, client, productURL+".json", `"catalog.title": "Component Library"`)
	waitForPublicBody(t, client, productURL+".md", "[Talk to sales]")
}
