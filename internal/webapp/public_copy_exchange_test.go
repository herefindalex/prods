package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestPublicCopyCSVXLSXExchangeValidatesPreviewsAndCommitsWorkingOnly(t *testing.T) {
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
	localizationResponse := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/website/localization", csrf,
		`{"expected_working_revision":1,"localization":{"default_locale":"en-US","enabled_locales":["en-US","de-DE"],"content_editing_enabled":false}}`)
	if body := responseBody(t, localizationResponse); localizationResponse.StatusCode != http.StatusOK {
		t.Fatalf("localization status=%d body=%s", localizationResponse.StatusCode, body)
	}
	catalogValue, err := store.PublicCopyCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	for _, format := range []string{"csv", "xlsx"} {
		request, err := http.NewRequest(http.MethodPost, server.URL+"/admin/api/website/public-copy/export/"+format+"?locales=en-US&scope=catalog", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-CSRF-Token", csrf)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || len(body) == 0 {
			t.Fatalf("%s export status=%d bytes=%d err=%v", format, response.StatusCode, len(body), err)
		}
		var rows []publicCopyExchangeRow
		if format == "csv" {
			rows, err = parsePublicCopyCSV(body)
		} else {
			rows, err = parsePublicCopyXLSX(body)
		}
		if err != nil || len(rows) == 0 || !strings.HasPrefix(rows[0].Key, "catalog.") {
			t.Fatalf("%s exported rows=%+v err=%v", format, rows, err)
		}
	}

	validCSV := fmt.Sprintf("Key,Locale,Value,Action,Definition Version,Official Bundle Version\n"+
		"catalog.title,en-US,Component Library,upsert,1,%s\n"+
		"rfq.title,de-DE,Verkaufsanfrage,upsert,1,%s\n", catalogValue.OfficialBundle, catalogValue.OfficialBundle)
	previewResponse := uploadPublicCopyExchange(t, client, server.URL, csrf, "copy.csv", []byte(validCSV))
	if previewResponse.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResponse.StatusCode, responseBody(t, previewResponse))
	}
	var preview publicCopyExchangePreview
	decodeResponseJSON(t, previewResponse, &preview)
	if !preview.FullyValidated || preview.WorkingRevision != 2 || len(preview.Changes) != 2 || len(preview.Rows) != 2 {
		t.Fatalf("valid preview=%+v", preview)
	}
	encodedRows, err := json.Marshal(preview.Rows)
	if err != nil {
		t.Fatal(err)
	}
	commit := adminJSONMethod(t, client, http.MethodPost, server.URL+"/admin/api/website/public-copy/import/commit", csrf,
		fmt.Sprintf(`{"expected_working_revision":2,"rows":%s}`, encodedRows))
	if body := responseBody(t, commit); commit.StatusCode != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", commit.StatusCode, body)
	}
	state, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.WorkingRevision != 3 || state.WorkingLocalization.PublicCopyOverrides["catalog.title"]["en-US"].Value != "Component Library" ||
		state.WorkingLocalization.PublicCopyOverrides["rfq.title"]["de-DE"].Value != "Verkaufsanfrage" {
		t.Fatalf("committed working overrides=%+v", state.WorkingLocalization.PublicCopyOverrides)
	}
	if state.ActiveLocalization.PublicCopyOverrides["catalog.title"] != nil {
		t.Fatalf("translation exchange published without Website Publish: %+v", state.ActiveLocalization.PublicCopyOverrides)
	}

	duplicateCSV := fmt.Sprintf("Key,Locale,Value,Action,Definition Version,Official Bundle Version\n"+
		"catalog.title,en-US,One,upsert,1,%s\n"+
		"catalog.title,en-US,Two,upsert,1,%s\n", catalogValue.OfficialBundle, catalogValue.OfficialBundle)
	duplicateResponse := uploadPublicCopyExchange(t, client, server.URL, csrf, "duplicate.csv", []byte(duplicateCSV))
	if duplicateResponse.StatusCode != http.StatusOK {
		t.Fatalf("duplicate preview status=%d body=%s", duplicateResponse.StatusCode, responseBody(t, duplicateResponse))
	}
	var duplicate publicCopyExchangePreview
	decodeResponseJSON(t, duplicateResponse, &duplicate)
	if duplicate.FullyValidated || len(duplicate.Issues) != 2 || duplicate.Issues[0].Code != "duplicate_entry" {
		t.Fatalf("duplicate exchange preview=%+v", duplicate)
	}

	invalidCSV := fmt.Sprintf("Key,Locale,Value,Action,Definition Version,Official Bundle Version\n"+
		"unknown.key,en-US,Value,upsert,1,%s\n"+
		"catalog.title,xx-XX,Value,upsert,1,%s\n"+
		"catalog.title,fr-FR,Value,upsert,1,%s\n"+
		"catalog.title,en-US,<b>unsafe</b>,upsert,999,old-bundle\n"+
		"rfq.title,en-US,Value,unsupported,1,%s\n"+
		"rfq.title,de-DE,Must be blank,reset,1,%s\n",
		catalogValue.OfficialBundle, catalogValue.OfficialBundle, catalogValue.OfficialBundle,
		catalogValue.OfficialBundle, catalogValue.OfficialBundle)
	invalidResponse := uploadPublicCopyExchange(t, client, server.URL, csrf, "invalid.csv", []byte(invalidCSV))
	if invalidResponse.StatusCode != http.StatusOK {
		t.Fatalf("invalid preview status=%d body=%s", invalidResponse.StatusCode, responseBody(t, invalidResponse))
	}
	var invalid publicCopyExchangePreview
	decodeResponseJSON(t, invalidResponse, &invalid)
	wantedIssueCodes := map[string]bool{
		"unknown_key": false, "unknown_locale": false, "disabled_locale": false,
		"resource_version_mismatch": false, "invalid_value": false,
		"invalid_action": false, "reset_value_present": false,
	}
	for _, issue := range invalid.Issues {
		if _, tracked := wantedIssueCodes[issue.Code]; tracked {
			wantedIssueCodes[issue.Code] = true
		}
	}
	if invalid.FullyValidated {
		t.Fatal("multi-error exchange unexpectedly validated")
	}
	for code, found := range wantedIssueCodes {
		if !found {
			t.Fatalf("aggregated exchange issues missing %q: %+v", code, invalid.Issues)
		}
	}

	stalePreviewResponse := uploadPublicCopyExchange(t, client, server.URL, csrf, "stale.csv", []byte(validCSV))
	var stalePreview publicCopyExchangePreview
	decodeResponseJSON(t, stalePreviewResponse, &stalePreview)
	advance := adminJSONMethod(t, client, http.MethodPut, server.URL+"/admin/api/website/public-copy/footer.terms/en-US", csrf,
		`{"expected_working_revision":3,"value":"Legal","definition_version":1}`)
	if body := responseBody(t, advance); advance.StatusCode != http.StatusOK {
		t.Fatalf("advance revision status=%d body=%s", advance.StatusCode, body)
	}
	encodedRows, _ = json.Marshal(stalePreview.Rows)
	staleCommit := adminJSONMethod(t, client, http.MethodPost, server.URL+"/admin/api/website/public-copy/import/commit", csrf,
		fmt.Sprintf(`{"expected_working_revision":%d,"rows":%s}`, stalePreview.WorkingRevision, encodedRows))
	if body := responseBody(t, staleCommit); staleCommit.StatusCode != http.StatusConflict {
		t.Fatalf("stale exchange commit status=%d body=%s", staleCommit.StatusCode, body)
	}
}

func uploadPublicCopyExchange(t *testing.T, client *http.Client, baseURL, csrf, filename string, content []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/admin/api/website/public-copy/import/preview", &body)
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
