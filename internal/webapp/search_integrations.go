package webapp

import (
	"errors"
	"net/http"
	"strings"

	"prods/internal/identity"
	"prods/internal/searchnotify"
	"prods/internal/storage/sqlite"
)

type searchIntegrationsResponse struct {
	searchnotify.Settings
	IndexNowAvailable bool   `json:"indexnow_available"`
	IndexNowKeyURL    string `json:"indexnow_key_url,omitempty"`
	GoogleAvailable   bool   `json:"google_available"`
}

func (s *Server) adminSearchIntegrations(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	settings, err := s.store.SearchIntegrationSettings(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.searchIntegrationsResponse(settings))
}

func (s *Server) adminUpdateSearchIntegrations(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request searchnotify.Settings
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if request.IndexNowEnabled && (s.searchManager == nil || !s.searchManager.IndexNowAvailable()) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "IndexNow requires a public HTTPS Base URL"})
		return
	}
	if request.GoogleEnabled && (s.searchManager == nil || !s.searchManager.GoogleAvailable()) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Google Search Console requires a public HTTPS Base URL and host OAuth credentials"})
		return
	}
	updated, err := s.store.UpdateSearchIntegrationSettings(r.Context(), current.UserID, request.Revision, request)
	if err != nil {
		switch {
		case errors.Is(err, searchnotify.ErrSettingsConflict):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, searchnotify.ErrInvalidSettings):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		case errors.Is(err, sqlite.ErrPermissionDenied):
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		default:
			s.internalError(w, err)
		}
		return
	}
	if s.searchManager != nil {
		s.searchManager.Wake()
	}
	writeJSON(w, http.StatusOK, s.searchIntegrationsResponse(updated))
}

func (s *Server) searchIntegrationsResponse(settings searchnotify.Settings) searchIntegrationsResponse {
	response := searchIntegrationsResponse{Settings: settings}
	if s.searchManager != nil {
		response.IndexNowAvailable = s.searchManager.IndexNowAvailable()
		response.GoogleAvailable = s.searchManager.GoogleAvailable()
	}
	if settings.IndexNowKey != "" {
		response.IndexNowKeyURL = strings.TrimRight(s.config.BaseURL, "/") + "/" + settings.IndexNowKey + ".txt"
	}
	return response
}

func (s *Server) serveIndexNowKey(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	file := strings.TrimPrefix(r.URL.Path, "/")
	if strings.Contains(file, "/") || !strings.HasSuffix(file, ".txt") {
		return false
	}
	key := strings.TrimSuffix(file, ".txt")
	settings, err := s.store.SearchIntegrationSettings(r.Context())
	if err != nil || !settings.IndexNowEnabled || key == "" || key != settings.IndexNowKey {
		return false
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(key))
	}
	return true
}

func (s *Server) adminRetrySearchSubmission(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	job, err := s.store.RetrySearchSubmission(r.Context(), current.UserID, r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, searchnotify.ErrJobNotRetryable):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, sqlite.ErrPermissionDenied):
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		default:
			s.internalError(w, err)
		}
		return
	}
	if s.searchManager != nil {
		s.searchManager.Wake()
	}
	writeJSON(w, http.StatusAccepted, job)
}
