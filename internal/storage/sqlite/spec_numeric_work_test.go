package sqlite

import (
	"prods/internal/catalog"
	"testing"
)

func TestNumericProfileWorkResumesAfterReopen(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	for {
		worked, err := f.store.ProcessSpecNumericProfileWork(t.Context(), 100)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	f.adopt(t)
	first, err := f.store.Product(t.Context(), c.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.CreateProduct(t.Context(), f.actor, catalog.Product{PartNumber: "SECOND-NUMERIC", CategoryID: first.CategoryID})
	if err != nil {
		t.Fatal(err)
	}
	var specRevision int64
	if err = f.store.db.QueryRow(`SELECT revision FROM spec_definitions WHERE id=?`, c.SpecID).Scan(&specRevision); err != nil {
		t.Fatal(err)
	}
	profile := c.Profile
	profile.DeclaredInputUnit = "V"
	configured, err := f.store.ConfigureSpecNumericProfile(t.Context(), f.actor, c.SpecID, specRevision, profile)
	if err != nil {
		t.Fatal(err)
	}
	worked, err := f.store.ProcessSpecNumericProfileWork(t.Context(), 1)
	if err != nil || !worked {
		t.Fatalf("first batch worked=%v err=%v", worked, err)
	}
	var status, cursor string
	if err = f.store.db.QueryRow(`SELECT status,cursor_product_id FROM spec_numeric_profile_work WHERE spec_id=? AND profile_revision=?`, c.SpecID, configured.Revision).Scan(&status, &cursor); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || cursor == "" {
		t.Fatalf("partial batch status=%s cursor=%s", status, cursor)
	}
	var seq int
	var name, path string
	if err = f.store.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReady(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	for {
		worked, err = reopened.ProcessSpecNumericProfileWork(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	for _, p := range []catalog.Product{first, second} {
		got, err := reopened.Product(t.Context(), p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Revision != p.Revision+1 {
			t.Fatalf("product %s revision=%d want=%d", p.ID, got.Revision, p.Revision+1)
		}
		var audit, intent int
		if err = reopened.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE target_id=? AND action='product.spec_profile_changed' AND actor_id IS NULL`, p.ID).Scan(&audit); err != nil {
			t.Fatal(err)
		}
		if err = reopened.db.QueryRow(`SELECT COUNT(*) FROM publication_intents WHERE entity_id=? AND desired_revision=? AND cause='product.spec_profile_changed'`, p.ID, got.Revision).Scan(&intent); err != nil {
			t.Fatal(err)
		}
		if audit < 1 || intent != 1 {
			t.Fatalf("audit=%d intent=%d", audit, intent)
		}
	}
	var preference string
	if err = reopened.db.QueryRow(`SELECT ns.preference,nv.status FROM spec_normalization_selections ns JOIN normalized_values nv ON nv.id=ns.selected_normalized_value_id WHERE ns.spec_value_id=?`, f.value.ID).Scan(&preference, &status); err != nil {
		t.Fatal(err)
	}
	if preference != "manual" || status != "stale" {
		t.Fatalf("manual preference=%s status=%s", preference, status)
	}
}
