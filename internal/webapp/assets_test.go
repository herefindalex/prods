package webapp

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestSharedPublicAssetAuthorizationTracksActiveProductReferences(t *testing.T) {
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

	documentTypeResponse := postAdminJSON(t, client, server.URL+"/admin/api/dictionaries", csrf,
		`{"kind":"document_type","name":"Datasheet","slug":"datasheet","status":"active","revision":1}`)
	if documentTypeResponse.StatusCode != http.StatusCreated {
		t.Fatalf("document type status=%d body=%s", documentTypeResponse.StatusCode, responseBody(t, documentTypeResponse))
	}
	var documentType catalog.DictionaryEntry
	decodeResponseJSON(t, documentTypeResponse, &documentType)

	first := createPublishedProduct(t, client, server.URL, csrf, "SHARED-A")
	pdf := []byte("%PDF-1.7\n% synthetic shared test document\n%%EOF\n")
	asset := uploadProductPDF(t, client, server.URL, csrf, first.ID, "shared.pdf", pdf)
	addProductAssetDocument(t, client, server.URL, csrf, first.ID, first.Revision, documentType.ID, asset.ID)
	waitForPublicBody(t, client, server.URL+"/products/"+first.Slug, asset.ID)

	second := createPublishedProduct(t, client, server.URL, csrf, "SHARED-B")
	addProductAssetDocument(t, client, server.URL, csrf, second.ID, second.Revision, documentType.ID, asset.ID)
	waitForPublicBody(t, client, server.URL+"/products/"+second.Slug, asset.ID)

	assetURL := server.URL + "/assets/" + asset.ID
	assetResponse, err := client.Get(assetURL)
	if err != nil {
		t.Fatal(err)
	}
	assetETag := assetResponse.Header.Get("ETag")
	if body := responseBody(t, assetResponse); assetResponse.StatusCode != http.StatusOK || body != string(pdf) || assetETag == "" {
		t.Fatalf("asset status=%d etag=%q body=%q", assetResponse.StatusCode, assetETag, body)
	}

	hideFirst := postAdminJSON(t, client, server.URL+"/admin/api/products/"+first.ID+"/hide", csrf, `{"expected_revision":2}`)
	if body := responseBody(t, hideFirst); hideFirst.StatusCode != http.StatusNoContent {
		t.Fatalf("hide first status=%d body=%s", hideFirst.StatusCode, body)
	}
	stillShared, err := client.Get(assetURL)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, stillShared); stillShared.StatusCode != http.StatusOK || body != string(pdf) {
		t.Fatalf("shared asset was revoked early status=%d body=%q", stillShared.StatusCode, body)
	}

	hideSecond := postAdminJSON(t, client, server.URL+"/admin/api/products/"+second.ID+"/hide", csrf, `{"expected_revision":2}`)
	if body := responseBody(t, hideSecond); hideSecond.StatusCode != http.StatusNoContent {
		t.Fatalf("hide second status=%d body=%s", hideSecond.StatusCode, body)
	}
	for _, headers := range []map[string]string{
		{"If-None-Match": assetETag},
		{"Range": "bytes=0-9", "If-Range": assetETag},
	} {
		request, err := http.NewRequest(http.MethodGet, assetURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body := responseBody(t, response)
		if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusPartialContent || response.StatusCode == http.StatusNotModified || strings.Contains(body, "%PDF") {
			t.Fatalf("revoked asset admitted status=%d body=%q", response.StatusCode, body)
		}
	}
}

func TestProductAssetUploadRejectsInvalidOversizeAndAnonymousRequests(t *testing.T) {
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
	product := createPublishedProduct(t, client, server.URL, csrf, "UPLOAD-VALIDATION")

	validPDF := []byte("%PDF-1.7\n%%EOF\n")
	for _, test := range []struct {
		name     string
		filename string
		body     []byte
		want     int
	}{
		{name: "extension", filename: "not-a-pdf.txt", body: validPDF, want: http.StatusUnprocessableEntity},
		{name: "magic", filename: "fake.pdf", body: []byte("plain text"), want: http.StatusUnprocessableEntity},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := uploadProductAssetRequest(t, client, server.URL, csrf, product.ID, test.filename, test.body)
			defer response.Body.Close()
			if response.StatusCode != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.StatusCode, test.want, responseBody(t, response))
			}
		})
	}

	oversize := make([]byte, maxProductDocumentBytes+1)
	copy(oversize, validPDF)
	response := uploadProductAssetRequest(t, client, server.URL, csrf, product.ID, "oversize.pdf", oversize)
	if body := responseBody(t, response); response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status=%d body=%s", response.StatusCode, body)
	}

	response = uploadProductAssetRequest(t, http.DefaultClient, server.URL, csrf, product.ID, "anonymous.pdf", validPDF)
	if body := responseBody(t, response); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d body=%s", response.StatusCode, body)
	}
}

func createPublishedProduct(t *testing.T, client *http.Client, baseURL, csrf, partNumber string) catalog.Product {
	t.Helper()
	response := postAdminJSON(t, client, baseURL+"/admin/api/products", csrf,
		`{"part_number":"`+partNumber+`","name":"`+partNumber+`","status":"published"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var product catalog.Product
	decodeResponseJSON(t, response, &product)
	return product
}

func uploadProductPDF(t *testing.T, client *http.Client, baseURL, csrf, productID, filename string, body []byte) catalog.Asset {
	t.Helper()
	response := uploadProductAssetRequest(t, client, baseURL, csrf, productID, filename, body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var asset catalog.Asset
	decodeResponseJSON(t, response, &asset)
	return asset
}

func uploadProductAssetRequest(t *testing.T, client *http.Client, baseURL, csrf, productID, filename string, body []byte) *http.Response {
	t.Helper()
	var encoded bytes.Buffer
	writer := multipart.NewWriter(&encoded)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/admin/api/products/"+productID+"/assets", &encoded)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func addProductAssetDocument(t *testing.T, client *http.Client, baseURL, csrf, productID string, revision int64, documentTypeID, assetID string) {
	t.Helper()
	body := `{"expected_revision":` + fmtInt(revision) + `,"document":{"label":"Datasheet","document_type_id":"` + documentTypeID + `","asset_id":"` + assetID + `","language":"en","sort_order":0}}`
	response := postAdminJSON(t, client, baseURL+"/admin/api/products/"+productID+"/documents", csrf, body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("add document status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
}

func fmtInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
