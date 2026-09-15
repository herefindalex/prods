package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/maildelivery"
	"prods/internal/storage/sqlite"
)

func (s *Server) adminSendSetPasswordInvitation(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, true)
	if !ok {
		return
	}
	if s.mailSender == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "SMTP is not configured"})
		return
	}

	var request struct {
		SetPasswordURL string `json:"set_password_url"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	rawToken, ok := s.invitationToken(request.SetPasswordURL)
	if !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid set-password URL"})
		return
	}

	attempt, err := s.store.CreateUserInvitationMailAttempt(r.Context(), current.UserID, r.PathValue("id"), rawToken)
	if err != nil {
		if errors.Is(err, sqlite.ErrInvitationMailInFlight) || errors.Is(err, sqlite.ErrInvitationMailUnknown) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.writeUserError(w, err)
		return
	}
	setPasswordURL := strings.TrimRight(s.config.BaseURL, "/") + "/set-password?token=" + url.QueryEscape(rawToken)
	deliveryContext := context.WithoutCancel(r.Context())
	result := s.mailSender.Send(deliveryContext, maildelivery.Message{
		To:      attempt.RecipientEmail,
		Subject: "Set your Prods password",
		Body: "An administrator created or renewed your Prods account access.\n\n" +
			"Use this one-time link to set your password:\n" + setPasswordURL + "\n\n" +
			"If you were not expecting this message, contact your administrator.",
	})
	status := identity.InvitationMailFailed
	switch result.Status {
	case inquiries.DeliveryAccepted:
		status = identity.InvitationMailAccepted
	case inquiries.DeliveryUnknown:
		status = identity.InvitationMailUnknown
	}
	attempt, err = s.store.CompleteUserInvitationMailAttempt(
		deliveryContext, attempt.ID, status, result.ErrorClass, result.ErrorMessage,
	)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, attempt)
}

func (s *Server) adminUserInvitationMailAttempts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityUsersManage, false); !ok {
		return
	}
	attempts, err := s.store.ListUserInvitationMailAttempts(r.Context(), r.PathValue("id"), 20)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if attempts == nil {
		attempts = []identity.InvitationMailAttempt{}
	}
	writeJSON(w, http.StatusOK, attempts)
}

func (s *Server) invitationToken(rawURL string) (string, bool) {
	provided, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || provided == nil || !provided.IsAbs() || provided.User != nil || provided.Fragment != "" {
		return "", false
	}
	base, err := url.Parse(s.config.BaseURL)
	if err != nil || base == nil {
		return "", false
	}
	if !strings.EqualFold(provided.Scheme, base.Scheme) ||
		!strings.EqualFold(provided.Hostname(), base.Hostname()) || provided.Port() != base.Port() ||
		provided.Path != strings.TrimRight(base.Path, "/")+"/set-password" {
		return "", false
	}
	query := provided.Query()
	values, exists := query["token"]
	if !exists || len(values) != 1 || strings.TrimSpace(values[0]) == "" || len(query) != 1 {
		return "", false
	}
	return values[0], true
}
