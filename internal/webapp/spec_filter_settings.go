package webapp

import (
	"net/http"
	"strconv"

	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/identity"
	"prods/internal/specnumeric"
)

func (s *Server) adminSpecSearchCapabilities(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": 1, "text_rule": catalogfilter.TextRule, "kinds": []string{"text", "choice", "multi_choice", "numeric"}, "numeric_profiles": specnumeric.SupportedProfiles()})
}
func (s *Server) adminSpecSearchProfile(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	result, err := s.store.SpecSearchProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) adminConfigureSpecSearchProfile(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                 `json:"expected_revision"`
		Filterable       bool                  `json:"filterable"`
		Profile          catalogfilter.Profile `json:"profile"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || request.ExpectedRevision < 1 {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	result, err := s.store.ConfigureSpecSearchProfile(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, catalog.SpecSearchProfile{Filterable: request.Filterable, Profile: request.Profile})
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	if s.publisher != nil {
		s.publisher.Wake()
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) adminStartSpecNormalizationWork(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil || request.ExpectedRevision < 1 {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	result, err := s.store.StartSpecNormalizationWork(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	if s.publisher != nil {
		s.publisher.Wake()
	}
	writeJSON(w, http.StatusAccepted, result)
}
func (s *Server) adminSpecNormalizationWork(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	result, err := s.store.SpecNormalizationWork(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (s *Server) adminSpecNormalizationWorkItems(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	page, size := 1, 50
	for _, entry := range []struct {
		key    string
		target *int
	}{{"page", &page}, {"page_size", &size}} {
		if raw := r.URL.Query().Get(entry.key); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 || entry.key == "page_size" && v > 100 {
				s.writeAPIError(w, r, http.StatusBadRequest, apiCodeValidationFailed)
				return
			}
			*entry.target = v
		}
	}
	if _, err := s.store.SpecNormalizationWork(r.Context(), r.PathValue("id")); err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	items, total, err := s.store.SpecNormalizationWorkItems(r.Context(), r.PathValue("id"), page, size)
	if err != nil {
		s.writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total, "page": page, "page_size": size})
}
