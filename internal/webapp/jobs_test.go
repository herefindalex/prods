package webapp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

func TestAdminJobsListsDurableFailureAndSafelyRetriesPublication(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Create(filepath.Join(root, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
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
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber: "ADMIN-JOB-1",
		Name:       "Admin job retry",
		Status:     catalog.Published,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, found, err := store.ClaimPublicationIntent(t.Context())
	if err != nil || !found {
		t.Fatalf("claim publication intent found=%v err=%v", found, err)
	}
	if err := store.FailPublicationIntent(t.Context(), intent.ID, "fixture renderer failure"); err != nil {
		t.Fatal(err)
	}

	app, _, err := New(store, Config{
		BaseURL: "http://catalog.example.test", PublicDir: filepath.Join(root, "generated"), WorkDir: filepath.Join(root, "work"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	server := httptest.NewServer(app)
	defer server.Close()

	unauthorized, err := http.Get(server.URL + "/admin/api/jobs")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, unauthorized); unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized jobs status=%d body=%s", unauthorized.StatusCode, body)
	}

	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")
	invalidLimit, err := client.Get(server.URL + "/admin/api/jobs?limit=0")
	if err != nil {
		t.Fatal(err)
	}
	if body := responseBody(t, invalidLimit); invalidLimit.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid limit status=%d body=%s", invalidLimit.StatusCode, body)
	}
	response, err := client.Get(server.URL + "/admin/api/jobs?limit=20")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("jobs status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var listed adminJobsResponse
	decodeResponseJSON(t, response, &listed)
	failed := findAdminJob(t, listed.Jobs, intent.ID)
	if failed.Kind != "publication" || failed.Status != "failed" || failed.ErrorMessage != "fixture renderer failure" || !failed.Retryable {
		t.Fatalf("listed failure = %+v", failed)
	}
	if len(listed.RetryPolicy.SafeKinds) != 2 || listed.RetryPolicy.SafeKinds[0] != "publication" || listed.RetryPolicy.SafeKinds[1] != "search_submission" {
		t.Fatalf("retry policy = %+v", listed.RetryPolicy)
	}

	withoutCSRF := postAdminJSON(t, client, server.URL+"/admin/api/jobs/publication/"+intent.ID+"/retry", "", `{}`)
	if body := responseBody(t, withoutCSRF); withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("retry without CSRF status=%d body=%s", withoutCSRF.StatusCode, body)
	}
	retryResponse := postAdminJSON(t, client, server.URL+"/admin/api/jobs/publication/"+intent.ID+"/retry", csrf, `{}`)
	if retryResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", retryResponse.StatusCode, responseBody(t, retryResponse))
	}
	var retry sqlite.JobRecord
	decodeResponseJSON(t, retryResponse, &retry)
	if retry.ID == intent.ID || retry.TargetID != product.ID || retry.Status != "pending" {
		t.Fatalf("retry response = %+v", retry)
	}
	waitForPublicBody(t, client, server.URL+"/products/"+product.Slug, "ADMIN-JOB-1")

	duplicate := postAdminJSON(t, client, server.URL+"/admin/api/jobs/publication/"+intent.ID+"/retry", csrf, `{}`)
	if body := responseBody(t, duplicate); duplicate.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate retry status=%d body=%s", duplicate.StatusCode, body)
	}
}

func findAdminJob(t *testing.T, jobs []sqlite.JobRecord, id string) sqlite.JobRecord {
	t.Helper()
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	t.Fatalf("job %q not found in %+v", id, jobs)
	return sqlite.JobRecord{}
}
