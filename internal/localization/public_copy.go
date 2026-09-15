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
	WhereUsed            []PublicCopyUsage   `json:"where_used,omitempty"`
}

type PublicCopyUsage struct {
	Surface     string `json:"surface"`
	Section     string `json:"section"`
	Purpose     string `json:"purpose"`
	SafeContext string `json:"safe_context"`
}

func PublicCopyWhereUsed(key string) []PublicCopyUsage {
	scope := key
	if index := strings.IndexByte(key, '.'); index >= 0 {
		scope = key[:index]
	}
	usage := PublicCopyUsage{SafeContext: "Admin-only sample preview; no Customer Content or live RFQ data is loaded."}
	switch scope {
	case "catalog":
		usage.Surface, usage.Section, usage.Purpose = "Public catalog", "page heading and empty-state actions", "Catalog navigation and discovery"
	case "search":
		usage.Surface, usage.Section, usage.Purpose = "Public search", "query form, results, and no-results", "Search guidance and requested-part conversion"
	case "product":
		usage.Surface, usage.Section, usage.Purpose = "Public Product", "semantic Product detail sections", "Field labels and RFQ call to action"
	case "rfq":
		usage.Surface, usage.Section, usage.Purpose = "Public RFQ", "request form and completion receipt", "Submission guidance and durable result acknowledgement"
	case "pagination":
		usage.Surface, usage.Section, usage.Purpose = "Public lists", "pagination navigation", "Move between bounded result pages"
	case "footer":
		usage.Surface, usage.Section, usage.Purpose = "All public pages", "site footer", "Legal and organization navigation"
	case "locale":
		usage.Surface, usage.Section, usage.Purpose = "All localized public pages", "language navigation", "Locale selection label"
	default:
		usage.Surface, usage.Section, usage.Purpose = "Public interface", scope, "Official interface message"
	}
	return []PublicCopyUsage{usage}
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
