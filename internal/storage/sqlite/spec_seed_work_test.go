package sqlite

import (
	"path/filepath"
	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/identity"
	"strings"
	"testing"
)

func TestSampleNormalizationIsDurableAndCoalescesUnactivatedIntents(t *testing.T) {
	s, err := Create(filepath.Join(t.TempDir(), "sample.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	sample := &InstallationSampleData{Categories: []catalog.Category{{ID: "cat_power", ParentID: "cat_root", Name: "Power", Slug: "power", Status: catalog.EntryActive}}, Version: "v-test", DatasetVersion: "test", SourceURL: "embedded:test", SHA256: strings.Repeat("a", 64), Specs: []catalog.SpecDefinition{{ID: "a", Name: "Voltage A", Filterable: true, SemanticVer: 1}, {ID: "b", Name: "Voltage B", Filterable: true, SemanticVer: 1}}, SpecSearchProfiles: map[string]catalogfilter.Profile{"a": catalogfilter.NumericProfile(numericTestProfile()), "b": catalogfilter.NumericProfile(numericTestProfile())}, SpecSets: []catalog.SpecSet{{ID: "set", Name: "Power", SpecIDs: []string{"a", "b"}}}, CategorySpecSets: []InstallationCategorySpecSet{{CategoryID: "cat_power", SpecSetID: "set"}}, Products: []catalog.Product{{ID: "p", PartNumber: "SAMPLE", CategoryID: "cat_power", Status: catalog.Published}}, SpecValues: []catalog.SpecValue{{ID: "va", ProductID: "p", SpecID: "a", RawValue: "3–5V"}, {ID: "vb", ProductID: "p", SpecID: "b", RawValue: "4–6V"}}}
	if _, err = s.CompleteInstallation(t.Context(), Installation{OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC", SampleData: sample}); err != nil {
		t.Fatal(err)
	}
	var pending, adopted int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM spec_normalization_work WHERE status='pending'`).Scan(&pending); err != nil || pending != 2 {
		t.Fatalf("durable initial work=%d %v", pending, err)
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM spec_normalization_selections`).Scan(&adopted); err != nil || adopted != 0 {
		t.Fatalf("installer parsed/adopted before returning: %d %v", adopted, err)
	}
	for {
		worked, err := s.ProcessSpecNormalizationWork(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	for _, id := range []string{"va", "vb"} {
		c, err := s.SpecNormalizationContext(t.Context(), id)
		if err != nil || c.Selection.Preference != catalog.NormalizedAutomatic {
			t.Fatalf("adoption %s: %+v %v", id, c, err)
		}
	}
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM publication_intents WHERE entity_id='p' AND status='pending'`).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("unactivated intents=%d %v", pending, err)
	}
}
