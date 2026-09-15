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
		s.internalError(w, err)
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
		s.internalError(w, err)
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
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	settings, err := s.store.UpdateRFQRecipientSettings(r.Context(), current.UserID, request.ExpectedRevision, request.Recipients)
	if err != nil {
		s.writeRFQManagementError(w, err)
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
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rfq, err := s.store.UpdateRFQStatus(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Status)
	if err != nil {
		s.writeRFQManagementError(w, err)
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
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rfq, err := s.store.ReplaceRFQRecipients(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Recipients)
	if err != nil {
		s.writeRFQManagementError(w, err)
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
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rfq, err := s.store.AnonymizeRFQ(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision)
	if err != nil {
		s.writeRFQManagementError(w, err)
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
		s.internalError(w, err)
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
		s.writeRFQManagementError(w, inquiries.ErrSMTPNotConfigured)
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		DeliveryKey      string `json:"delivery_key"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	attempt, err := s.store.CreateSMTPDeliveryAttempt(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.DeliveryKey)
	if err != nil {
		s.writeRFQManagementError(w, err)
		return
	}
	s.mailManager.Wake()
	status := http.StatusAccepted
	if attempt.Replay {
		status = http.StatusOK
	}
	writeJSON(w, status, attempt)
}

func (s *Server) writeRFQManagementError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inquiries.ErrRFQNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, inquiries.ErrRFQRevisionConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, inquiries.ErrDeliveryConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, inquiries.ErrRFQAnonymized),
		errors.Is(err, inquiries.ErrRFQDeliveryInFlight):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, inquiries.ErrSMTPNotConfigured):
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": err.Error()})
	case errors.Is(err, inquiries.ErrInvalidRFQStatus),
		errors.Is(err, inquiries.ErrInvalidTransition),
		errors.Is(err, inquiries.ErrInvalidRecipient),
		errors.Is(err, inquiries.ErrInvalidDelivery),
		errors.Is(err, inquiries.ErrNoDeliveryRecipient):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, sqlite.ErrPermissionDenied):
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		s.internalError(w, err)
	}
}
