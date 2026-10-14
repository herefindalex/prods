package catalog

import "testing"

func TestNormalizeFieldSourceLocalesDefaultsAndAllowsPerFieldCorrection(t *testing.T) {
	got, err := NormalizeFieldSourceLocales(map[string]string{"description": "ja-jp"}, "zh-TW", ProductTranslatableFields)
	if err != nil {
		t.Fatal(err)
	}
	if got["name"] != "zh-TW" || got["description"] != "ja-JP" || len(got) != len(ProductTranslatableFields) {
		t.Fatalf("source locales = %+v", got)
	}
	if _, err := NormalizeFieldSourceLocales(map[string]string{"specification": "en-US"}, "en-US", ProductTranslatableFields); err != ErrInvalidContentLocale {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestProductPrepareRejectsSharedSpecificationSourceLocale(t *testing.T) {
	product := Product{ID: "source-1", PartNumber: "SOURCE-1", SourceLocales: map[string]string{"specification": "en-US"}}
	if err := product.Prepare(); err != ErrInvalidProduct {
		t.Fatalf("shared specification source=%v", err)
	}
	product.SourceLocales = map[string]string{"name": "en-US", "description": "ja-JP", "features": "zh-TW"}
	if err := product.Prepare(); err != nil {
		t.Fatal(err)
	}
}
