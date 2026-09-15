package sqlite

import (
	"testing"
	"time"

	"prods/internal/catalog"
)

func TestOperationalHealthDetectsSearchProjectionDriftAndCurrentWork(t *testing.T) {
	store, owner := installedStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "HEALTH-1"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(t.Context(), `
		INSERT INTO public_visibility(product_id,generation,visible,updated_at) VALUES(?,1,1,?);
		INSERT INTO public_activations(
			product_id,source_revision,public_revision,site_epoch,route,artifact_id,
			manifest_hash,visibility_generation,view_json,activated_at
		) VALUES(?,1,1,1,?,'artifact-health','hash',1,'{}',?);
		INSERT INTO publication_intents(id,entity_type,entity_id,desired_revision,cause,status,created_at,updated_at)
		VALUES('intent-health','product',?,1,'test','failed',?,?);
	`, product.ID, now, product.ID, "/products/"+product.Slug, now, product.ID, now, now); err != nil {
		t.Fatal(err)
	}
	health, err := store.OperationalHealth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if health.ActivePublicProducts != 1 || health.SearchProjectionDrift != 1 || health.PublicationFailed != 1 {
		t.Fatalf("operational health with drift=%+v", health)
	}
	if _, err := store.db.ExecContext(t.Context(), `
		INSERT INTO public_search_projection(
			product_id,source_revision,site_epoch,visibility_generation,projection_version,
			part_number_folded,product_name_folded,manufacturer_folded,brand_folded,category_folded,all_folded,updated_at
		) VALUES(?,1,1,1,?,'health-1','','','','','health-1',?)
	`, product.ID, catalog.SearchProjectionVersion, now); err != nil {
		t.Fatal(err)
	}
	health, err = store.OperationalHealth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if health.SearchProjectionDrift != 0 {
		t.Fatalf("operational health after projection repair=%+v", health)
	}
}

func TestOperationalHealthIncludesDurableAssetLifecycleState(t *testing.T) {
	store, owner := installedStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "HEALTH-ASSET-1"})
	if err != nil {
		t.Fatal(err)
	}
	asset := createGCAsset(t, store, owner.ID, product.ID, "asset_health")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(t.Context(), `
INSERT INTO asset_gc_orphans(asset_id,orphaned_at) VALUES(?,?);
INSERT INTO asset_gc_deletions(asset_id,storage_path,state,last_error,planned_at,updated_at)
VALUES('asset_failed','products/missing/content.png','failed','offline',?,?);`, asset.ID, now, now, now); err != nil {
		t.Fatal(err)
	}
	health, err := store.OperationalHealth(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if health.AssetOrphans != 1 || health.AssetDeletionPending != 0 || health.AssetDeletionFailed != 1 {
		t.Fatalf("asset lifecycle health=%+v", health)
	}
}
