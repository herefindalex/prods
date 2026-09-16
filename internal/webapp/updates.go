package webapp

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"prods/internal/distribution"
	"prods/internal/identity"
)

func (s *Server) adminSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	if s.config.DistributionClient == nil {
		writeJSON(w, http.StatusOK, distribution.UpdateStatus{
			Status: "unavailable", CurrentVersion: s.config.ApplicationVersion,
			CheckedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.config.DistributionClient.UpdateStatus(
		ctx, s.config.ApplicationVersion, runtime.GOOS, runtime.GOARCH,
	))
}
