package catalog

import "prods/internal/catalogfilter"

type SpecSearchProfile struct {
	SpecID              string                `json:"spec_id"`
	SpecSemanticVersion int64                 `json:"spec_semantic_version"`
	Revision            int64                 `json:"revision"`
	Filterable          bool                  `json:"filterable"`
	Profile             catalogfilter.Profile `json:"profile"`
}

type SpecNormalizationWork struct {
	ID              string `json:"id"`
	SpecID          string `json:"spec_id"`
	Status          string `json:"status"`
	Cursor          string `json:"cursor"`
	ProfileRevision int64  `json:"profile_revision"`
	SemanticVersion int64  `json:"semantic_version"`
	Processed       int64  `json:"processed"`
	Failed          int64  `json:"failed"`
	Skipped         int64  `json:"skipped"`
	Total           int64  `json:"total"`
}
type SpecNormalizationWorkItem struct {
	SpecValueID string `json:"spec_value_id"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
}
