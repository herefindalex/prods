package sqlite

import (
	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"reflect"
	"testing"
)

func TestUpgradePreservesExistingSpecSources(t *testing.T) {
	f := newNormalizationFixture(t)
	f.adopt(t)
	before := f.snapshot(t)
	values, err := f.store.ProductSpecValues(t.Context(), before.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.db.Exec(`DROP TABLE spec_normalization_work_items;DROP TABLE spec_normalization_work;DROP TABLE spec_filter_profile_work;DROP TABLE spec_filter_profiles;DELETE FROM schema_migrations WHERE version=20;UPDATE system_state SET schema_version=19 WHERE singleton=1;`); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Upgrade(t.Context(), "verified-filter-upgrade-backup"); err != nil {
		t.Fatal(err)
	}
	after := f.snapshot(t)
	afterValues, err := f.store.ProductSpecValues(t.Context(), before.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(values, afterValues) {
		t.Fatal("upgrade altered Raw, selected source, manual history or preference")
	}
	p, err := f.store.SpecSearchProfile(t.Context(), before.SpecID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Profile.Kind != "numeric" {
		t.Fatal("existing explicit numeric profile was replaced")
	}
	p.Profile = catalogfilter.DefaultTextProfile()
	if _, err = f.store.ConfigureSpecSearchProfile(t.Context(), f.actor, before.SpecID, p.Revision, catalog.SpecSearchProfile{Filterable: true, Profile: p.Profile}); err != nil {
		t.Fatal(err)
	}
	finalValues, err := f.store.ProductSpecValues(t.Context(), before.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if finalValues[0].Value.RawValue != values[0].Value.RawValue || finalValues[0].SourceSelection != values[0].SourceSelection {
		t.Fatal("text configuration rewrote source data")
	}
}
