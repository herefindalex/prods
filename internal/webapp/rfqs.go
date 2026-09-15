package webapp

import (
	"errors"
	"net/http"

	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/storage/sqlite"
)

func (s *Server) adminRFQRecipientUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, false); !ok {
		return
	}
	users, err := s.store.ListRFQRecipientUsers(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) adminRFQRecipientSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, false); !ok {
		return
	}
	settings, err := s.store.RFQRecipientSettings(r.Context())
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) adminUpdateRFQRecipientSettings(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                 `json:"expected_revision"`
		Recipients       []inquiries.Recipient `json:"recipients"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	settings, err := s.store.UpdateRFQRecipientSettings(r.Context(), current.UserID, request.ExpectedRevision, request.Recipients)
	if err != nil {
		s.writeRFQManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) adminUpdateRFQStatus(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64            `json:"expected_revision"`
		Status           inquiries.Status `json:"status"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	rfq, err := s.store.UpdateRFQStatus(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Status)
	if err != nil {
		s.writeRFQManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rfq)
}

func (s *Server) adminReplaceRFQRecipients(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64                 `json:"expected_revision"`
		Recipients       []inquiries.Recipient `json:"recipients"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	rfq, err := s.store.ReplaceRFQRecipients(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Recipients)
	if err != nil {
		s.writeRFQManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rfq)
}

func (s *Server) adminAnonymizeRFQ(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	rfq, err := s.store.AnonymizeRFQ(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision)
	if err != nil {
		s.writeRFQManagementError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rfq)
}

func (s *Server) adminRFQDeliveries(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityRFQView, false); !ok {
		return
	}
	attempts, err := s.store.ListSMTPDeliveryAttempts(r.Context(), r.PathValue("id"))
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, attempts)
}

func (s *Server) adminCreateRFQDelivery(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityRFQManage, true)
	if !ok {
		return
	}
	if s.mailManager == nil {
		s.writeRFQManagementError(w, r, inquiries.ErrSMTPNotConfigured)
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		DeliveryKey      string `json:"delivery_key"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	attempt, err := s.store.CreateSMTPDeliveryAttempt(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.DeliveryKey)
	if err != nil {
		s.writeRFQManagementError(w, r, err)
		return
	}
	s.mailManager.Wake()
	status := http.StatusAccepted
	if attempt.Replay {
		status = http.StatusOK
	}
	writeJSON(w, status, attempt)
}

func (s *Server) writeRFQManagementError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, inquiries.ErrRFQNotFound):
		s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
	case errors.Is(err, inquiries.ErrRFQRevisionConflict):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
	case errors.Is(err, inquiries.ErrDeliveryConflict):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeConflict)
	case errors.Is(err, inquiries.ErrRFQAnonymized),
		errors.Is(err, inquiries.ErrRFQDeliveryInFlight):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeConflict)
	case errors.Is(err, inquiries.ErrSMTPNotConfigured):
		s.writeAPIError(w, r, http.StatusPreconditionFailed, apiCodeSMTPNotConfigured)
	case errors.Is(err, inquiries.ErrInvalidRFQStatus),
		errors.Is(err, inquiries.ErrInvalidTransition),
		errors.Is(err, inquiries.ErrInvalidRecipient),
		errors.Is(err, inquiries.ErrInvalidDelivery),
		errors.Is(err, inquiries.ErrNoDeliveryRecipient):
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
	case errors.Is(err, sqlite.ErrPermissionDenied):
		s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
	default:
		s.internalAPIError(w, r, err)
	}
}
