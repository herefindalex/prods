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
		s.internalError(w, err)
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
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	settings, err := s.store.UpdateSiteTimeZone(r.Context(), current.UserID, request.ExpectedRevision, request.TimeZone)
	if err != nil {
		switch {
		case errors.Is(err, site.ErrInvalidSettings):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		case errors.Is(err, site.ErrSettingsConflict):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, sqlite.ErrPermissionDenied):
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			s.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, settings)
}
