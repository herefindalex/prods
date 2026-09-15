package localization

import "testing"

func TestResolveValidatesExplicitLocaleAndNegotiatesHeader(t *testing.T) {
	defaultLocale, supported, err := NormalizeSupported("en-us", []string{"en-US", "zh-tw", "en-US"})
	if err != nil || defaultLocale != "en-US" || len(supported) != 2 || supported[1] != "zh-TW" {
		t.Fatalf("normalized default=%q supported=%v err=%v", defaultLocale, supported, err)
	}
	locale, err := Resolve(defaultLocale, supported, "zh-tw", "")
	if err != nil || locale != "zh-TW" {
		t.Fatalf("explicit locale=%q err=%v", locale, err)
	}
	locale, err = Resolve(defaultLocale, supported, "", "zh-Hant-TW, en;q=0.8")
	if err != nil || locale != "zh-TW" {
		t.Fatalf("negotiated locale=%q err=%v", locale, err)
	}
	if _, err := Resolve(defaultLocale, supported, "ja-JP", ""); err != ErrUnsupportedLocale {
		t.Fatalf("unsupported error=%v", err)
	}
	if For("zh-TW").Search != "搜尋" || For("en-US").Search != "Search" {
		t.Fatal("message catalog selection failed")
	}
}

func TestNormalizeSupportedRejectsLocalesWithoutAnInterfaceCatalog(t *testing.T) {
	if _, _, err := NormalizeSupported("fr-FR", []string{"fr-FR"}); err != ErrUnsupportedLocale {
		t.Fatalf("French-only interface locale error = %v, want %v", err, ErrUnsupportedLocale)
	}
	if IsAvailable("fr-FR") {
		t.Fatal("fr-FR must not be advertised without an embedded interface catalog")
	}
	available := Available()
	if len(available) != 2 || available[0] != "en-US" || available[1] != "zh-TW" {
		t.Fatalf("available interface locales = %v", available)
	}
}
