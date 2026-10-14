package webapp

import (
	"io"
	"net/http"
	"strings"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/specnumeric"
)

func (s *Server) adminConfigureNumericProfile(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                `json:"expected_revision"`
		Profile          *specnumeric.Profile `json:"profile"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || request.ExpectedRevision < 1 || request.Profile == nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	result, err := s.store.ConfigureSpecNumericProfile(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, *request.Profile)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) adminSpecNormalizationContext(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	result, err := s.store.SpecNormalizationContext(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func validNormalizationExpectation(expected catalog.NormalizationExpectation) bool {
	return expected.ProductRevision > 0 && expected.SourceRevision > 0 && expected.SpecSemanticVersion > 0 && expected.ProfileRevision > 0 && expected.SelectionRevision >= 0
}
func (s *Server) adminNormalizeSpec(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		Expected catalog.NormalizationExpectation `json:"expected"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || !validNormalizationExpectation(request.Expected) {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	c, err := s.store.SpecNormalizationContext(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	if c.Expected != request.Expected {
		s.writeCatalogError(w, r, catalog.ErrRevisionConflict)
		return
	}
	// Parsing uses the frozen server context outside writer admission. Record
	// rechecks every expected version within the admitted transaction.
	result := specnumeric.Normalize(c.Raw, c.Profile)
	outcome, err := s.store.RecordSpecNormalization(r.Context(), current.UserID, catalog.NormalizeSpecCommand{SpecValueID: c.SpecValueID, Expected: request.Expected}, result)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, outcome)
}
func (s *Server) adminAdoptSpec(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		Expected  catalog.NormalizationExpectation `json:"expected"`
		ValueJSON string                           `json:"value_json"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || !validNormalizationExpectation(request.Expected) {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	c, err := s.store.SpecNormalizationContext(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	// Conflict precedence must survive even a now-invalid payload/profile.
	if c.Expected != request.Expected {
		s.writeCatalogError(w, r, catalog.ErrRevisionConflict)
		return
	}
	var value specnumeric.Value
	if err := decodeJSON(io.NopCloser(strings.NewReader(request.ValueJSON)), &value); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	unsupportedProfile := value.Unit != "" && value.Unit != c.Profile.CanonicalUnit ||
		value.Quantity != "" && value.Quantity != c.Profile.Quantity ||
		value.SemanticRole != "" && value.SemanticRole != c.Profile.SemanticRole ||
		value.Shape != "" && !c.Profile.AllowsShape(value.Shape)
	if unsupportedProfile {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	if _, err := specnumeric.UnmarshalValue(request.ValueJSON, c.Profile); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	outcome, err := s.store.AdoptManualSpecValue(r.Context(), current.UserID, catalog.AdoptSpecCommand{SpecValueID: c.SpecValueID, Expected: request.Expected, ValueJSON: request.ValueJSON})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, outcome)
}
func (s *Server) adminSelectAutomaticSpec(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		Expected    catalog.NormalizationExpectation `json:"expected"`
		CandidateID string                           `json:"candidate_id"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || !validNormalizationExpectation(request.Expected) || request.CandidateID == "" {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	outcome, err := s.store.SelectAutomaticSpecValue(r.Context(), current.UserID, catalog.SelectAutomaticSpecCommand{SpecValueID: r.PathValue("id"), Expected: request.Expected, CandidateID: request.CandidateID})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, outcome)
}
func (s *Server) adminSelectSpecSource(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		SpecID                          string `json:"spec_id"`
		SpecValueID                     string `json:"spec_value_id"`
		ExpectedProductRevision         int64  `json:"expected_product_revision"`
		ExpectedSourceRevision          int64  `json:"expected_source_revision"`
		ExpectedSourceSelectionRevision int64  `json:"expected_source_selection_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || request.SpecID == "" || request.SpecValueID == "" || request.ExpectedProductRevision < 1 || request.ExpectedSourceRevision < 1 || request.ExpectedSourceSelectionRevision < 0 {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	result, err := s.store.SelectSpecSource(r.Context(), current.UserID, catalog.SelectSpecSourceCommand{ProductID: r.PathValue("id"), SpecID: request.SpecID, SpecValueID: request.SpecValueID, ExpectedProductRevision: request.ExpectedProductRevision, ExpectedSourceRevision: request.ExpectedSourceRevision, ExpectedSourceSelectionRevision: request.ExpectedSourceSelectionRevision})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
