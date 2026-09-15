package localization

import "testing"

func TestPublicCopyValidationRejectsMarkupAndPlaceholderDrift(t *testing.T) {
	definition := PublicCopyDefinition{
		Key: "rfq.success", DefinitionVersion: 1, Description: "RFQ success",
		ValueKind: PublicCopyPlain, RequiredPlaceholders: []string{"reference"},
		AllowedPlaceholders: []string{"reference"}, OfficialBundle: OfficialBundleVersion,
	}
	for _, invalid := range []string{
		"Received", "Received {unknown}", `<script>alert(1)</script> {reference}`,
	} {
		if err := ValidatePublicCopyValue(definition, invalid); err == nil {
			t.Fatalf("invalid public copy accepted: %q", invalid)
		}
	}
	result, err := RenderPublicCopy(definition, "en-US", "Received {reference}", map[string]string{"reference": "RFQ-1"}, 0)
	if err != nil || result != "Received RFQ-1" {
		t.Fatalf("render = %q, %v", result, err)
	}
}

func TestPublicCopyPluralUsesLocaleRulesAndRequiredOtherBranch(t *testing.T) {
	definition := PublicCopyDefinition{
		Key: "search.count", DefinitionVersion: 1, Description: "Search result count",
		ValueKind: PublicCopyPlural, RequiredPlaceholders: []string{"count"},
		AllowedPlaceholders: []string{"count"}, OfficialBundle: OfficialBundleVersion,
	}
	if err := ValidatePublicCopyValue(definition, `{"one":"{count} result"}`); err == nil {
		t.Fatal("plural without other branch was accepted")
	}
	value := `{"one":"{count} result","other":"{count} results"}`
	english, err := RenderPublicCopy(definition, "en-US", value, map[string]string{"count": "1"}, 1)
	if err != nil || english != "1 result" {
		t.Fatalf("English plural = %q, %v", english, err)
	}
	japanese, err := RenderPublicCopy(definition, "ja-JP", value, map[string]string{"count": "1"}, 1)
	if err != nil || japanese != "1 results" {
		t.Fatalf("Japanese plural must use other branch = %q, %v", japanese, err)
	}
}
