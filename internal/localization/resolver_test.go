package localization

import "testing"

func TestCustomerValueResolverSkipsDisabledCandidatesAndReportsProvenance(t *testing.T) {
	values := map[string]string{"en-US": "Power", "zh-TW": "電源"}
	resolved := ResolveCustomerValue("fr-FR", "en-US", "zh-TW", []string{"en-US", "fr-FR"}, values)
	if resolved.Value != "Power" || resolved.EffectiveLocale != "en-US" || resolved.Provenance != ProvenanceSiteDefault {
		t.Fatalf("resolved = %+v", resolved)
	}
	resolved = ResolveCustomerValue("fr-FR", "fr-FR", "zh-TW", []string{"fr-FR"}, values)
	if resolved.Value != "" || resolved.EffectiveLocale != "" || resolved.Provenance != ProvenanceAbsent {
		t.Fatalf("disabled source leaked = %+v", resolved)
	}
}

func TestPublicCopyResolverUsesOverrideDefaultChainWithoutDisabledEmergencyFallback(t *testing.T) {
	defaults := []PublicCopyDefault{
		{Key: "catalog.title", Locale: "en-US", Value: "Catalog"},
		{Key: "catalog.title", Locale: "fr-FR", Value: "Catalogue"},
	}
	overrides := PublicCopyOverrideMap{
		"catalog.title": {"fr-FR": {Key: "catalog.title", Locale: "fr-FR", Value: "Produits", DefinitionVersion: 1}},
	}
	resolved := ResolvePublicCopy("catalog.title", "fr-FR", "en-US", []string{"en-US", "fr-FR"}, overrides, defaults)
	if resolved.Value != "Produits" || resolved.EffectiveLocale != "fr-FR" || resolved.Provenance != ProvenanceOverride {
		t.Fatalf("resolved = %+v", resolved)
	}
	resolved = ResolvePublicCopy("catalog.title", "fr-FR", "fr-FR", []string{"fr-FR"}, nil, []PublicCopyDefault{{Key: "catalog.title", Locale: "en-US", Value: "Catalog"}})
	if resolved.Provenance != ProvenanceAbsent {
		t.Fatalf("disabled en-US fallback leaked = %+v", resolved)
	}
}

func TestResolvePublishedLocaleRejectsDisabledExplicitAndNegotiatesEnabled(t *testing.T) {
	if _, err := ResolvePublishedLocale("en-US", []string{"en-US", "fr-FR"}, "zh-TW", ""); err != ErrUnsupportedLocale {
		t.Fatalf("disabled explicit error = %v", err)
	}
	locale, err := ResolvePublishedLocale("en-US", []string{"en-US", "fr-FR"}, "", "fr-CA, en;q=0.8")
	if err != nil || locale != "fr-FR" {
		t.Fatalf("negotiated = %q, %v", locale, err)
	}
}
