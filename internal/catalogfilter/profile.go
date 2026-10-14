package catalogfilter

import (
	"fmt"
	"slices"
	"strings"

	"prods/internal/searchtext"
	"prods/internal/specnumeric"
)

const TextRule = "unicode-fold-space-1"

type Profile struct {
	Version   int                  `json:"version"`
	Kind      string               `json:"kind"`
	TextRule  string               `json:"text_rule,omitempty"`
	Delimiter string               `json:"delimiter,omitempty"`
	Operators []string             `json:"operators"`
	Numeric   *specnumeric.Profile `json:"numeric,omitempty"`
}

func DefaultTextProfile() Profile {
	return Profile{Version: 1, Kind: "text", TextRule: TextRule, Operators: []string{"equals", "contains"}}
}
func NumericProfile(p specnumeric.Profile) Profile {
	result := Profile{Version: 1, Kind: "numeric", Numeric: &p}
	for _, op := range p.AllowedOperators {
		result.Operators = append(result.Operators, string(op))
	}
	return result
}
func ValidateProfile(p Profile) error {
	if p.Version != 1 || len(p.Operators) == 0 {
		return fmt.Errorf("unsupported search profile version or empty operators")
	}
	allowed := []string{}
	switch p.Kind {
	case "text":
		allowed = []string{"equals", "contains"}
	case "choice", "multi_choice":
		allowed = []string{"equals"}
	case "numeric":
		if p.Numeric == nil {
			return fmt.Errorf("numeric profile required")
		}
		if err := specnumeric.ValidateProfile(*p.Numeric); err != nil {
			return err
		}
		for _, op := range p.Numeric.AllowedOperators {
			allowed = append(allowed, string(op))
		}
		if len(allowed) != len(p.Operators) {
			return fmt.Errorf("numeric operators must match their authoritative profile")
		}
	default:
		return fmt.Errorf("unsupported search kind")
	}
	seen := map[string]bool{}
	for _, op := range p.Operators {
		if seen[op] || !slices.Contains(allowed, op) {
			return fmt.Errorf("unsupported or duplicate search operator")
		}
		seen[op] = true
	}
	if p.Kind == "numeric" {
		if p.TextRule != "" || p.Delimiter != "" {
			return fmt.Errorf("numeric profile cannot declare text rules")
		}
	} else {
		if p.Numeric != nil || p.TextRule != TextRule {
			return fmt.Errorf("unsupported text rule")
		}
		if p.Kind == "multi_choice" {
			if strings.TrimSpace(p.Delimiter) == "" || len(p.Delimiter) > 8 {
				return fmt.Errorf("explicit literal delimiter required")
			}
		} else if p.Delimiter != "" {
			return fmt.Errorf("delimiter only applies to declared multiple choices")
		}
	}
	return nil
}

func FoldText(value string) string { return searchtext.Fold(strings.Join(strings.Fields(value), " ")) }
func Values(raw string, p Profile) ([]string, error) {
	if err := ValidateProfile(p); err != nil {
		return nil, err
	}
	if p.Kind == "numeric" {
		return nil, fmt.Errorf("numeric facts require controlled adoption")
	}
	parts := []string{raw}
	if p.Kind == "multi_choice" {
		parts = strings.Split(raw, p.Delimiter)
	}
	var result []string
	for _, part := range parts {
		value := FoldText(part)
		if value != "" && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result, nil
}
