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
