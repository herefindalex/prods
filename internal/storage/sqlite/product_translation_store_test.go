package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func TestProductTranslationsPreserveSourceRevisionAndDisabledLocaleData(t *testing.T) {
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{OwnerEmail: "owner@example.test", PasswordHash: hash, DefaultLocale: "en-US", SupportedLocales: []string{"en-US", "zh-TW"}, TimeZone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "ML-1", Name: "Source name", Description: "Source description"})
	if err != nil {
		t.Fatal(err)
	}
	zhSource, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "ML-ZH", Name: "中文原文", SourceLocale: "zh-TW"})
	if err != nil {
		t.Fatal(err)
	}
	zhContent, err := store.ProductContent(t.Context(), zhSource.ID)
	if err != nil || zhContent.SourceLocale != "zh-TW" {
		t.Fatalf("zh source content=%+v err=%v", zhContent, err)
	}
	if _, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{PartNumber: "ML-BAD", SourceLocale: "fr-FR"}); !errors.Is(err, catalog.ErrInvalidProduct) {
		t.Fatalf("invalid source locale error=%v", err)
	}
	if _, err = store.SaveProductTranslation(t.Context(), owner.ID, product.ID, product.Revision, catalog.ProductTranslation{Locale: "zh-TW", Name: "翻譯名稱"}); !errors.Is(err, ErrContentLocaleUnavailable) {
		t.Fatalf("disabled editing error = %v", err)
	}
	settings, err := store.SiteSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings, err = store.UpdateContentLocalization(t.Context(), owner.ID, settings.Revision, true, []string{"en-US", "zh-TW"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := store.SaveProductTranslation(t.Context(), owner.ID, product.ID, product.Revision, catalog.ProductTranslation{Locale: "zh-TW", Name: "翻譯名稱", Description: "翻譯說明"})
	if err != nil {
		t.Fatal(err)
	}
	if content.SourceLocale != "en-US" || content.ProductRevision != product.Revision+1 || len(content.Translations) != 1 || content.Translations[0].Name != "翻譯名稱" {
		t.Fatalf("content = %+v", content)
	}
	clone, err := store.CloneProduct(t.Context(), owner.ID, product.ID, content.ProductRevision, catalog.Product{PartNumber: "ML-2"})
	if err != nil {
		t.Fatal(err)
	}
	clonedContent, err := store.ProductContent(t.Context(), clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if clonedContent.SourceLocale != "en-US" || len(clonedContent.Translations) != 1 || clonedContent.Translations[0].Description != "翻譯說明" {
		t.Fatalf("cloned content = %+v", clonedContent)
	}
	settings, err = store.UpdateContentLocalization(t.Context(), owner.ID, settings.Revision, true, []string{"en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveProductTranslation(t.Context(), owner.ID, product.ID, content.ProductRevision, catalog.ProductTranslation{Locale: "zh-TW", Name: "不可公開"}); !errors.Is(err, ErrContentLocaleUnavailable) {
		t.Fatalf("removed locale error = %v", err)
	}
	preserved, err := store.ProductContent(t.Context(), product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(preserved.Translations) != 1 || preserved.Translations[0].Name != "翻譯名稱" {
		t.Fatalf("preserved = %+v", preserved)
	}
}
