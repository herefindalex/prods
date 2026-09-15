package localization

import (
	"sort"
	"strings"

	"golang.org/x/text/language"
)

const OfficialBundleVersion = "v1"

type LocaleDefinition struct {
	Code       string `json:"code"`
	NativeName string `json:"native_name"`
	SortOrder  int    `json:"sort_order"`
}

var builtinLocaleRegistry = []LocaleDefinition{
	{Code: "en-US", NativeName: "English (United States)", SortOrder: 10},
	{Code: "zh-TW", NativeName: "繁體中文", SortOrder: 20},
	{Code: "zh-CN", NativeName: "简体中文", SortOrder: 30},
	{Code: "ja-JP", NativeName: "日本語", SortOrder: 40},
	{Code: "ko-KR", NativeName: "한국어", SortOrder: 50},
	{Code: "de-DE", NativeName: "Deutsch", SortOrder: 60},
	{Code: "fr-FR", NativeName: "Français", SortOrder: 70},
	{Code: "it-IT", NativeName: "Italiano", SortOrder: 80},
	{Code: "es-ES", NativeName: "Español", SortOrder: 90},
	{Code: "pt-BR", NativeName: "Português (Brasil)", SortOrder: 100},
}

func BuiltinLocales() []LocaleDefinition {
	result := append([]LocaleDefinition(nil), builtinLocaleRegistry...)
	sort.Slice(result, func(i, j int) bool { return result[i].SortOrder < result[j].SortOrder })
	return result
}

func BuiltinLocaleCodes() []string {
	locales := BuiltinLocales()
	result := make([]string, 0, len(locales))
	for _, locale := range locales {
		result = append(result, locale.Code)
	}
	return result
}

func NormalizeBuiltinLocale(value string) (string, bool) {
	tag, err := language.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", false
	}
	normalized := tag.String()
	for _, locale := range builtinLocaleRegistry {
		if locale.Code == normalized {
			return normalized, true
		}
	}
	return "", false
}
