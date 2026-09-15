package site

import (
	"errors"
	"testing"

	"prods/internal/localization"
)

func TestPrepareTimeZone(t *testing.T) {
	for _, value := range []string{"UTC", "America/New_York", "Asia/Taipei", "Local"} {
		if prepared, err := PrepareTimeZone("  " + value + "  "); err != nil || prepared != value {
			t.Fatalf("PrepareTimeZone(%q) = %q, %v", value, prepared, err)
		}
	}
	for _, value := range []string{"", "Not/A_Time_Zone", "UTC\nInjected"} {
		if _, err := PrepareTimeZone(value); !errors.Is(err, ErrInvalidSettings) {
			t.Fatalf("PrepareTimeZone(%q) error = %v", value, err)
		}
	}
}

func TestWebsiteLocalizationRequiresEnabledBuiltinDefaultAndRetainsDisabledOverrides(t *testing.T) {
	settings := WebsiteLocalization{
		DefaultLocale:  "zh-tw",
		EnabledLocales: []string{"en-US", "zh-tw", "en-US"},
		PublicCopyOverrides: map[string]map[string]localization.PublicCopyOverride{
			"catalog.title": {
				"fr-FR": {Key: "catalog.title", Locale: "fr-FR", Value: "Catalogue", DefinitionVersion: 1},
			},
		},
	}
	if err := settings.Prepare(); err != nil {
		t.Fatal(err)
	}
	if settings.DefaultLocale != "zh-TW" || len(settings.EnabledLocales) != 2 {
		t.Fatalf("normalized settings = %+v", settings)
	}
	settings.DefaultLocale = "ja-JP"
	if err := settings.Prepare(); err != ErrInvalidSettings {
		t.Fatalf("disabled default error = %v", err)
	}
}
