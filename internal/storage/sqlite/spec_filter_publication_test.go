package sqlite

import (
	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/publishing"
	"testing"
	"time"
)

func TestPublishedFilterDefinitionStaysFrozen(t *testing.T) {
	f := newNormalizationFixture(t)
	ctx := t.Context()
	c := f.snapshot(t)
	p, err := f.store.Product(ctx, c.ProductID)
	if err != nil {
		t.Fatal(err)
	}
	p.Status = catalog.Published
	p, err = f.store.UpdateProduct(ctx, f.actor, p.Revision, p)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.PublicationSource(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	intent, ok, err := f.store.ClaimPublicationIntent(ctx)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	view, err := publishing.FromProduct(p, "https://catalog.example.test")
	if err != nil {
		t.Fatal(err)
	}
	view.FilterDefinitions = source.FilterDefinitions
	view.NumericDefinitions = source.NumericDefinitions
	view.NumericFacts = source.NumericFacts
	view.Specifications = []publishing.Specification{{ID: c.SpecID, RawValue: c.Raw}}
	activation := publishing.ActivePublication{ProductID: p.ID, SourceRevision: p.Revision, PublicRevision: p.Revision, SiteEpoch: source.SiteEpoch, Route: source.Route, ArtifactID: "art_filter", ManifestHash: "hash_filter", ActivatedAt: time.Now().UTC(), View: view}
	for range 5 {
		ok, err = f.store.ActivatePublication(ctx, intent, activation, websiteVisibilityStub{})
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			break
		}
		intent, ok, err = f.store.ClaimPublicationIntent(ctx)
		if err != nil || !ok {
			t.Fatalf("claim: %v %v", ok, err)
		}
		ok = false
	}
	if !ok {
		t.Fatal("initial unit not activated")
	}
	profile, err := f.store.SpecSearchProfile(ctx, c.SpecID)
	if err != nil {
		t.Fatal(err)
	}
	profile.Profile = catalogfilter.DefaultTextProfile()
	if _, err = f.store.ConfigureSpecSearchProfile(ctx, f.actor, c.SpecID, profile.Revision, profile); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.SaveSpecValue(ctx, f.actor, p.ID, c.SpecID, "SPI", "en-US", p.Revision); err != nil {
		t.Fatal(err)
	}
	active, err := f.store.ActivePublications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].View.FilterDefinitions[0].Profile.Kind != "numeric" {
		t.Fatalf("working config leaked: %+v", active)
	}
	if active[0].View.Specifications[0].RawValue != c.Raw {
		t.Fatal("working Raw leaked")
	}
	tx, err := f.store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if current, err := publicationNumericSnapshotCurrent(ctx, tx, activation); err != nil || current {
		t.Fatalf("stale candidate accepted before fanout: %v %v", current, err)
	}
	tx.Rollback()
	if _, err = f.store.ProcessSpecSearchProfileWork(ctx, 100); err != nil {
		t.Fatal(err)
	}
	source, err = f.store.PublicationSource(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	activation.SourceRevision = source.Product.Revision
	activation.PublicRevision = source.Product.Revision
	activation.View.Revision = source.Product.Revision
	activation.View.FilterDefinitions = source.FilterDefinitions
	activation.View.NumericDefinitions = source.NumericDefinitions
	activation.View.NumericFacts = source.NumericFacts
	activation.View.Specifications = []publishing.Specification{{ID: c.SpecID, RawValue: source.Specs[0].RawValue}}
	activated := false
	for range 5 {
		intent, found, err := f.store.ClaimPublicationIntent(ctx)
		if err != nil || !found {
			t.Fatalf("claim replacement: %v %v", found, err)
		}
		activated, err = f.store.ActivatePublication(ctx, intent, activation, websiteVisibilityStub{})
		if err != nil {
			t.Fatal(err)
		}
		if activated {
			break
		}
	}
	if !activated {
		t.Fatal("replacement not activated")
	}
	active, err = f.store.ActivePublications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active[0].View.FilterDefinitions[0].Profile.Kind != "text" || active[0].View.Specifications[0].RawValue != "SPI" {
		t.Fatalf("new unit missing: %+v", active[0].View)
	}
}
