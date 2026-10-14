package sqlite

import (
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/publishing"
)

func TestSpecProjectionOldRebuildCannotReplaceNewActivation(t *testing.T) {
	store, owner := installedStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "SPEC-REBUILD", Status: catalog.Published})
	if err != nil {
		t.Fatal(err)
	}
	activate := func(product catalog.Product) {
		t.Helper()
		intent, found, err := store.ClaimPublicationIntent(t.Context())
		if err != nil || !found || intent.DesiredRevision != product.Revision {
			t.Fatalf("intent=%+v found=%v err=%v", intent, found, err)
		}
		view, err := publishing.FromProduct(product, "https://catalog.example.test")
		if err != nil {
			t.Fatal(err)
		}
		view.SiteEpoch = 1
		activation := publishing.ActivePublication{ProductID: product.ID, SourceRevision: product.Revision, PublicRevision: product.Revision, SiteEpoch: 1, Route: "/products/" + product.Slug, ArtifactID: "art_" + product.ID, ManifestHash: "hash_" + product.ID, ActivatedAt: time.Now().UTC(), View: view}
		activated, err := store.ActivatePublication(t.Context(), intent, activation, websiteVisibilityStub{})
		if err != nil || !activated {
			t.Fatalf("activated=%v err=%v", activated, err)
		}
	}
	activate(product)
	old, err := store.ActivePublications(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	product.Name = "New revision"
	product, err = store.UpdateProduct(t.Context(), owner.ID, product.Revision, product)
	if err != nil {
		t.Fatal(err)
	}
	activate(product)
	newer, err := store.ActivePublications(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Delayed reconciliation must leave the newer activation's marker intact.
	if err = store.RebuildSpecSearchProjection(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	got, err := store.SpecSearchCandidates(t.Context(), nil, publishing.SpecProjectionVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Inventory) != 1 || got.Inventory[0].PublicRevision != newer[0].PublicRevision || len(got.Candidates) != 1 {
		t.Fatalf("old rebuild replaced new binding: %+v", got)
	}
}
