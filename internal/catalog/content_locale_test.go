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
	if _, err := NormalizeFieldSourceLocales(map[string]string{"price": "en-US"}, "en-US", ProductTranslatableFields); err != ErrInvalidContentLocale {
		t.Fatalf("unknown field error = %v", err)
	}
}
