package webapp

import (
	"errors"
	"net/http"

	"prods/internal/brandcapture"
	"prods/internal/identity"
	"prods/internal/site"
)

type brandCaptureResponse struct {
	WorkingRevision int64                      `json:"working_revision"`
	Candidate       brandcapture.Candidate     `json:"candidate"`
	Proposed        site.Configuration         `json:"proposed_configuration"`
	Diff            []brandcapture.FieldChange `json:"diff"`
}

func (s *Server) adminCaptureWebsiteBrand(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, true); !ok {
		return
	}
	var request struct {
		SourceURL string `json:"source_url"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if s.brandCapturer == nil {
		http.Error(w, "brand capture unavailable", http.StatusServiceUnavailable)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	candidate, err := s.brandCapturer.Capture(r.Context(), request.SourceURL)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, brandcapture.ErrInvalidURL) || errors.Is(err, brandcapture.ErrNonPublicDestination) {
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	proposed, diff, err := brandcapture.BuildProposal(state.Working, candidate)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, brandCaptureResponse{
		WorkingRevision: state.WorkingRevision,
		Candidate:       candidate,
		Proposed:        proposed,
		Diff:            diff,
	})
}
