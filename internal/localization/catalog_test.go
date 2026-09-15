package localization

import (
	"reflect"
	"testing"
)

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

func TestEveryBuiltinLocaleHasACompleteInterfaceCatalog(t *testing.T) {
	want := BuiltinLocaleCodes()
	available := Available()
	if !reflect.DeepEqual(available, want) {
		t.Fatalf("available interface locales = %v, want %v", available, want)
	}
	for _, locale := range want {
		if !IsAvailable(locale) {
			t.Fatalf("locale %s is not available", locale)
		}
		messages := reflect.ValueOf(For(locale))
		for index := 0; index < messages.NumField(); index++ {
			if messages.Field(index).String() == "" {
				t.Fatalf("locale %s has empty message %s", locale, messages.Type().Field(index).Name)
			}
		}
		defaultLocale, supported, err := NormalizeSupported(locale, []string{locale})
		if err != nil || defaultLocale != locale || len(supported) != 1 || supported[0] != locale {
			t.Fatalf("locale %s normalization = %q %v %v", locale, defaultLocale, supported, err)
		}
	}
}
