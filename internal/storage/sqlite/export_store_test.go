package sqlite

import (
	"testing"

	"prods/internal/catalog"
)

func TestProductExportUsesCorrectedSpecSourceLocale(t *testing.T) {
	store, owner := installedStore(t)
	spec, err := store.CreateSpecDefinition(t.Context(), owner.ID, catalog.SpecDefinition{Name: "Source locale export"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "EXPORT-ORIGIN"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.SaveSpecValue(t.Context(), owner.ID, product.ID, spec.ID, "original", "en-US", product.Revision)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.SaveSpecValue(t.Context(), owner.ID, product.ID, spec.ID, "corrected", "zh-TW", product.Revision+1)
	if err != nil {
		t.Fatal(err)
	}

	var got string
	err = store.ForEachProductExport(t.Context(), owner.ID, func(ProductExportRow) error {
		return nil
	}, func(row RawSpecExportRow) error {
		if row.ProductID == product.ID && row.SpecID == spec.ID {
			got = row.SourceLocale
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != updated.SourceLocale {
		t.Fatalf("export source locale = %q, corrected Raw origin = %q", got, updated.SourceLocale)
	}
}
