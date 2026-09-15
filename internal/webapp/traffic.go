package webapp

import (
	"errors"
	"net/http"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/storage/sqlite"
)

type trafficSettingsResponse struct {
	sqlite.TrafficSettings
	TrustedProxyConfigured bool `json:"trusted_proxy_configured"`
}

func (s *Server) adminTrafficSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	settings, err := s.store.TrafficSettings(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.writeTrafficSettings(w, settings)
}

func (s *Server) adminUpdateTrafficSettings(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedVersion int64                  `json:"expected_version"`
		Settings        sqlite.TrafficSettings `json:"settings"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	settings, err := s.store.UpdateTrafficSettings(r.Context(), current.UserID, request.ExpectedVersion, request.Settings)
	if errors.Is(err, catalog.ErrRevisionConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, sqlite.ErrInvalidTrafficSettings) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.writeTrafficSettings(w, settings)
}

func (s *Server) writeTrafficSettings(w http.ResponseWriter, settings sqlite.TrafficSettings) {
	writeJSON(w, http.StatusOK, trafficSettingsResponse{
		TrafficSettings:        settings,
		TrustedProxyConfigured: s.proxyTrust.configured(),
	})
}
