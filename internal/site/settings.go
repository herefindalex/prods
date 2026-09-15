package site

import (
	"errors"
	"strings"
	"time"
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
