package sqlite

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/searchnotify"
)

func TestSearchIntegrationsReconcileDurableJobsReceiptsAndRemoval(t *testing.T) {
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.SearchIntegrationSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.IndexNowEnabled = true
	settings.GoogleEnabled = true
	settings.GoogleSiteURL = "https://catalog.example.test/"
	settings, err = store.UpdateSearchIntegrationSettings(t.Context(), owner.ID, settings.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.IndexNowKey) < 24 || settings.Revision != 2 {
		t.Fatalf("updated search settings=%+v", settings)
	}
	if _, err := store.UpdateSearchIntegrationSettings(t.Context(), owner.ID, 1, settings); !errors.Is(err, searchnotify.ErrSettingsConflict) {
		t.Fatalf("stale search settings error=%v", err)
	}

	publications := []publishing.ActivePublication{
		{ProductID: "p1", Route: "/products/one", SourceRevision: 1, SiteEpoch: 1, ManifestHash: "hash-one"},
		{ProductID: "p2", Route: "/products/two", SourceRevision: 1, SiteEpoch: 1, ManifestHash: "hash-two"},
	}
	created, err := store.ReconcileSearchSubmissions(t.Context(), "https://catalog.example.test", settings, publications)
	if err != nil || created != 2 {
		t.Fatalf("initial search reconcile created=%d err=%v", created, err)
	}
	now := time.Now().UTC().Add(time.Second)
	providers := map[string]bool{}
	for range 2 {
		job, found, err := store.ClaimSearchSubmission(t.Context(), now, true, true)
		if err != nil || !found {
			t.Fatalf("claim search job found=%v err=%v", found, err)
		}
		providers[job.Provider] = true
		if job.Provider == searchnotify.ProviderIndexNow && len(job.URLs) != 2 {
			t.Fatalf("IndexNow URLs=%v", job.URLs)
		}
		status := 202
		if job.Provider == searchnotify.ProviderGoogle {
			status = 204
		}
		if err := store.CompleteSearchSubmission(t.Context(), job.ID, status, "accepted, not indexed"); err != nil {
			t.Fatal(err)
		}
	}
	if !providers[searchnotify.ProviderIndexNow] || !providers[searchnotify.ProviderGoogle] {
		t.Fatalf("claimed providers=%v", providers)
	}
	if created, err := store.ReconcileSearchSubmissions(t.Context(), "https://catalog.example.test", settings, publications); err != nil || created != 0 {
		t.Fatalf("unchanged search reconcile created=%d err=%v", created, err)
	}

	publications = publications[:1]
	publications[0].SourceRevision = 2
	created, err = store.ReconcileSearchSubmissions(t.Context(), "https://catalog.example.test", settings, publications)
	if err != nil || created != 2 {
		t.Fatalf("update/removal search reconcile created=%d err=%v", created, err)
	}
	job, found, err := store.ClaimSearchSubmission(t.Context(), now.Add(time.Second), true, false)
	if err != nil || !found || job.Provider != searchnotify.ProviderIndexNow || len(job.URLs) != 2 {
		t.Fatalf("update/removal IndexNow job=%+v found=%v err=%v", job, found, err)
	}
	if err := store.FailSearchSubmission(t.Context(), job.ID, 503, "temporary outage", true, now); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimSearchSubmission(t.Context(), now.Add(4*time.Second), true, false); err != nil || found {
		t.Fatalf("backoff claim found=%v err=%v", found, err)
	}
	if retry, found, err := store.ClaimSearchSubmission(t.Context(), now.Add(6*time.Second), true, false); err != nil || !found || retry.ID != job.ID || retry.Attempts != 2 {
		t.Fatalf("retry claim=%+v found=%v err=%v", retry, found, err)
	}
	if err := store.ResetSearchSubmissionClaims(t.Context()); err != nil {
		t.Fatal(err)
	}
	if resumed, found, err := store.ClaimSearchSubmission(t.Context(), time.Now().UTC().Add(time.Second), true, false); err != nil || !found || resumed.ID != job.ID {
		t.Fatalf("restart claim=%+v found=%v err=%v", resumed, found, err)
	}
}

func TestDisablingSearchProviderCancelsPendingWorkAndRemovesPublicKey(t *testing.T) {
	store, owner := installedSearchStore(t)
	settings, err := store.SearchIntegrationSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.IndexNowEnabled = true
	settings, err = store.UpdateSearchIntegrationSettings(t.Context(), owner.ID, settings.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReconcileSearchSubmissions(t.Context(), "https://catalog.example.test", settings,
		[]publishing.ActivePublication{{ProductID: "p1", Route: "/products/one", SourceRevision: 1, SiteEpoch: 1, ManifestHash: "one"}}); err != nil {
		t.Fatal(err)
	}
	settings.IndexNowEnabled = false
	settings, err = store.UpdateSearchIntegrationSettings(t.Context(), owner.ID, settings.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimSearchSubmission(t.Context(), time.Now().UTC().Add(time.Hour), true, true); err != nil || found {
		t.Fatalf("disabled provider claim found=%v err=%v", found, err)
	}
}

func installedSearchStore(t *testing.T) (*Store, identity.User) {
	t.Helper()
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}
