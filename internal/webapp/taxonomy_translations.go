package webapp

import (
	"database/sql"
	"errors"
	"net/http"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func (s *Server) adminTaxonomyContent(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	content, err := s.store.TaxonomyContent(r.Context(), r.PathValue("type"), r.PathValue("id"))
	if err != nil {
		s.writeContentLocaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (s *Server) adminSaveTaxonomySourceLocales(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64             `json:"expected_revision"`
		SourceLocale     string            `json:"source_locale"`
		SourceLocales    map[string]string `json:"source_locales"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	content, err := s.store.SaveTaxonomySourceLocales(r.Context(), current.UserID, r.PathValue("type"), r.PathValue("id"), request.ExpectedRevision, request.SourceLocale, request.SourceLocales)
	if err != nil {
		s.writeContentLocaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, content)
}

func (s *Server) adminSaveTaxonomyTranslation(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                       `json:"expected_revision"`
		Translation      catalog.TaxonomyTranslation `json:"translation"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	request.Translation.Locale = r.PathValue("locale")
	content, err := s.store.SaveTaxonomyTranslation(r.Context(), current.UserID, r.PathValue("type"), r.PathValue("id"), request.ExpectedRevision, request.Translation)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
			return
		}
		s.writeContentLocaleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, content)
}
