package localization

import (
	"errors"
	"sort"
	"strings"
)

var ErrInvalidPublicCopy = errors.New("invalid public copy")

type PublicCopyValueKind string

const (
	PublicCopyPlain  PublicCopyValueKind = "plain"
	PublicCopyRich   PublicCopyValueKind = "rich"
	PublicCopyPlural PublicCopyValueKind = "plural"
	PublicCopySelect PublicCopyValueKind = "select"
)

type PublicCopyDefinition struct {
	Key                  string              `json:"key"`
	DefinitionVersion    int64               `json:"definition_version"`
	Description          string              `json:"description"`
	ValueKind            PublicCopyValueKind `json:"value_kind"`
	RequiredPlaceholders []string            `json:"required_placeholders"`
	AllowedPlaceholders  []string            `json:"allowed_placeholders"`
	Sample               map[string]string   `json:"sample,omitempty"`
	OfficialBundle       string              `json:"official_bundle_version"`
}

type PublicCopyOverride struct {
	Key               string `json:"key"`
	Locale            string `json:"locale"`
	Value             string `json:"value"`
	DefinitionVersion int64  `json:"definition_version"`
	NeedsReview       bool   `json:"needs_review,omitempty"`
}

type PublicCopyDefault struct {
	Key               string `json:"key"`
	Locale            string `json:"locale"`
	Value             string `json:"value"`
	DefinitionVersion int64  `json:"definition_version"`
	OfficialBundle    string `json:"official_bundle_version"`
}

type PublicCopyCatalog struct {
	OfficialBundle string                 `json:"official_bundle_version"`
	ReviewStatus   string                 `json:"review_status"`
	Definitions    []PublicCopyDefinition `json:"definitions"`
	Defaults       []PublicCopyDefault    `json:"defaults"`
}

// PublicCopyOverrideMap is keyed by stable copy key and then locale. It is
// serialized as part of the Website working/published revision.
type PublicCopyOverrideMap map[string]map[string]PublicCopyOverride

func (definition *PublicCopyDefinition) Prepare() error {
	definition.Key = strings.TrimSpace(definition.Key)
	definition.Description = strings.TrimSpace(definition.Description)
	definition.OfficialBundle = strings.TrimSpace(definition.OfficialBundle)
	definition.RequiredPlaceholders = normalizedKeys(definition.RequiredPlaceholders)
	definition.AllowedPlaceholders = normalizedKeys(definition.AllowedPlaceholders)
	if definition.Key == "" || definition.DefinitionVersion < 1 || definition.Description == "" || definition.OfficialBundle == "" {
		return ErrInvalidPublicCopy
	}
	switch definition.ValueKind {
	case PublicCopyPlain, PublicCopyRich, PublicCopyPlural, PublicCopySelect:
	default:
		return ErrInvalidPublicCopy
	}
	allowed := make(map[string]struct{}, len(definition.AllowedPlaceholders))
	for _, placeholder := range definition.AllowedPlaceholders {
		allowed[placeholder] = struct{}{}
	}
	for _, placeholder := range definition.RequiredPlaceholders {
		if _, ok := allowed[placeholder]; !ok {
			return ErrInvalidPublicCopy
		}
	}
	return nil
}

func normalizedKeys(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
