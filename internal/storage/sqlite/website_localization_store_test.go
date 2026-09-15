package sqlite

import (
	"path/filepath"
	"testing"

	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/site"
)

func TestWebsiteLocalizationSaveIsWorkingOnlyAndVersionRestoreIncludesIt(t *testing.T) {
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US", "zh-TW"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if initial.ActiveLocalization.DefaultLocale != "en-US" || len(initial.ActiveLocalization.EnabledLocales) != 2 {
		t.Fatalf("initial active localization = %+v", initial.ActiveLocalization)
	}
	working, err := store.SaveWebsiteLocalization(t.Context(), owner.ID, initial.WorkingRevision, site.WebsiteLocalization{
		DefaultLocale: "zh-TW", EnabledLocales: []string{"en-US", "zh-TW", "fr-FR"}, ContentEditingEnabled: true,
		PublicCopyOverrides: localization.PublicCopyOverrideMap{
			"catalog.title": {
				"fr-FR": {Key: "catalog.title", Locale: "fr-FR", Value: "Catalogue", DefinitionVersion: 1},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if working.WorkingLocalization.DefaultLocale != "zh-TW" || working.ActiveLocalization.DefaultLocale != "en-US" {
		t.Fatalf("working save changed active state: %+v", working)
	}
	versions, err := store.WebsiteVersions(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.RestoreWebsiteVersion(t.Context(), owner.ID, working.WorkingRevision, versions[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if restored.WorkingLocalization.DefaultLocale != "en-US" || len(restored.WorkingLocalization.PublicCopyOverrides) != 0 {
		t.Fatalf("restored localization = %+v", restored.WorkingLocalization)
	}
}
