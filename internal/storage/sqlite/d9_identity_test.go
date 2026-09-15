package sqlite

import (
	"errors"
	"testing"

	"prods/internal/catalog"
)

func TestBlankAndNamedManufacturerIdentitiesCoexistButAssignmentRechecksConflict(t *testing.T) {
	store, owner := installedStore(t)
	blank, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "ABC123", Name: "Unassigned part"})
	if err != nil {
		t.Fatal(err)
	}
	ti, err := store.CreateDictionaryEntry(t.Context(), owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryManufacturer, Name: "TI", Slug: "ti",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber: "ABC123", Name: "TI part", ManufacturerID: ti.ID,
	}); err != nil {
		t.Fatalf("blank and named identities must coexist: %v", err)
	}
	blank.ManufacturerID = ti.ID
	if _, err := store.UpdateProduct(t.Context(), owner.ID, blank.Revision, blank); !errors.Is(err, catalog.ErrIdentityConflict) {
		t.Fatalf("assignment conflict = %v", err)
	}
	unchanged, err := store.Product(t.Context(), blank.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.ManufacturerID != "" || unchanged.Revision != blank.Revision {
		t.Fatalf("conflicting assignment changed product = %+v", unchanged)
	}
}
