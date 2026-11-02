package webapp

import (
	"prods/internal/catalogfilter"
	"prods/internal/publishing"
	"testing"
)

func TestCatalogRevocationRemovesFacetValues(t *testing.T) {
	f := newNumericPublicationFixture(t)
	p, err := f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	p.PackageFormFactor = "UNIQUE-PACKAGE"
	if _, err = f.store.UpdateProduct(t.Context(), f.actor, p.Revision, p); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	request := publishing.CatalogQuery{Scope: catalogfilter.Scope{Kind: "all"}}
	before, err := f.engine.AdmitCatalog(t.Context(), request)
	if err != nil || len(before.Options["package"]) != 1 {
		t.Fatalf("before: %+v %v", before.Options, err)
	}
	p, err = f.store.Product(t.Context(), f.productID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.engine.Revoke(t.Context(), publishing.RevokeRequest{ActorID: f.actor, ProductID: p.ID, ExpectedRevision: p.Revision}); err != nil {
		t.Fatal(err)
	}
	after, err := f.engine.AdmitCatalog(t.Context(), request)
	if err != nil || len(after.Views) != 0 || len(after.Options["package"]) != 0 {
		t.Fatalf("revoked rows=%d options=%+v err=%v", len(after.Views), after.Options, err)
	}
}
