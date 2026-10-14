package sqlite

import (
	"errors"
	"testing"

	"prods/internal/catalog"
	"prods/internal/catalogfilter"
)

func TestSpecSearchProfileRequiresExpectedRevision(t *testing.T) {
	s, owner := installedStore(t)
	spec, err := s.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Interface", Filterable: true})
	if err != nil {
		t.Fatal(err)
	}
	p := catalog.SpecSearchProfile{Filterable: true, Profile: catalogfilter.Profile{Version: 1, Kind: "choice", TextRule: "unicode-fold-space-1", Operators: []string{"equals"}}}
	got, err := s.ConfigureSpecSearchProfile(t.Context(), owner.ID, spec.ID, spec.Revision, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != spec.Revision+1 || got.Profile.Kind != "choice" {
		t.Fatalf("configured: %+v", got)
	}
	p.Profile.Kind = "text"
	if _, err = s.ConfigureSpecSearchProfile(t.Context(), owner.ID, spec.ID, spec.Revision, p); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale config: %v", err)
	}
	current, err := s.SpecSearchProfile(t.Context(), spec.ID)
	if err != nil || current.Profile.Kind != "choice" {
		t.Fatalf("config replaced: %+v %v", current, err)
	}
	var work int
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM spec_filter_profile_work WHERE spec_id=? AND status='pending'`, spec.ID).Scan(&work); err != nil || work != 1 {
		t.Fatalf("durable work=%d err=%v", work, err)
	}
}

func TestNormalizationWorkPreservesManualPreferenceAndRaw(t *testing.T) {
	f := newNormalizationFixture(t)
	manual := f.adopt(t)
	c := f.snapshot(t)
	if _, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "4–6V", "en-US", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	p, err := f.store.SpecSearchProfile(t.Context(), c.SpecID)
	if err != nil {
		t.Fatal(err)
	}
	work, err := f.store.StartSpecNormalizationWork(t.Context(), f.actor, c.SpecID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	for {
		worked, err := f.store.ProcessSpecNormalizationWork(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	current := f.snapshot(t)
	if current.Raw != "4–6V" || current.Selection.Preference != catalog.NormalizedManual || current.Selection.SelectedNormalizedValueID != manual.NormalizedValueID {
		t.Fatalf("manual/source changed: %+v", current)
	}
	state, err := f.store.SpecNormalizationWork(t.Context(), work.ID)
	if err != nil || state.Status != "completed" || state.Skipped != 1 || state.Processed != 1 {
		t.Fatalf("work: %+v err=%v", state, err)
	}
}

func TestTextSearchProfileInvalidatesNumericContext(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	p, err := f.store.SpecSearchProfile(t.Context(), c.SpecID)
	if err != nil {
		t.Fatal(err)
	}
	p.Profile = catalogfilter.DefaultTextProfile()
	if _, err = f.store.ConfigureSpecSearchProfile(t.Context(), f.actor, c.SpecID, p.Revision, p); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SpecNormalizationContext(t.Context(), c.SpecValueID); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("obsolete numeric meaning remained editable: %v", err)
	}
}

func TestNormalizationWorkReportsUnsupportedRaw(t *testing.T) {
	f := newNormalizationFixture(t)
	c := f.snapshot(t)
	if _, err := f.store.SaveSpecValue(t.Context(), f.actor, c.ProductID, c.SpecID, "62 accuracy", "en-US", c.Expected.ProductRevision); err != nil {
		t.Fatal(err)
	}
	p, err := f.store.SpecSearchProfile(t.Context(), c.SpecID)
	if err != nil {
		t.Fatal(err)
	}
	work, err := f.store.StartSpecNormalizationWork(t.Context(), f.actor, c.SpecID, p.Revision)
	if err != nil {
		t.Fatal(err)
	}
	for {
		worked, err := f.store.ProcessSpecNormalizationWork(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	state, err := f.store.SpecNormalizationWork(t.Context(), work.ID)
	if err != nil || state.Failed != 1 {
		t.Fatalf("work: %+v err=%v", state, err)
	}
	current := f.snapshot(t)
	if current.Raw != "62 accuracy" || current.Selection.SelectedNormalizedValueID != "" {
		t.Fatalf("invented numeric fact: %+v", current)
	}
}
