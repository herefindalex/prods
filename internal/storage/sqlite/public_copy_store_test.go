package sqlite

import (
	"path/filepath"
	"testing"

	"prods/internal/localization"
)

func TestOfficialPublicCopyInstallRequiresEveryBuiltinLocaleAndIsDurable(t *testing.T) {
	store, err := CreatePOC(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog := localization.PublicCopyCatalog{
		OfficialBundle: "test-v1",
		ReviewStatus:   "generated-unreviewed",
		Definitions: []localization.PublicCopyDefinition{{
			Key: "catalog.title", DefinitionVersion: 1, Description: "Catalog title",
			ValueKind: localization.PublicCopyPlain, OfficialBundle: "test-v1",
		}},
	}
	for _, locale := range localization.BuiltinLocaleCodes() {
		catalog.Defaults = append(catalog.Defaults, localization.PublicCopyDefault{
			Key: "catalog.title", Locale: locale, Value: "Catalog " + locale,
			DefinitionVersion: 1, OfficialBundle: "test-v1",
		})
	}
	missing := catalog
	missing.Defaults = append([]localization.PublicCopyDefault(nil), catalog.Defaults[:len(catalog.Defaults)-1]...)
	if err := store.InstallOfficialPublicCopy(t.Context(), missing); err == nil {
		t.Fatal("incomplete official bundle installed")
	}
	if err := store.InstallOfficialPublicCopy(t.Context(), catalog); err != nil {
		t.Fatal(err)
	}
	if err := store.InstallOfficialPublicCopy(t.Context(), catalog); err != nil {
		t.Fatalf("idempotent install: %v", err)
	}
	loaded, err := store.PublicCopyCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OfficialBundle != "test-v1" || loaded.ReviewStatus != "generated-unreviewed" || len(loaded.Definitions) != 1 || len(loaded.Defaults) != 10 {
		t.Fatalf("loaded catalog = %+v", loaded)
	}
}
