package localization

import (
	"strings"

	"golang.org/x/text/language"
)

type Provenance string

const (
	ProvenanceRequested   Provenance = "requested"
	ProvenanceSiteDefault Provenance = "site_default"
	ProvenanceSource      Provenance = "source"
	ProvenanceOfficial    Provenance = "official_default"
	ProvenanceOverride    Provenance = "customer_override"
	ProvenanceAbsent      Provenance = "absent"
)

type ResolvedValue struct {
	Value           string     `json:"value"`
	RequestedLocale string     `json:"requested_locale"`
	EffectiveLocale string     `json:"effective_locale,omitempty"`
	Provenance      Provenance `json:"provenance"`
}

func NormalizeEnabledLocales(defaultLocale string, enabled []string) (string, []string, error) {
	defaultLocale, ok := NormalizeBuiltinLocale(defaultLocale)
	if !ok {
		return "", nil, ErrUnsupportedLocale
	}
	seen := make(map[string]struct{}, len(enabled))
	result := make([]string, 0, len(enabled))
	for _, value := range enabled {
		locale, supported := NormalizeBuiltinLocale(value)
		if !supported {
			return "", nil, ErrUnsupportedLocale
		}
		if _, exists := seen[locale]; exists {
			continue
		}
		seen[locale] = struct{}{}
		result = append(result, locale)
	}
	if len(result) == 0 {
		return "", nil, ErrUnsupportedLocale
	}
	if _, exists := seen[defaultLocale]; !exists {
		return "", nil, ErrUnsupportedLocale
	}
	return defaultLocale, result, nil
}

func ResolvePublishedLocale(defaultLocale string, enabled []string, explicit, acceptLanguage string) (string, error) {
	defaultLocale, enabled, err := NormalizeEnabledLocales(defaultLocale, enabled)
	if err != nil {
		return "", err
	}
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		locale, supported := NormalizeBuiltinLocale(explicit)
		if !supported || !containsLocale(enabled, locale) {
			return "", ErrUnsupportedLocale
		}
		return locale, nil
	}
	tags := make([]language.Tag, 0, len(enabled))
	for _, locale := range enabled {
		tag, _ := language.Parse(locale)
		tags = append(tags, tag)
	}
	matcher := language.NewMatcher(tags)
	for _, header := range strings.Split(acceptLanguage, ",") {
		value := strings.TrimSpace(strings.SplitN(header, ";", 2)[0])
		if value == "" || value == "*" {
			continue
		}
		tag, parseErr := language.Parse(value)
		if parseErr != nil {
			continue
		}
		_, index, confidence := matcher.Match(tag)
		if confidence != language.No {
			return enabled[index], nil
		}
	}
	return defaultLocale, nil
}

func ResolveCustomerValue(requestedLocale, siteDefault, sourceLocale string, enabled []string, values map[string]string) ResolvedValue {
	requestedLocale, _ = NormalizeBuiltinLocale(requestedLocale)
	siteDefault, enabled, err := NormalizeEnabledLocales(siteDefault, enabled)
	if err != nil {
		return ResolvedValue{RequestedLocale: requestedLocale, Provenance: ProvenanceAbsent}
	}
	sourceLocale, _ = NormalizeBuiltinLocale(sourceLocale)
	candidates := []struct {
		locale     string
		provenance Provenance
	}{
		{requestedLocale, ProvenanceRequested},
		{siteDefault, ProvenanceSiteDefault},
		{sourceLocale, ProvenanceSource},
	}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.locale == "" || !containsLocale(enabled, candidate.locale) {
			continue
		}
		if _, duplicate := seen[candidate.locale]; duplicate {
			continue
		}
		seen[candidate.locale] = struct{}{}
		if value := strings.TrimSpace(values[candidate.locale]); value != "" {
			return ResolvedValue{Value: value, RequestedLocale: requestedLocale, EffectiveLocale: candidate.locale, Provenance: candidate.provenance}
		}
	}
	return ResolvedValue{RequestedLocale: requestedLocale, Provenance: ProvenanceAbsent}
}

func ResolvePublicCopy(key, requestedLocale, siteDefault string, enabled []string, overrides PublicCopyOverrideMap, defaults []PublicCopyDefault) ResolvedValue {
	requestedLocale, _ = NormalizeBuiltinLocale(requestedLocale)
	siteDefault, enabled, err := NormalizeEnabledLocales(siteDefault, enabled)
	if err != nil {
		return ResolvedValue{RequestedLocale: requestedLocale, Provenance: ProvenanceAbsent}
	}
	defaultValues := make(map[string]string)
	for _, item := range defaults {
		if item.Key == key {
			defaultValues[item.Locale] = item.Value
		}
	}
	candidates := []string{requestedLocale, siteDefault, "en-US"}
	seen := make(map[string]struct{}, len(candidates))
	for _, locale := range candidates {
		if locale == "" || !containsLocale(enabled, locale) {
			continue
		}
		if _, duplicate := seen[locale]; duplicate {
			continue
		}
		seen[locale] = struct{}{}
		if byLocale := overrides[key]; byLocale != nil {
			if item, exists := byLocale[locale]; exists && !item.NeedsReview && strings.TrimSpace(item.Value) != "" {
				return ResolvedValue{Value: item.Value, RequestedLocale: requestedLocale, EffectiveLocale: locale, Provenance: ProvenanceOverride}
			}
		}
		if value := strings.TrimSpace(defaultValues[locale]); value != "" {
			return ResolvedValue{Value: value, RequestedLocale: requestedLocale, EffectiveLocale: locale, Provenance: ProvenanceOfficial}
		}
	}
	return ResolvedValue{RequestedLocale: requestedLocale, Provenance: ProvenanceAbsent}
}

func containsLocale(locales []string, target string) bool {
	for _, locale := range locales {
		if locale == target {
			return true
		}
	}
	return false
}
