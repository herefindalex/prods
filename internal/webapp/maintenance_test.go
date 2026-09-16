package webapp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestMaintenanceAPIPreservesCategoryAndDisabledReferenceContracts(t *testing.T) {
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
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
	app, _, err := New(store, Config{BaseURL: "http://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	parent := postAdminJSON(t, client, server.URL+"/admin/api/categories", csrf,
		`{"id":"cat_power","name":"Power","slug":"power","status":"active","revision":1}`)
	if parent.StatusCode != http.StatusCreated {
		t.Fatalf("create parent status=%d body=%s", parent.StatusCode, responseBody(t, parent))
	}
	var parentCategory catalog.Category
	decodeResponseJSON(t, parent, &parentCategory)

	child := postAdminJSON(t, client, server.URL+"/admin/api/categories", csrf,
		`{"id":"cat_regulators","parent_id":"cat_power","name":"Regulators","slug":"regulators","status":"active","revision":1}`)
	if child.StatusCode != http.StatusCreated {
		t.Fatalf("create child status=%d body=%s", child.StatusCode, responseBody(t, child))
	}

	cycle := postAdminJSON(t, client, server.URL+"/admin/api/categories/cat_power/move", csrf,
		`{"expected_revision":1,"parent_id":"cat_regulators"}`)
	if body := responseBody(t, cycle); cycle.StatusCode != http.StatusConflict || !strings.Contains(body, `"code":"category_cycle"`) {
		t.Fatalf("category cycle status=%d body=%s", cycle.StatusCode, body)
	}

	maker := postAdminJSON(t, client, server.URL+"/admin/api/dictionaries", csrf,
		`{"id":"dic_acme","kind":"manufacturer","name":"Acme","slug":"acme","status":"active","revision":1}`)
	if maker.StatusCode != http.StatusCreated {
		t.Fatalf("create dictionary status=%d body=%s", maker.StatusCode, responseBody(t, maker))
	}
	var manufacturer catalog.DictionaryEntry
	decodeResponseJSON(t, maker, &manufacturer)

	createdProduct := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"ACME-1","manufacturer_id":"dic_acme","category_id":"cat_regulators","status":"hidden"}`)
	if createdProduct.StatusCode != http.StatusCreated {
		t.Fatalf("create product status=%d body=%s", createdProduct.StatusCode, responseBody(t, createdProduct))
	}
	var product catalog.Product
	decodeResponseJSON(t, createdProduct, &product)

	specResponse := postAdminJSON(t, client, server.URL+"/admin/api/specs", csrf,
		`{"id":"spec_voltage","name":"Input voltage","preferred_unit":"V","filterable":true,"semantic_version":1,"status":"active","revision":1}`)
	if specResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create spec status=%d body=%s", specResponse.StatusCode, responseBody(t, specResponse))
	}
	_ = responseBody(t, specResponse)
	specSetResponse := postAdminJSON(t, client, server.URL+"/admin/api/spec-sets", csrf,
		`{"id":"set_regulators","name":"Regulator specs","status":"active","revision":1,"spec_ids":["spec_voltage"]}`)
	if specSetResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create spec set status=%d body=%s", specSetResponse.StatusCode, responseBody(t, specSetResponse))
	}
	_ = responseBody(t, specSetResponse)
	assignment := postAdminJSON(t, client, server.URL+"/admin/api/categories/cat_regulators/spec-set", csrf,
		`{"expected_revision":1,"spec_set_id":"set_regulators"}`)
	if body := responseBody(t, assignment); assignment.StatusCode != http.StatusNoContent {
		t.Fatalf("assign spec set status=%d body=%s", assignment.StatusCode, body)
	}
	valueResponse := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/spec-values", csrf,
		`{"expected_revision":2,"spec_id":"spec_voltage","raw_value":"3.0–3.6","source_locale":"en-US"}`)
	if valueResponse.StatusCode != http.StatusOK {
		t.Fatalf("save spec value status=%d body=%s", valueResponse.StatusCode, responseBody(t, valueResponse))
	}
	var value catalog.SpecValue
	decodeResponseJSON(t, valueResponse, &value)
	if !value.Active || value.SourceRevision != 1 {
		t.Fatalf("saved spec value = %#v", value)
	}

	documentType := postAdminJSON(t, client, server.URL+"/admin/api/dictionaries", csrf,
		`{"id":"dic_datasheet","kind":"document_type","name":"Datasheet","slug":"datasheet","status":"active","revision":1}`)
	if documentType.StatusCode != http.StatusCreated {
		t.Fatalf("create document type status=%d body=%s", documentType.StatusCode, responseBody(t, documentType))
	}
	_ = responseBody(t, documentType)
	documentResponse := postAdminJSON(t, client, server.URL+"/admin/api/products/"+product.ID+"/documents", csrf,
		`{"expected_revision":3,"document":{"id":"doc_acme_1","label":"Datasheet","document_type_id":"dic_datasheet","external_url":"https://example.test/acme-1.pdf","language":"en-US","sort_order":1}}`)
	if documentResponse.StatusCode != http.StatusCreated {
		t.Fatalf("add document status=%d body=%s", documentResponse.StatusCode, responseBody(t, documentResponse))
	}
	_ = responseBody(t, documentResponse)

	setsResponse, err := client.Get(server.URL + "/admin/api/spec-sets")
	if err != nil {
		t.Fatal(err)
	}
	var sets []catalog.SpecSet
	decodeResponseJSON(t, setsResponse, &sets)
	if setsResponse.StatusCode != http.StatusOK || len(sets) != 1 || len(sets[0].SpecIDs) != 1 {
		t.Fatalf("spec sets response = %#v", sets)
	}
	assignmentResponse, err := client.Get(server.URL + "/admin/api/categories/cat_regulators/spec-set")
	if err != nil {
		t.Fatal(err)
	}
	var assignedSet catalog.SpecSet
	decodeResponseJSON(t, assignmentResponse, &assignedSet)
	if assignmentResponse.StatusCode != http.StatusOK || assignedSet.ID != "set_regulators" {
		t.Fatalf("category spec set response = %#v", assignedSet)
	}
	valuesResponse, err := client.Get(server.URL + "/admin/api/products/" + product.ID + "/spec-values")
	if err != nil {
		t.Fatal(err)
	}
	var values []catalog.SpecValueDetail
	decodeResponseJSON(t, valuesResponse, &values)
	if valuesResponse.StatusCode != http.StatusOK || len(values) != 1 || values[0].Value.RawValue != "3.0–3.6" {
		t.Fatalf("product spec values response = %#v", values)
	}
	documentsResponse, err := client.Get(server.URL + "/admin/api/products/" + product.ID + "/documents")
	if err != nil {
		t.Fatal(err)
	}
	var documents []catalog.ProductDocument
	decodeResponseJSON(t, documentsResponse, &documents)
	if documentsResponse.StatusCode != http.StatusOK || len(documents) != 1 || documents[0].ExternalURL == "" {
		t.Fatalf("product documents response = %#v", documents)
	}

	disable := postAdminJSON(t, client, server.URL+"/admin/api/dictionaries/dic_acme/disable", csrf,
		`{"expected_revision":1}`)
	if body := responseBody(t, disable); disable.StatusCode != http.StatusNoContent {
		t.Fatalf("disable dictionary status=%d body=%s", disable.StatusCode, body)
	}

	newReference := postAdminJSON(t, client, server.URL+"/admin/api/products", csrf,
		`{"part_number":"ACME-2","manufacturer_id":"dic_acme","category_id":"cat_regulators","status":"hidden"}`)
	if body := responseBody(t, newReference); newReference.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"disabled_reference"`) {
		t.Fatalf("disabled new reference status=%d body=%s", newReference.StatusCode, body)
	}

	list, err := client.Get(server.URL + "/admin/api/dictionaries?kind=manufacturer")
	if err != nil {
		t.Fatal(err)
	}
	if list.StatusCode != http.StatusOK {
		t.Fatalf("dictionary list status=%d body=%s", list.StatusCode, responseBody(t, list))
	}
	var entries []catalog.DictionaryEntry
	decodeResponseJSON(t, list, &entries)
	if len(entries) != 1 || entries[0].Status != catalog.EntryDisabled {
		t.Fatalf("disabled dictionary was not retained: %#v", entries)
	}

	categories, err := client.Get(server.URL + "/admin/api/categories")
	if err != nil {
		t.Fatal(err)
	}
	if categories.StatusCode != http.StatusOK {
		t.Fatalf("category list status=%d body=%s", categories.StatusCode, responseBody(t, categories))
	}
	var categoryRows []catalog.Category
	decodeResponseJSON(t, categories, &categoryRows)
	if len(categoryRows) < 4 || parentCategory.ID != "cat_power" {
		t.Fatalf("category hierarchy response = %#v", categoryRows)
	}
}
