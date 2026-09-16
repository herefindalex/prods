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
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	if s.brandCapturer == nil {
		s.writeAPIError(w, r, http.StatusServiceUnavailable, apiCodeServiceUnavailable)
		return
	}
	state, err := s.store.WebsiteState(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	candidate, err := s.brandCapturer.Capture(r.Context(), request.SourceURL)
	if err != nil {
		status := http.StatusBadGateway
		code := apiCodeServiceUnavailable
		if errors.Is(err, brandcapture.ErrInvalidURL) || errors.Is(err, brandcapture.ErrNonPublicDestination) {
			status = http.StatusUnprocessableEntity
			code = apiCodeValidationFailed
		} else if errors.Is(err, brandcapture.ErrSourceAccessDenied) {
			code = apiCodeBrandCaptureDenied
		}
		s.writeAPIError(w, r, status, code)
		return
	}
	proposed, diff, err := brandcapture.BuildProposal(state.Working, candidate)
	if err != nil {
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
		return
	}
	writeJSON(w, http.StatusOK, brandCaptureResponse{
		WorkingRevision: state.WorkingRevision,
		Candidate:       candidate,
		Proposed:        proposed,
		Diff:            diff,
	})
}
