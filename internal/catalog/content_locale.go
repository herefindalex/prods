package catalog

import (
	"errors"
	"strings"

	"prods/internal/localization"
)

var ErrInvalidContentLocale = errors.New("invalid customer content source locale")

var ProductTranslatableFields = []string{"name", "description", "features", "specification"}
var TaxonomyTranslatableFields = []string{"name", "description"}

func NormalizeFieldSourceLocales(values map[string]string, fallback string, allowedFields []string) (map[string]string, error) {
	fallback, ok := localization.NormalizeBuiltinLocale(fallback)
	if !ok {
		return nil, ErrInvalidContentLocale
	}
	allowed := make(map[string]struct{}, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{}{}
	}
	result := make(map[string]string, len(allowedFields))
	for _, field := range allowedFields {
		result[field] = fallback
	}
	for field, value := range values {
		field = strings.TrimSpace(field)
		if _, exists := allowed[field]; !exists {
			return nil, ErrInvalidContentLocale
		}
		locale, supported := localization.NormalizeBuiltinLocale(value)
		if !supported {
			return nil, ErrInvalidContentLocale
		}
		result[field] = locale
	}
	return result, nil
}
