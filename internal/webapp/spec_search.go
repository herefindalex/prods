package webapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"prods/internal/publishing"
	"prods/internal/specnumeric"
)

// Public search is a bounded read command, independent of Admin sessions.
func (s *Server) publicSpecSearch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Predicates     []specSearchPredicate `json:"predicates"`
		CategoryID     string                `json:"category_id"`
		ManufacturerID string                `json:"manufacturer_id"`
		Locale         string                `json:"locale"`
		Page           int                   `json:"page"`
		PageSize       int                   `json:"page_size"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || len(request.Predicates) < 1 || len(request.Predicates) > 16 || request.Page < 1 || request.PageSize < 1 || request.PageSize > 100 {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	predicates := make([]specnumeric.Predicate, len(request.Predicates))
	for i, wire := range request.Predicates {
		predicate, err := wire.predicate()
		if err != nil {
			s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
			return
		}
		predicates[i] = predicate
	}
	if s.publisher == nil {
		s.writeAPIError(w, r, http.StatusServiceUnavailable, apiCodeServiceUnavailable)
		return
	}
	website, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	active := website.ActiveLocalization
	if request.Locale != "" && !slices.Contains(active.EnabledLocales, request.Locale) {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeValidationFailed)
		return
	}
	result, err := s.publisher.SearchSpecs(r.Context(), publishing.SpecSearchRequest{Predicates: predicates, CategoryID: request.CategoryID, ManufacturerID: request.ManufacturerID, Locale: request.Locale, Page: request.Page, PageSize: request.PageSize})
	switch {
	case errors.Is(err, publishing.ErrUnsupportedSpecSearch):
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
	case errors.Is(err, publishing.ErrInvalidSpecSearch):
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
	case errors.Is(err, publishing.ErrPublicUnavailable):
		s.writeAPIError(w, r, http.StatusServiceUnavailable, apiCodeServiceUnavailable)
	case err != nil:
		s.internalAPIError(w, r, err)
	default:
		writeJSON(w, http.StatusOK, result)
	}
}

// Preserve key presence until the closed operand union has been validated.
// A pointer-only DTO cannot distinguish a forbidden null key from absence.
type specSearchPredicate struct {
	SpecID   string               `json:"spec_id"`
	Operator specnumeric.Operator `json:"operator"`
	Value    json.RawMessage      `json:"value"`
	Lower    json.RawMessage      `json:"lower"`
	Upper    json.RawMessage      `json:"upper"`
	Unit     string               `json:"unit"`
}

func (wire specSearchPredicate) predicate() (specnumeric.Predicate, error) {
	present := func(value json.RawMessage) bool {
		return len(value) > 0 && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
	}
	invalid := false
	switch wire.Operator {
	case specnumeric.OperatorBetween, specnumeric.OperatorContainsRange, specnumeric.OperatorOverlapsRange:
		invalid = wire.Value != nil || !present(wire.Lower) || !present(wire.Upper)
	case specnumeric.OperatorContainsPoint, specnumeric.OperatorEqual, specnumeric.OperatorLess, specnumeric.OperatorLessEqual, specnumeric.OperatorGreater, specnumeric.OperatorGreaterEqual:
		invalid = wire.Lower != nil || wire.Upper != nil || !present(wire.Value)
	}
	if invalid {
		return specnumeric.Predicate{}, errors.New("invalid predicate operand fields")
	}
	predicate := specnumeric.Predicate{SpecID: wire.SpecID, Operator: wire.Operator, Unit: wire.Unit}
	for _, operand := range []struct {
		encoded json.RawMessage
		target  **specnumeric.Decimal
	}{
		{wire.Value, &predicate.Value}, {wire.Lower, &predicate.Lower}, {wire.Upper, &predicate.Upper},
	} {
		if len(operand.encoded) > 0 {
			if err := json.Unmarshal(operand.encoded, operand.target); err != nil {
				return specnumeric.Predicate{}, err
			}
		}
	}
	return predicate, nil
}
