package sqlite

import (
	"errors"
	"testing"

	"prods/internal/catalog"
)

func TestTaxonomyContentPersistsPerFieldSourceLocales(t *testing.T) {
	store, owner := installedStore(t)
	category, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{
		Name: "Power", Slug: "power", Description: "Power products",
		SourceLocales: map[string]string{"description": "de-DE"},
	})
	if err != nil {
		t.Fatal(err)
	}
	content, err := store.TaxonomyContent(t.Context(), "category", category.ID)
	if err != nil {
		t.Fatal(err)
	}
	if content.SourceLocales["name"] != "en-US" || content.SourceLocales["description"] != "de-DE" {
		t.Fatalf("source locales = %+v", content.SourceLocales)
	}
}

func TestTaxonomyTranslationsAreDurableWhenEditingOrLocaleIsDisabled(t *testing.T) {
	store, owner := installedStore(t)
	category, err := store.CreateCategory(t.Context(), owner.ID, catalog.Category{Name: "Power", Slug: "power"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.SiteSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings, err = store.UpdateContentLocalization(t.Context(), owner.ID, settings.Revision, true, []string{"en-US", "zh-TW"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := store.SaveTaxonomyTranslation(t.Context(), owner.ID, "category", category.ID, category.Revision, catalog.TaxonomyTranslation{
		Locale: "zh-TW", Name: "電源", Description: "電源產品",
	})
	if err != nil {
		t.Fatal(err)
	}
	settings, err = store.UpdateContentLocalization(t.Context(), owner.ID, settings.Revision, false, []string{"en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveTaxonomyTranslation(t.Context(), owner.ID, "category", category.ID, content.SubjectRevision, catalog.TaxonomyTranslation{Locale: "zh-TW", Name: "不可寫"}); !errors.Is(err, ErrContentLocaleUnavailable) {
		t.Fatalf("disabled editing error = %v", err)
	}
	preserved, err := store.TaxonomyContent(t.Context(), "category", category.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserved.Translations) != 1 || preserved.Translations[0].Name != "電源" {
		t.Fatalf("preserved translations = %+v", preserved.Translations)
	}
}
