package localization

import "testing"

func TestBuiltinLocaleRegistryIsPinnedAndNormalizesTags(t *testing.T) {
	want := []string{"en-US", "zh-TW", "zh-CN", "ja-JP", "ko-KR", "de-DE", "fr-FR", "it-IT", "es-ES", "pt-BR"}
	wantNames := []string{"English", "繁中", "简中", "日本語", "한국어", "Deutsch", "Français", "Italiano", "Español", "Português (Brasil)"}
	got := BuiltinLocaleCodes()
	if len(got) != len(want) {
		t.Fatalf("locale count = %d, want %d: %v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("locale[%d] = %q, want %q", index, got[index], want[index])
		}
		if gotName := BuiltinLocaleName(got[index]); gotName != wantNames[index] {
			t.Fatalf("locale name[%d] = %q, want %q", index, gotName, wantNames[index])
		}
	}
	if gotName := BuiltinLocaleName("unsupported"); gotName != "unsupported" {
		t.Fatalf("unsupported locale name = %q", gotName)
	}
	if normalized, ok := NormalizeBuiltinLocale("zh-tw"); !ok || normalized != "zh-TW" {
		t.Fatalf("normalize zh-tw = %q, %v", normalized, ok)
	}
	if _, ok := NormalizeBuiltinLocale("nl-NL"); ok {
		t.Fatal("unsupported locale accepted")
	}
}

func TestPublicCopyDefinitionRequiresAllowedRequiredPlaceholders(t *testing.T) {
	definition := PublicCopyDefinition{
		Key:                  "rfq.received",
		DefinitionVersion:    1,
		Description:          "RFQ success message",
		ValueKind:            PublicCopyPlain,
		RequiredPlaceholders: []string{"reference"},
		AllowedPlaceholders:  []string{"reference", "reference"},
		OfficialBundle:       OfficialBundleVersion,
	}
	if err := definition.Prepare(); err != nil {
		t.Fatal(err)
	}
	if len(definition.AllowedPlaceholders) != 1 {
		t.Fatalf("allowed placeholders = %v", definition.AllowedPlaceholders)
	}
	definition.RequiredPlaceholders = []string{"missing"}
	if err := definition.Prepare(); err != ErrInvalidPublicCopy {
		t.Fatalf("invalid placeholder error = %v", err)
	}
}
