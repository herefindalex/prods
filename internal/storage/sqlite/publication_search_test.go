package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/publishing"
)

func TestSearchPublicationsRanksPartContainsBeforeNameMatch(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	exact := addSearchPublication(t, ctx, store, owner.ID, "AMP", "Exact part")
	contains := addSearchPublication(t, ctx, store, owner.ID, "X-AMP-X", "Contains part")
	name := addSearchPublication(t, ctx, store, owner.ID, "ZZZ-1", "AMP module")

	results, err := store.SearchPublications(ctx, catalog.FoldSearch("amp"), catalog.SearchProjectionVersion)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	ids := []string{results[0].ProductID, results[1].ProductID, results[2].ProductID}
	want := []string{exact.ID, contains.ID, name.ID}
	for index := range want {
		if ids[index] != want[index] {
			t.Fatalf("ranked IDs = %v, want %v", ids, want)
		}
	}
}

func addSearchPublication(t *testing.T, ctx context.Context, store *Store, actorID, partNumber, name string) catalog.Product {
	t.Helper()
	product, err := store.CreateProduct(ctx, actorID, catalog.Product{PartNumber: partNumber, Name: name, Status: catalog.Published})
	if err != nil {
		t.Fatal(err)
	}
	view := publishing.PublicView{
		ID: product.ID, Revision: product.Revision, SiteEpoch: 1, PartNumber: product.PartNumber, Name: product.Name,
		Language: "en-US", DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, CanonicalURL: "https://catalog.example.test/products/" + product.Slug,
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	activation := publishing.ActivePublication{
		ProductID: product.ID, SourceRevision: product.Revision, PublicRevision: product.Revision, SiteEpoch: 1,
		Route: "/products/" + product.Slug, ArtifactID: "art_" + product.ID, ManifestHash: "hash_" + product.ID,
		VisibilityGeneration: 1, ActivatedAt: time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC), View: view,
	}
	err = store.withWriteTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_visibility(product_id,generation,visible,updated_at) VALUES(?,1,1,?)`, product.ID, activation.ActivatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_activations(product_id,source_revision,public_revision,site_epoch,route,artifact_id,manifest_hash,visibility_generation,view_json,activated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			product.ID, product.Revision, product.Revision, 1, activation.Route, activation.ArtifactID, activation.ManifestHash, 1, string(encoded), activation.ActivatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		return upsertPublicSearchProjection(ctx, tx, activation, 1, activation.ActivatedAt.Format(time.RFC3339Nano))
	})
	if err != nil {
		t.Fatal(err)
	}
	return product
}
