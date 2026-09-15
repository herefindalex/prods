package webapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func TestPublicCopyAdminLifecycleAndPublicationBoundary(t *testing.T) {
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
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber: "COPY-API-1",
		Name:       "Copy API product",
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
	productURL := server.URL + "/products/" + product.Slug
	waitForPublicBody(t, client, productURL, "Request quote")

	type editorState struct {
		WorkingRevision int64                              `json:"working_revision"`
		Overrides       localization.PublicCopyOverrideMap `json:"overrides"`
		Catalog         localization.PublicCopyCatalog     `json:"catalog"`
		EnabledLocales  []string                           `json:"enabled_locales"`
	}
	response, err := client.Get(server.URL + "/admin/api/website/public-copy")
	if err != nil {
		t.Fatal(err)
	}
	var initial editorState
	decodeResponseJSON(t, response, &initial)
	if initial.WorkingRevision != 1 || len(initial.Catalog.Definitions) == 0 || len(initial.Catalog.Defaults) == 0 || len(initial.Catalog.Definitions[0].WhereUsed) == 0 {
		t.Fatalf("initial public copy editor state=%+v", initial)
	}

	revision := initial.WorkingRevision
	save := func(key, value string) {
		t.Helper()
		response := adminJSONMethod(t, client, http.MethodPut,
			server.URL+"/admin/api/website/public-copy/"+key+"/en-US", csrf,
			fmt.Sprintf(`{"expected_working_revision":%d,"value":%q,"definition_version":1}`, revision, value))
		if body := responseBody(t, response); response.StatusCode != http.StatusOK {
			t.Fatalf("save %s status=%d body=%s", key, response.StatusCode, body)
		}
		revision++
	}
	save("footer.terms", "Legal terms")
	save("product.request_quote", "Talk to sales")
	save("catalog.title", "Component Library")
	save("rfq.title", "Sales inquiry")

	reset := adminJSONMethod(t, client, http.MethodDelete,
		server.URL+"/admin/api/website/public-copy/footer.terms/en-US", csrf,
		fmt.Sprintf(`{"expected_working_revision":%d}`, revision))
	if body := responseBody(t, reset); reset.StatusCode != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", reset.StatusCode, body)
	}
	revision++

	invalid := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/website/public-copy/catalog.title/en-US", csrf,
		fmt.Sprintf(`{"expected_working_revision":%d,"value":"<b>invalid</b>","definition_version":1}`, revision))
	if body := responseBody(t, invalid); invalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid markup status=%d body=%s", invalid.StatusCode, body)
	}
	stale := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/website/public-copy/catalog.title/en-US", csrf,
		`{"expected_working_revision":1,"value":"stale","definition_version":1}`)
	if body := responseBody(t, stale); stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale revision status=%d body=%s", stale.StatusCode, body)
	}

	response, err = client.Get(server.URL + "/admin/api/website/public-copy")
	if err != nil {
		t.Fatal(err)
	}
	var working editorState
	decodeResponseJSON(t, response, &working)
	if working.WorkingRevision != revision {
		t.Fatalf("failed writes changed working revision: got=%d want=%d", working.WorkingRevision, revision)
	}
	if working.Overrides["footer.terms"] != nil {
		t.Fatalf("reset retained target override: %+v", working.Overrides["footer.terms"])
	}
	if working.Overrides["catalog.title"]["en-US"].Value != "Component Library" ||
		working.Overrides["product.request_quote"]["en-US"].Value != "Talk to sales" ||
		working.Overrides["rfq.title"]["en-US"].Value != "Sales inquiry" {
		t.Fatalf("reset or failed writes damaged other overrides: %+v", working.Overrides)
	}

	// The working copy must not leak through any public renderer before Publish.
	waitForPublicBody(t, client, productURL, "Request quote")
	waitForPublicBody(t, client, server.URL+"/search", "Catalog")
	waitForPublicBody(t, client, server.URL+"/rfq", "Request for quotation")

	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		fmt.Sprintf(`{"expected_epoch":1,"expected_working_revision":%d,"config":{"product_prefix":"/products","url_pattern":"compact"}}`, revision))
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", published.StatusCode, body)
	}
	waitForPublicBody(t, client, productURL, "Talk to sales")
	waitForPublicBody(t, client, productURL+".json", `"product.request_quote": "Talk to sales"`)
	waitForPublicBody(t, client, productURL+".md", "[Talk to sales]")
	waitForPublicBody(t, client, server.URL+"/search", "Component Library")
	waitForPublicBody(t, client, server.URL+"/rfq", "Sales inquiry")
}

func TestWebsiteVersionRestoreKeepsPublicCopyActiveUntilRepublished(t *testing.T) {
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
		OwnerEmail:       "owner@example.test",
		PasswordHash:     hash,
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
	defer server.Close()
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	saved := adminJSONMethod(t, client, http.MethodPut,
		server.URL+"/admin/api/website/public-copy/catalog.title/en-US", csrf,
		`{"expected_working_revision":1,"value":"Active copy","definition_version":1}`)
	if body := responseBody(t, saved); saved.StatusCode != http.StatusOK {
		t.Fatalf("save status=%d body=%s", saved.StatusCode, body)
	}
	published := postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if body := responseBody(t, published); published.StatusCode != http.StatusOK {
		t.Fatalf("publish status=%d body=%s", published.StatusCode, body)
	}
	restore := postAdminJSON(t, client, server.URL+"/admin/api/website/versions/restore", csrf,
		`{"expected_working_revision":2,"version":1}`)
	if restore.StatusCode != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restore.StatusCode, responseBody(t, restore))
	}
	var restored site.State
	decodeResponseJSON(t, restore, &restored)
	if restored.WorkingLocalization.PublicCopyOverrides["catalog.title"] != nil {
		t.Fatalf("restore did not create version-1 working copy: %+v", restored.WorkingLocalization)
	}
	if restored.ActiveLocalization.PublicCopyOverrides["catalog.title"]["en-US"].Value != "Active copy" {
		t.Fatalf("restore mutated active public copy without Publish: %+v", restored.ActiveLocalization)
	}
}
