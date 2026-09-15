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
	"prods/internal/publishing"
	"prods/internal/storage/sqlite"
)

func TestProductImagesSupportManagedExternalPrimaryFallbackAndRevocation(t *testing.T) {
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
	product := createPublishedProduct(t, client, server.URL, csrf, "IMAGE-1")

	upload := uploadProductAssetRequest(t, client, server.URL, csrf, product.ID, "front.png", encodeTestPNG(t, 16, 12))
	if upload.StatusCode != http.StatusCreated {
		t.Fatalf("upload image status=%d body=%s", upload.StatusCode, responseBody(t, upload))
	}
	var asset catalog.Asset
	decodeResponseJSON(t, upload, &asset)
	if asset.MIMEType != "image/png" {
		t.Fatalf("asset MIME = %s", asset.MIMEType)
	}
	added := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/images", csrf,
		`{"expected_revision":1,"image":{"asset_id":"`+asset.ID+`","sort_order":20,"primary":false}}`)
	if added.StatusCode != http.StatusCreated {
		t.Fatalf("add managed image status=%d body=%s", added.StatusCode, responseBody(t, added))
	}
	var managed catalog.ProductImage
	decodeResponseJSON(t, added, &managed)
	if !managed.Primary {
		t.Fatal("first image was not made primary")
	}

	externalURL := "https://cdn.example.test/product/side.webp"
	added = postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/images", csrf,
		`{"expected_revision":2,"image":{"external_url":"`+externalURL+`","alt_text":"Side view","sort_order":10,"primary":true}}`)
	if added.StatusCode != http.StatusCreated {
		t.Fatalf("add external image status=%d body=%s", added.StatusCode, responseBody(t, added))
	}
	var external catalog.ProductImage
	decodeResponseJSON(t, added, &external)

	_, html := getEventually(t, client, server.URL+"/products/"+product.Slug, externalURL)
	if !strings.Contains(html, `/assets/`+asset.ID) || !strings.Contains(html, `alt="IMAGE-1"`) || !strings.Contains(html, `alt="Side view"`) {
		t.Fatalf("published image HTML = %s", html)
	}
	_, jsonBody := getEventually(t, client, server.URL+"/products/"+product.Slug+".json", externalURL)
	var view publishing.PublicView
	if err := json.Unmarshal([]byte(jsonBody), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Images) != 2 || !view.Images[0].Primary || view.Images[0].URL != externalURL {
		t.Fatalf("public images = %+v", view.Images)
	}

	assetURL := server.URL + "/assets/" + asset.ID
	publicAsset, err := client.Get(assetURL)
	if err != nil {
		t.Fatal(err)
	}
	assetETag := publicAsset.Header.Get("ETag")
	if body := responseBody(t, publicAsset); publicAsset.StatusCode != http.StatusOK || assetETag == "" || len(body) == 0 {
		t.Fatalf("public image asset status=%d etag=%q", publicAsset.StatusCode, assetETag)
	}

	setPrimary := putAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/images/"+managed.ID, csrf,
		`{"expected_revision":3,"image":{"asset_id":"`+asset.ID+`","sort_order":20,"primary":true}}`)
	if setPrimary.StatusCode != http.StatusOK {
		t.Fatalf("set primary status=%d body=%s", setPrimary.StatusCode, responseBody(t, setPrimary))
	}
	_ = responseBody(t, setPrimary)

	deleted := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/images/"+managed.ID+"/delete", csrf, `{"expected_revision":4}`)
	if body := responseBody(t, deleted); deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("delete image status=%d body=%s", deleted.StatusCode, body)
	}
	_, jsonBody = getEventually(t, client, server.URL+"/products/"+product.Slug+".json", `"revision": 5`)
	if err := json.Unmarshal([]byte(jsonBody), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Images) != 1 || !view.Images[0].Primary || view.Images[0].ID != external.ID {
		t.Fatalf("images after delete = %+v", view.Images)
	}
	request, err := http.NewRequest(http.MethodGet, assetURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", assetETag)
	revoked, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, revoked); revoked.StatusCode == http.StatusOK || revoked.StatusCode == http.StatusPartialContent || revoked.StatusCode == http.StatusNotModified || strings.HasPrefix(body, "\x89PNG") {
		t.Fatalf("orphan image asset still public status=%d body=%q", revoked.StatusCode, body)
	}

	badExternal := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/images", csrf,
		`{"expected_revision":5,"image":{"external_url":"javascript:alert(1)","sort_order":1}}`)
	if body := responseBody(t, badExternal); badExternal.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad external URL status=%d body=%s", badExternal.StatusCode, body)
	}
	badSVG := uploadProductAssetRequest(t, client, server.URL, csrf, product.ID, "bad.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	if body := responseBody(t, badSVG); badSVG.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("product SVG status=%d body=%s", badSVG.StatusCode, body)
	}
}

func putAdminJSON(t *testing.T, client *http.Client, target, csrf, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPut, target, strings.NewReader(body))
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
