package localization

import (
	"strings"
	"testing"
)

func TestOfficialPublicCopyIsCompleteForEveryBuiltinLocale(t *testing.T) {
	catalog := OfficialPublicCopyCatalog()
	if catalog.OfficialBundle != OfficialBundleVersion {
		t.Fatalf("bundle = %q", catalog.OfficialBundle)
	}
	if !strings.Contains(catalog.ReviewStatus, "not reviewed") {
		t.Fatalf("review status must disclose review limit: %q", catalog.ReviewStatus)
	}
	wantDefaults := len(catalog.Definitions) * len(BuiltinLocaleCodes())
	if len(catalog.Definitions) == 0 || len(catalog.Defaults) != wantDefaults {
		t.Fatalf("definitions=%d defaults=%d want=%d", len(catalog.Definitions), len(catalog.Defaults), wantDefaults)
	}
	seen := make(map[string]map[string]bool)
	for index := range catalog.Definitions {
		definition := catalog.Definitions[index]
		if err := definition.Prepare(); err != nil {
			t.Fatalf("definition %q: %v", definition.Key, err)
		}
		if _, duplicate := seen[definition.Key]; duplicate {
			t.Fatalf("duplicate key %q", definition.Key)
		}
		seen[definition.Key] = make(map[string]bool)
	}
	for _, item := range catalog.Defaults {
		if strings.TrimSpace(item.Value) == "" {
			t.Fatalf("empty default %s/%s", item.Key, item.Locale)
		}
		if seen[item.Key] == nil {
			t.Fatalf("default has unknown key %q", item.Key)
		}
		if seen[item.Key][item.Locale] {
			t.Fatalf("duplicate default %s/%s", item.Key, item.Locale)
		}
		seen[item.Key][item.Locale] = true
	}
	for key, locales := range seen {
		for _, locale := range BuiltinLocaleCodes() {
			if !locales[locale] {
				t.Fatalf("missing default %s/%s", key, locale)
			}
		}
	}
}
