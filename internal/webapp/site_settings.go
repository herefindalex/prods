package webapp

import (
	"errors"
	"net/http"

	"prods/internal/identity"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func (s *Server) adminSiteSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	settings, err := s.store.SiteSettings(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) adminUpdateSiteSettings(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		TimeZone         string `json:"time_zone"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	settings, err := s.store.UpdateSiteTimeZone(r.Context(), current.UserID, request.ExpectedRevision, request.TimeZone)
	if err != nil {
		switch {
		case errors.Is(err, site.ErrInvalidSettings):
			s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		case errors.Is(err, site.ErrSettingsConflict):
			s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
		case errors.Is(err, sqlite.ErrPermissionDenied):
			s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
		default:
			s.internalAPIError(w, r, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
