package localization

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholderPattern = regexp.MustCompile(`\{([A-Za-z][A-Za-z0-9_]*)\}`)

func ValidatePublicCopyValue(definition PublicCopyDefinition, value string) error {
	if err := definition.Prepare(); err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "<>") {
		return ErrInvalidPublicCopy
	}
	branches := map[string]string{"other": value}
	if definition.ValueKind == PublicCopyPlural || definition.ValueKind == PublicCopySelect {
		branches = make(map[string]string)
		if err := json.Unmarshal([]byte(value), &branches); err != nil || len(branches) == 0 || strings.TrimSpace(branches["other"]) == "" {
			return ErrInvalidPublicCopy
		}
	}
	allowed := make(map[string]bool, len(definition.AllowedPlaceholders))
	for _, name := range definition.AllowedPlaceholders {
		allowed[name] = true
	}
	for branch, text := range branches {
		if strings.TrimSpace(branch) == "" || strings.TrimSpace(text) == "" || strings.ContainsAny(text, "<>") {
			return ErrInvalidPublicCopy
		}
		seen := make(map[string]bool)
		for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			name := match[1]
			if !allowed[name] {
				return ErrInvalidPublicCopy
			}
			seen[name] = true
		}
		for _, name := range definition.RequiredPlaceholders {
			if !seen[name] {
				return ErrInvalidPublicCopy
			}
		}
	}
	return nil
}

func ValidatePublicCopyOverride(definition PublicCopyDefinition, override PublicCopyOverride) error {
	locale, ok := NormalizeBuiltinLocale(override.Locale)
	if !ok || locale != override.Locale || strings.TrimSpace(override.Key) != definition.Key || override.DefinitionVersion != definition.DefinitionVersion {
		return ErrInvalidPublicCopy
	}
	return ValidatePublicCopyValue(definition, override.Value)
}

func RenderPublicCopy(definition PublicCopyDefinition, locale, value string, variables map[string]string, count int) (string, error) {
	if err := ValidatePublicCopyValue(definition, value); err != nil {
		return "", err
	}
	selected := value
	if definition.ValueKind == PublicCopyPlural || definition.ValueKind == PublicCopySelect {
		branches := make(map[string]string)
		if err := json.Unmarshal([]byte(value), &branches); err != nil {
			return "", ErrInvalidPublicCopy
		}
		branch := "other"
		if definition.ValueKind == PublicCopyPlural {
			if exact := branches["="+strconv.Itoa(count)]; exact != "" {
				selected = exact
			} else {
				branch = pluralBranch(locale, count)
			}
		} else if candidate := variables["select"]; candidate != "" {
			branch = candidate
		}
		if selected == value {
			selected = branches[branch]
			if selected == "" {
				selected = branches["other"]
			}
		}
	}
	missing := ""
	result := placeholderPattern.ReplaceAllStringFunc(selected, func(token string) string {
		name := token[1 : len(token)-1]
		value, ok := variables[name]
		if !ok {
			missing = name
			return token
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("%w: missing placeholder %s", ErrInvalidPublicCopy, missing)
	}
	return result, nil
}

func pluralBranch(locale string, count int) string {
	switch locale {
	case "zh-TW", "zh-CN", "ja-JP", "ko-KR":
		return "other"
	case "fr-FR", "pt-BR":
		if count == 0 || count == 1 {
			return "one"
		}
	default:
		if count == 1 {
			return "one"
		}
	}
	return "other"
}
