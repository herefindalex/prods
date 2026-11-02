package publishing

import (
	"prods/internal/catalogfilter"
	"reflect"
	"sort"
)

// PublicFilterDefinition is frozen with the product publication unit.
type PublicFilterDefinition struct {
	Field           string                `json:"field"`
	SemanticVersion int64                 `json:"semantic_version"`
	ProfileRevision int64                 `json:"profile_revision"`
	Profile         catalogfilter.Profile `json:"profile"`
}

// CommonFilterDefinitions uses formal applicability, independent of Raw presence.
func CommonFilterDefinitions(views []PublicView) []PublicFilterDefinition {
	common := map[string]PublicFilterDefinition{}
	if len(views) == 0 {
		return nil
	}
	for _, d := range views[0].FilterDefinitions {
		if catalogfilter.ValidateProfile(d.Profile) == nil {
			common[d.Field] = d
		}
	}
	for _, v := range views[1:] {
		present := map[string]PublicFilterDefinition{}
		for _, d := range v.FilterDefinitions {
			present[d.Field] = d
		}
		for field, d := range common {
			other, ok := present[field]
			if !ok || !reflect.DeepEqual(d, other) {
				delete(common, field)
			}
		}
	}
	result := make([]PublicFilterDefinition, 0, len(common))
	for _, d := range common {
		result = append(result, d)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Field < result[j].Field })
	return result
}
