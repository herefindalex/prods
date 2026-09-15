package sqlite

import (
	"errors"
	"testing"

	"prods/internal/catalog"
)

func TestProductApplicationsAreValidatedSearchableReplaceableAndCloneable(t *testing.T) {
	ctx := t.Context()
	store, owner := installedStore(t)
	industrial, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryApplication, Name: "Industrial Control", Slug: "industrial-control", Status: catalog.EntryActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	automotive, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryApplication, Name: "Automotive", Slug: "automotive", Status: catalog.EntryActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryBrand, Name: "Not an application", Slug: "not-application", Status: catalog.EntryActive,
	})
	if err != nil {
		t.Fatal(err)
	}

	created, err := store.CreateProduct(ctx, owner.ID, catalog.Product{
		PartNumber: "APP-100", Status: catalog.Published,
		ApplicationIDs: []string{" " + industrial.ID + " ", automotive.ID, industrial.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Applications) != 2 || created.Applications[0].Name != "Industrial Control" || created.Applications[1].Name != "Automotive" {
		t.Fatalf("created applications = %+v", created.Applications)
	}
	stored, err := store.Product(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.ApplicationIDs) != 2 || stored.ApplicationIDs[0] != industrial.ID || stored.ApplicationIDs[1] != automotive.ID {
		t.Fatalf("stored application ids = %v", stored.ApplicationIDs)
	}
	products, total, err := store.ListPublished(ctx, "industrial control", 1, 20)
	if err != nil || total != 1 || len(products) != 1 || products[0].ID != created.ID {
		t.Fatalf("application search products=%+v total=%d err=%v", products, total, err)
	}

	updated := stored
	updated.ApplicationIDs = []string{automotive.ID}
	updated.Applications = nil
	updated, err = store.UpdateProduct(ctx, owner.ID, stored.Revision, updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Applications) != 1 || updated.Applications[0].ID != automotive.ID {
		t.Fatalf("updated applications = %+v", updated.Applications)
	}
	products, total, err = store.ListPublished(ctx, "industrial control", 1, 20)
	if err != nil || total != 0 || len(products) != 0 {
		t.Fatalf("removed application search products=%+v total=%d err=%v", products, total, err)
	}

	clone, err := store.CloneProduct(ctx, owner.ID, updated.ID, updated.Revision, catalog.Product{PartNumber: "APP-101"})
	if err != nil {
		t.Fatal(err)
	}
	if len(clone.ApplicationIDs) != 1 || clone.ApplicationIDs[0] != automotive.ID {
		t.Fatalf("clone applications = %+v", clone.Applications)
	}

	_, err = store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "APP-BAD", ApplicationIDs: []string{brand.ID}})
	if !errors.Is(err, catalog.ErrDisabledReference) {
		t.Fatalf("non-application reference error = %v", err)
	}
}
