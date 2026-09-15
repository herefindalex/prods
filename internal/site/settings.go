package site

import (
	"errors"
	"strings"
	"time"

	"prods/internal/localization"
)

var (
	ErrInvalidSettings  = errors.New("invalid site settings")
	ErrSettingsConflict = errors.New("site settings revision conflict")
)

type Settings struct {
	DefaultLocale              string   `json:"default_locale"`
	SupportedLocales           []string `json:"supported_locales"`
	ContentMultilingualEnabled bool     `json:"content_multilingual_enabled"`
	TimeZone                   string   `json:"time_zone"`
	Revision                   int64    `json:"revision"`
	UpdatedAt                  string   `json:"updated_at"`
}

// WebsiteLocalization is versioned with the rest of the Website working
// configuration. Settings above remains the installation/runtime compatibility
// record until all callers have migrated; it is not the public activation
// authority.
type WebsiteLocalization struct {
	DefaultLocale         string                             `json:"default_locale"`
	EnabledLocales        []string                           `json:"enabled_locales"`
	ContentEditingEnabled bool                               `json:"content_editing_enabled"`
	PublicCopyOverrides   localization.PublicCopyOverrideMap `json:"public_copy_overrides,omitempty"`
}

func (settings *WebsiteLocalization) Prepare() error {
	defaultLocale, ok := localization.NormalizeBuiltinLocale(settings.DefaultLocale)
	if !ok {
		return ErrInvalidSettings
	}
	seen := make(map[string]struct{}, len(settings.EnabledLocales))
	normalized := make([]string, 0, len(settings.EnabledLocales))
	for _, value := range settings.EnabledLocales {
		locale, supported := localization.NormalizeBuiltinLocale(value)
		if !supported {
			return ErrInvalidSettings
		}
		if _, exists := seen[locale]; exists {
			continue
		}
		seen[locale] = struct{}{}
		normalized = append(normalized, locale)
	}
	if len(normalized) == 0 {
		return ErrInvalidSettings
	}
	if _, exists := seen[defaultLocale]; !exists {
		return ErrInvalidSettings
	}
	settings.DefaultLocale = defaultLocale
	settings.EnabledLocales = normalized
	if settings.PublicCopyOverrides == nil {
		settings.PublicCopyOverrides = make(localization.PublicCopyOverrideMap)
	}
	for key, byLocale := range settings.PublicCopyOverrides {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || trimmedKey != key {
			return ErrInvalidSettings
		}
		for locale, override := range byLocale {
			normalizedLocale, supported := localization.NormalizeBuiltinLocale(locale)
			if !supported || normalizedLocale != locale || override.Key != key || override.Locale != locale || override.DefinitionVersion < 1 {
				return ErrInvalidSettings
			}
		}
	}
	return nil
}

func PrepareTimeZone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidSettings
	}
	location, err := time.LoadLocation(value)
	if err != nil || location.String() != value {
		return "", ErrInvalidSettings
	}
	return value, nil
}
