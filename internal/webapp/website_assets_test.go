package webapp

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestWebsiteImageUploadValidationPreviewAndPublicAuthorization(t *testing.T) {
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

	response := uploadWebsiteAssetRequest(t, http.DefaultClient, server.URL, csrf, "anonymous.png", []byte("not important"))
	if body := responseBody(t, response); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous upload status=%d body=%s", response.StatusCode, body)
	}
	response = uploadWebsiteAssetRequest(t, client, server.URL, csrf, "fake.png", []byte("not a PNG"))
	if body := responseBody(t, response); response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("fake PNG status=%d body=%s", response.StatusCode, body)
	}
	unsafeSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	response = uploadWebsiteAssetRequest(t, client, server.URL, csrf, "unsafe.svg", unsafeSVG)
	if body := responseBody(t, response); response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe SVG status=%d body=%s", response.StatusCode, body)
	}
	safeSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><title>Logo</title><path fill="#123456" d="M0 0h10v10H0z"/></svg>`)
	response = uploadWebsiteAssetRequest(t, client, server.URL, csrf, "safe.svg", safeSVG)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("safe SVG status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var svgAsset catalog.Asset
	decodeResponseJSON(t, response, &svgAsset)
	if svgAsset.MIMEType != "image/svg+xml" {
		t.Fatalf("SVG asset=%+v", svgAsset)
	}

	pngBody := encodeTestPNG(t, 16, 12)
	response = uploadWebsiteAssetRequest(t, client, server.URL, csrf, "logo.png", pngBody)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("PNG status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var logo catalog.Asset
	decodeResponseJSON(t, response, &logo)
	publicAssetURL := server.URL + "/assets/" + logo.ID
	before, err := client.Get(publicAssetURL)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, before); before.StatusCode != http.StatusNotFound || strings.Contains(body, "PNG") {
		t.Fatalf("unpublished website asset leaked status=%d body=%s", before.StatusCode, body)
	}

	oversizeDimensions := encodeTestPNG(t, maxWebsiteImageSide+1, 1)
	response = uploadWebsiteAssetRequest(t, client, server.URL, csrf, "wide.png", oversizeDimensions)
	if body := responseBody(t, response); response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("oversize dimensions status=%d body=%s", response.StatusCode, body)
	}

	state, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state.Working.Organization.DisplayName = "Logo Company"
	state.Working.Organization.PrimaryLogoAsset = logo.ID
	encoded, err := json.Marshal(map[string]any{"expected_revision": state.WorkingRevision, "configuration": state.Working})
	if err != nil {
		t.Fatal(err)
	}
	response = adminJSONRequest(t, client, http.MethodPut, server.URL+"/admin/api/website/configuration", csrf, string(encoded))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("save logo status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	response = postAdminJSON(t, client, server.URL+"/admin/api/website/routes/publish", csrf,
		`{"expected_epoch":1,"expected_working_revision":2,"config":{"product_prefix":"/products","url_pattern":"compact"}}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("publish logo status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	after, err := client.Get(publicAssetURL)
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, after); after.StatusCode != http.StatusOK || !bytes.Equal([]byte(body), pngBody) || after.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("published website asset status=%d type=%q", after.StatusCode, after.Header.Get("Content-Type"))
	}
	search, err := client.Get(server.URL + "/search")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, search); !strings.Contains(body, `/assets/`+logo.ID) || !strings.Contains(body, "Logo Company") {
		t.Fatalf("published logo missing from dynamic public page: %s", body)
	}
}

func encodeTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	imageValue := image.NewRGBA(image.Rect(0, 0, width, height))
	imageValue.Set(0, 0, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	var output bytes.Buffer
	if err := png.Encode(&output, imageValue); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func uploadWebsiteAssetRequest(t *testing.T, client *http.Client, baseURL, csrf, filename string, body []byte) *http.Response {
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
	request, err := http.NewRequest(http.MethodPost, baseURL+"/admin/api/website/assets", &encoded)
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
