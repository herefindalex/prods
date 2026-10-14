package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"

	"prods/internal/catalog"
)

func TestSharedSpecMigrationQueuesPublishedRevisionWithoutActivating(t *testing.T) {
	store, owner := installedStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "SHARED-SPECS", Specification: "authoritative specification"})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := store.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Voltage"})
	if err != nil {
		t.Fatal(err)
	}
	removeNumericSchema(t, store)
	if _, err := store.db.Exec(`
UPDATE products SET revision=7,status='published' WHERE id=?1;
INSERT INTO product_spec_values(id,product_id,spec_id,raw_value,source_locale,source_revision,active,created_at,updated_at)
VALUES('spv_shared_legacy',?1,?2,'3–5V','zh-TW',3,1,'legacy-time','legacy-time');
INSERT INTO normalized_values(id,spec_value_id,value_json,source_revision,spec_semantic_version,normalizer_version,source,status,failure_reason,created_at)
VALUES('nrm_shared_legacy','spv_shared_legacy','{"min":3,"max":5}',3,1,'legacy-manual','manual','current','','legacy-time');
INSERT INTO product_translations(product_id,locale,specification,revision,updated_at) VALUES(?1,'zh-TW','舊的翻譯規格',2,'legacy-time');
INSERT INTO public_activations(product_id,source_revision,public_revision,site_epoch,route,artifact_id,manifest_hash,visibility_generation,view_json,activated_at)
VALUES(?1,7,7,1,'/products/shared-specs','artifact_legacy_shared','manifest-legacy',2,'{"specification":"authoritative specification"}','legacy-time');
INSERT INTO public_visibility(product_id,generation,visible,updated_at) VALUES(?1,2,1,'legacy-time');
INSERT INTO public_route_history(route,product_id,target_route,state,site_epoch,updated_at)
VALUES('/products/shared-specs-old',?1,'/products/shared-specs','redirect',1,'legacy-time');
INSERT INTO publication_intents(id,entity_type,entity_id,desired_revision,cause,status,created_at,updated_at,error_message)
VALUES('out_shared_failed','product',?1,7,'legacy','failed','legacy-time','legacy-time','legacy-failure'),
('out_shared_pending','product',?1,7,'legacy','pending','legacy-time','legacy-time','');`,
		product.ID, spec.ID); err != nil {
		t.Fatal(err)
	}
	var beforeVersion int
	if err := store.db.QueryRow(`SELECT schema_version FROM system_state`).Scan(&beforeVersion); err != nil {
		t.Fatal(err)
	}
	if beforeVersion != 18 {
		t.Fatalf("fixture schema=%d, want 18", beforeVersion)
	}
	if err := store.Upgrade(t.Context(), "verified-shared-model-v18"); err != nil {
		t.Fatal(err)
	}
	var seq int
	var name, path string
	if err := store.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenReady(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var revision int64
	if err := store.db.QueryRow(`SELECT revision FROM products WHERE id=?`, product.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 8 {
		t.Fatalf("published source revision=%d, want 8", revision)
	}
	var auditActor sql.NullString
	var detailsJSON, auditResult, targetType string
	if err := store.db.QueryRow(`SELECT actor_id,result,target_type,details_json FROM admin_log WHERE target_id=? AND action='product.spec_model_migrated'`, product.ID).Scan(&auditActor, &auditResult, &targetType, &detailsJSON); err != nil {
		t.Fatal(err)
	}
	var details struct {
		Migration             int    `json:"migration"`
		OldRevision           int64  `json:"old_revision"`
		NewRevision           int64  `json:"new_revision"`
		RenderContractVersion string `json:"render_contract_version"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &details); err != nil {
		t.Fatal(err)
	}
	if auditActor.Valid || auditResult != "success" || targetType != "product" || details.Migration != 19 || details.OldRevision != 7 || details.NewRevision != 8 || details.RenderContractVersion != "shared-specs-1" {
		t.Fatalf("system audit: actor=%+v result=%s type=%s details=%s", auditActor, auditResult, targetType, detailsJSON)
	}
	var pending, auditCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM publication_intents WHERE entity_type='product' AND entity_id=? AND desired_revision=8 AND status='pending' AND cause='product.spec_model_migrated'`, product.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE target_id=? AND action='product.spec_model_migrated'`, product.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if pending != 1 || auditCount != 1 {
		t.Fatalf("migration intent=%d audit=%d, want one each", pending, auditCount)
	}
	var raw, normalizedJSON, translation, activation, route, oldFailed, oldPending string
	if err := store.db.QueryRow(`SELECT pv.raw_value,n.value_json,t.specification,
 a.source_revision||'|'||a.public_revision||'|'||a.route||'|'||a.artifact_id||'|'||a.manifest_hash||'|'||a.view_json,
 h.target_route||'|'||h.state,
 (SELECT status||'|'||error_message FROM publication_intents WHERE id='out_shared_failed'),
 (SELECT status FROM publication_intents WHERE id='out_shared_pending')
 FROM product_spec_values pv JOIN normalized_values n ON n.spec_value_id=pv.id
 JOIN product_translations t ON t.product_id=pv.product_id JOIN public_activations a ON a.product_id=pv.product_id
 JOIN public_route_history h ON h.product_id=pv.product_id WHERE pv.id='spv_shared_legacy'`).Scan(&raw, &normalizedJSON, &translation, &activation, &route, &oldFailed, &oldPending); err != nil {
		t.Fatal(err)
	}
	if raw != "3–5V" || normalizedJSON != `{"min":3,"max":5}` || translation != "舊的翻譯規格" ||
		activation != `7|7|/products/shared-specs|artifact_legacy_shared|manifest-legacy|{"specification":"authoritative specification"}` ||
		route != "/products/shared-specs|redirect" || oldFailed != "failed|legacy-failure" || oldPending != "pending" {
		t.Fatalf("legacy data/activation/history changed: %q %q %q %q %q %q %q", raw, normalizedJSON, translation, activation, route, oldFailed, oldPending)
	}
}

func TestSharedSpecMigrationLeavesHiddenAndArchivedProductsUnqueued(t *testing.T) {
	store, owner := installedStore(t)
	hidden, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "SHARED-HIDDEN"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "SHARED-ARCHIVED"})
	if err != nil {
		t.Fatal(err)
	}
	removeNumericSchema(t, store)
	if _, err := store.db.Exec(`UPDATE products SET record_state='archived' WHERE id=?`, archived.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Upgrade(t.Context(), "verified-shared-ineligible-v18"); err != nil {
		t.Fatal(err)
	}
	var unchanged, intents, audits int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM products WHERE id IN (?,?) AND revision=1`, hidden.ID, archived.ID).Scan(&unchanged); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM publication_intents WHERE cause='product.spec_model_migrated'`).Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='product.spec_model_migrated'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if unchanged != 2 || intents != 0 || audits != 0 {
		t.Fatalf("ineligible products changed: unchanged=%d intents=%d audits=%d", unchanged, intents, audits)
	}
}
