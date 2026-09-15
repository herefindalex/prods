package webapp

import (
	"errors"
	"net/http"
	"strings"

	"prods/internal/identity"
	"prods/internal/site"
)

func isMaintenanceProtectedPath(path string) bool {
	adminPath := path == "/admin" || strings.HasPrefix(path, "/admin/")
	return path != "/health/live" && path != "/set-password" && path != "/maintenance/api/state" &&
		!adminPath && !strings.HasPrefix(path, "/static/")
}

func (s *Server) maintenanceSnapshot() site.Maintenance {
	if current := s.maintenance.Load(); current != nil {
		return *current
	}
	return site.Maintenance{Message: site.DefaultMaintenanceMessage, Revision: 1}
}

func (s *Server) writeMaintenanceResponse(w http.ResponseWriter, r *http.Request, maintenance site.Maintenance) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", "300")
	if r.URL.Path == "/health/ready" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("not ready\n"))
		}
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'none'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(http.StatusServiceUnavailable)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="referrer" content="no-referrer"><title>Prods maintenance</title><link rel="stylesheet" href="/static/system/system.css"></head><body><div id="system-root" data-mode="maintenance"></div><noscript><main><h1>Temporarily unavailable</h1><p>This system page requires JavaScript. Enable JavaScript to view status and retry controls.</p></main></noscript><script type="module" src="/static/system/system.js"></script></body></html>`))
}

func (s *Server) siteMaintenanceState(w http.ResponseWriter, _ *http.Request) {
	maintenance := s.maintenanceSnapshot()
	message := strings.TrimSpace(maintenance.Message)
	if message == "" {
		message = site.DefaultMaintenanceMessage
	}
	writeJSON(w, http.StatusOK, struct {
		Active            bool   `json:"active"`
		Message           string `json:"message"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	}{Active: maintenance.Active, Message: message, RetryAfterSeconds: 300})
}

func (s *Server) adminSiteMaintenance(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.maintenanceSnapshot())
}

func (s *Server) adminUpdateSiteMaintenance(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Active           bool   `json:"active"`
		Message          string `json:"message"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// A writer lock is the manual-maintenance admission boundary. All public
	// mutations admitted before this point finish before activation commits;
	// requests arriving after it cannot enter until they observe the new state.
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	updated, err := s.store.UpdateSiteMaintenance(r.Context(), current.UserID, request.ExpectedRevision, request.Active, request.Message)
	if err != nil {
		switch {
		case errors.Is(err, site.ErrMaintenanceConflict):
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		case errors.Is(err, site.ErrInvalidMaintenance):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		default:
			s.internalError(w, err)
		}
		return
	}
	s.maintenance.Store(&updated)
	writeJSON(w, http.StatusOK, updated)
}
