package catalog

import "prods/internal/specnumeric"

type SpecNumericProfile struct {
	SpecID              string              `json:"spec_id"`
	SpecSemanticVersion int64               `json:"spec_semantic_version"`
	Revision            int64               `json:"revision"`
	Profile             specnumeric.Profile `json:"profile"`
}

type NormalizationSelection struct {
	SpecValueID               string           `json:"spec_value_id"`
	SelectedNormalizedValueID string           `json:"selected_normalized_value_id"`
	Preference                NormalizedSource `json:"preference"`
	Revision                  int64            `json:"revision"`
}

type SpecSourceSelection struct {
	ProductID   string `json:"product_id"`
	SpecID      string `json:"spec_id"`
	SpecValueID string `json:"spec_value_id"`
	Revision    int64  `json:"revision"`
}

// ProductSpecSource names the persisted shared Product+Spec mapping.
// It is the same command result model, not a second representation.
type ProductSpecSource = SpecSourceSelection

type NormalizationExpectation struct {
	ProductRevision     int64 `json:"product_revision"`
	SourceRevision      int64 `json:"source_revision"`
	SpecSemanticVersion int64 `json:"spec_semantic_version"`
	ProfileRevision     int64 `json:"profile_revision"`
	SelectionRevision   int64 `json:"selection_revision"`
}

type NormalizeSpecCommand struct {
	SpecValueID string                   `json:"spec_value_id"`
	Expected    NormalizationExpectation `json:"expected"`
}

type AdoptSpecCommand struct {
	SpecValueID string                   `json:"spec_value_id"`
	Expected    NormalizationExpectation `json:"expected"`
	ValueJSON   string                   `json:"value_json"`
}

type SelectAutomaticSpecCommand struct {
	SpecValueID string                   `json:"spec_value_id"`
	Expected    NormalizationExpectation `json:"expected"`
	CandidateID string                   `json:"candidate_id"`
}

type SelectSpecSourceCommand struct {
	ProductID                       string `json:"product_id"`
	SpecID                          string `json:"spec_id"`
	SpecValueID                     string `json:"spec_value_id"`
	ExpectedProductRevision         int64  `json:"expected_product_revision"`
	ExpectedSourceRevision          int64  `json:"expected_source_revision"`
	ExpectedSourceSelectionRevision int64  `json:"expected_source_selection_revision"`
}

type SpecNormalizationContext struct {
	SpecValueID     string                   `json:"spec_value_id"`
	ProductID       string                   `json:"product_id"`
	SpecID          string                   `json:"spec_id"`
	Expected        NormalizationExpectation `json:"expected"`
	Raw             string                   `json:"raw"`
	SourceLocale    string                   `json:"source_locale"`
	Profile         specnumeric.Profile      `json:"profile"`
	Selection       NormalizationSelection   `json:"selection"`
	SourceSelection ProductSpecSource        `json:"source_selection"`
}

type NormalizationOutcome struct {
	State              string `json:"state"`
	ReasonCode         string `json:"reason_code"`
	NormalizedValueID  string `json:"normalized_value_id"`
	ProductRevision    int64  `json:"product_revision"`
	SelectionRevision  int64  `json:"selection_revision"`
	PublicationPending bool   `json:"publication_pending"`
}
