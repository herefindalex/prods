package webapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prods/internal/bulkops"
	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestProductBulkFixedPreflightPartialResultsReplayAndRevocation(t *testing.T) {
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
		OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	products := make([]catalog.Product, 3)
	for index := range products {
		products[index], err = store.CreateProduct(t.Context(), owner.ID, catalog.Product{
			PartNumber: fmt.Sprintf("BULK-%d", index+1), Name: "Bulk product",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	category, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{Name: "Bulk category", Slug: "bulk-category"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := store.CreateDictionaryEntry(t.Context(), owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryLifecycle, Name: "Bulk lifecycle", Slug: "bulk-lifecycle",
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
	defer server.Close()
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	preflight := postAdminJSON(t, client, server.URL+"/admin/api/product-bulk/preflight", csrf,
		fmt.Sprintf(`{"product_ids":[%q,%q,%q],"action":"publish"}`, products[0].ID, products[1].ID, products[2].ID))
	if preflight.StatusCode != http.StatusCreated {
		t.Fatalf("publish preflight status=%d body=%s", preflight.StatusCode, responseBody(t, preflight))
	}
	var publishRun bulkops.Run
	decodeResponseJSON(t, preflight, &publishRun)
	if publishRun.Preview.SelectionCount != 3 || publishRun.Preview.EligibleCount != 3 {
		t.Fatalf("fixed selection preview=%+v", publishRun.Preview)
	}
	concurrent := products[1]
	concurrent.Name = "Concurrent edit"
	if _, err := store.UpdateProduct(t.Context(), owner.ID, concurrent.Revision, concurrent); err != nil {
		t.Fatal(err)
	}
	executed := postAdminJSON(t, client, server.URL+"/admin/api/product-bulk/"+publishRun.Preview.RunID+"/execute", csrf, `{}`)
	if executed.StatusCode != http.StatusOK {
		t.Fatalf("publish execute status=%d body=%s", executed.StatusCode, responseBody(t, executed))
	}
	var publishReceipt bulkops.Receipt
	decodeResponseJSON(t, executed, &publishReceipt)
	if publishReceipt.Succeeded != 2 || publishReceipt.Conflicts != 1 || len(publishReceipt.Results) != 3 {
		t.Fatalf("per-Product partial receipt=%+v", publishReceipt)
	}
	waitForPublicBody(t, client, server.URL+"/products/"+products[0].Slug, products[0].PartNumber)
	waitForPublicBody(t, client, server.URL+"/products/"+products[2].Slug, products[2].PartNumber)
	replayed := postAdminJSON(t, client, server.URL+"/admin/api/product-bulk/"+publishRun.Preview.RunID+"/execute", csrf, `{}`)
	if replayed.StatusCode != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", replayed.StatusCode, responseBody(t, replayed))
	}
	var replayReceipt bulkops.Receipt
	decodeResponseJSON(t, replayed, &replayReceipt)
	if !replayReceipt.Replay || replayReceipt.Succeeded != publishReceipt.Succeeded || replayReceipt.Conflicts != publishReceipt.Conflicts {
		t.Fatalf("durable Bulk replay=%+v", replayReceipt)
	}

	productURL := server.URL + "/products/" + products[0].Slug
	publicResponse, err := client.Get(productURL)
	if err != nil {
		t.Fatal(err)
	}
	etag := publicResponse.Header.Get("ETag")
	publicResponse.Body.Close()
	hideRun := prepareBulkRun(t, client, server.URL, csrf, bulkops.ActionHide, "", products[0].ID, products[2].ID)
	hideReceipt := executeBulkRun(t, client, server.URL, csrf, hideRun.Preview.RunID)
	if hideReceipt.Succeeded != 2 {
		t.Fatalf("Hide receipt=%+v", hideReceipt)
	}
	request, err := http.NewRequest(http.MethodGet, productURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", etag)
	request.Header.Set("Range", "bytes=0-40")
	revoked, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	revoked.Body.Close()
	if revoked.StatusCode == http.StatusOK || revoked.StatusCode == http.StatusPartialContent || revoked.StatusCode == http.StatusNotModified {
		t.Fatalf("Bulk Hide admitted stale ETag/Range request: %d", revoked.StatusCode)
	}

	categoryRun := prepareBulkRun(t, client, server.URL, csrf, bulkops.ActionChangeCategory, category.ID, products[0].ID, products[2].ID)
	firstPlan := categoryRun.Preview.Items[0]
	if _, changed, err := store.ApplyProductBulkEdit(t.Context(), owner.ID, firstPlan.ProductID, firstPlan.ExpectedRevision, bulkops.ActionChangeCategory, category.ID); err != nil || !changed {
		t.Fatalf("simulate interrupted first item changed=%t err=%v", changed, err)
	}
	categoryReceipt := executeBulkRun(t, client, server.URL, csrf, categoryRun.Preview.RunID)
	if categoryReceipt.Succeeded != 2 || categoryReceipt.Conflicts != 0 {
		t.Fatalf("restart reconciliation receipt=%+v", categoryReceipt)
	}

	lifecycleRun := prepareBulkRun(t, client, server.URL, csrf, bulkops.ActionChangeLifecycle, lifecycle.ID, products[0].ID, products[2].ID)
	if receipt := executeBulkRun(t, client, server.URL, csrf, lifecycleRun.Preview.RunID); receipt.Succeeded != 2 {
		t.Fatalf("lifecycle receipt=%+v", receipt)
	}
	archiveRun := prepareBulkRun(t, client, server.URL, csrf, bulkops.ActionArchive, "", products[0].ID, products[2].ID)
	if archiveRun.Preview.Warning == "" {
		t.Fatal("Archive preflight omitted the read-only/revocation/import warning")
	}
	if receipt := executeBulkRun(t, client, server.URL, csrf, archiveRun.Preview.RunID); receipt.Succeeded != 2 {
		t.Fatalf("Archive receipt=%+v", receipt)
	}
	for _, index := range []int{0, 2} {
		archived, err := store.Product(t.Context(), products[index].ID)
		if err != nil || archived.RecordState != catalog.RecordArchived || archived.Status != catalog.Hidden {
			t.Fatalf("archived Product=%+v err=%v", archived, err)
		}
	}
	ineligible := prepareBulkRun(t, client, server.URL, csrf, bulkops.ActionPublish, "", products[0].ID)
	if ineligible.Preview.EligibleCount != 0 || ineligible.Preview.Items[0].Message == "" {
		t.Fatalf("Archived Product preflight=%+v", ineligible.Preview)
	}
	if receipt := executeBulkRun(t, client, server.URL, csrf, ineligible.Preview.RunID); receipt.Invalid != 1 {
		t.Fatalf("Archived Product execution=%+v", receipt)
	}
}

func prepareBulkRun(t *testing.T, client *http.Client, baseURL, csrf string, action bulkops.Action, targetID string, productIDs ...string) bulkops.Run {
	t.Helper()
	encodedIDs := ""
	for index, id := range productIDs {
		if index > 0 {
			encodedIDs += ","
		}
		encodedIDs += fmt.Sprintf("%q", id)
	}
	response := postAdminJSON(t, client, baseURL+"/admin/api/product-bulk/preflight", csrf,
		fmt.Sprintf(`{"product_ids":[%s],"action":%q,"target_id":%q}`, encodedIDs, action, targetID))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("Bulk preflight status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var run bulkops.Run
	decodeResponseJSON(t, response, &run)
	return run
}

func executeBulkRun(t *testing.T, client *http.Client, baseURL, csrf, runID string) bulkops.Receipt {
	t.Helper()
	response := postAdminJSON(t, client, baseURL+"/admin/api/product-bulk/"+runID+"/execute", csrf, `{}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Bulk execute status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var receipt bulkops.Receipt
	decodeResponseJSON(t, response, &receipt)
	return receipt
}
