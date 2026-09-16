package webapp

import (
	"net/http"

	"prods/internal/identity"
)

// Register only known UI paths. In particular API and private Preview errors
// must never be converted into a successful SPA document.
func (s *Server) registerAdminConsole() {
	for _, page := range []string{"catalog", "taxonomy", "imports", "product-bulk", "jobs", "website", "listing-profiles", "public-copy", "access", "activity", "settings", "health", "backups", "traffic"} {
		s.mux.HandleFunc("GET /admin/"+page, s.admin)
	}
	s.mux.HandleFunc("GET /admin/api/session", s.adminSession)
}

func (s *Server) adminSession(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityAdminAccess, false)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	id := current.UserID
	if s.config.EnablePOCAdmin && id == "" {
		id = "poc-admin"
	}
	writeJSON(w, http.StatusOK, struct {
		ID           string                       `json:"id"`
		CacheScope   string                       `json:"cache_scope"`
		Capabilities map[identity.Capability]bool `json:"capabilities"`
	}{id, identity.TokenDigest(current.CSRF), current.Capabilities})
}

// The scope is not an authentication credential. It prevents an already-open
// client from mixing cached data with a different cookie session in another tab.
func (s *Server) matchesAdminScope(r *http.Request, current session) bool {
	scope := r.Header.Get("X-Prods-Session-Scope")
	return scope == "" || s.validCSRF(scope, identity.TokenDigest(current.CSRF))
}
