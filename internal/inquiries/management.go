package inquiries

import (
	"errors"
	"net/mail"
	"strings"
)

type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
	StatusSpam       Status = "spam"
)

var (
	ErrRFQNotFound         = errors.New("RFQ not found")
	ErrRFQRevisionConflict = errors.New("RFQ revision conflict")
	ErrInvalidRFQStatus    = errors.New("invalid RFQ status")
	ErrInvalidTransition   = errors.New("invalid RFQ status transition")
	ErrInvalidRecipient    = errors.New("invalid RFQ recipient")
	ErrRFQAnonymized       = errors.New("RFQ personal data has been anonymized")
	ErrRFQDeliveryInFlight = errors.New("RFQ delivery is currently in progress")
)

type RecipientKind string

const (
	RecipientUser  RecipientKind = "user"
	RecipientEmail RecipientKind = "email"
)

// Recipient is an assignment target, not an authorization grant and not a
// delivery record. Email and DisplayName are resolved for Admin presentation.
type Recipient struct {
	Kind        RecipientKind `json:"kind"`
	UserID      string        `json:"user_id,omitempty"`
	Email       string        `json:"email,omitempty"`
	DisplayName string        `json:"display_name,omitempty"`
}

type RecipientUserOption struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type RecipientSettings struct {
	Revision   int64       `json:"revision"`
	UpdatedBy  string      `json:"updated_by,omitempty"`
	UpdatedAt  string      `json:"updated_at"`
	Recipients []Recipient `json:"recipients"`
}

func ValidStatus(status Status) bool {
	switch status {
	case StatusNew, StatusInProgress, StatusClosed, StatusSpam:
		return true
	default:
		return false
	}
}

// CanTransition deliberately encodes only the PRD transitions. In particular,
// Closed means processing ended, not won business, and it may be reopened.
func CanTransition(from, to Status) bool {
	switch from {
	case StatusNew:
		return to == StatusInProgress || to == StatusSpam
	case StatusInProgress:
		return to == StatusClosed || to == StatusSpam
	case StatusClosed:
		return to == StatusInProgress
	case StatusSpam:
		return to == StatusNew || to == StatusInProgress
	default:
		return false
	}
}

func NormalizeRecipients(recipients []Recipient) ([]Recipient, error) {
	normalized := make([]Recipient, 0, len(recipients))
	seenUsers := make(map[string]struct{}, len(recipients))
	seenEmails := make(map[string]struct{}, len(recipients))
	for _, recipient := range recipients {
		recipient.UserID = strings.TrimSpace(recipient.UserID)
		recipient.Email = strings.TrimSpace(recipient.Email)
		recipient.DisplayName = ""
		switch recipient.Kind {
		case RecipientUser:
			if recipient.UserID == "" || recipient.Email != "" {
				return nil, ErrInvalidRecipient
			}
			if _, exists := seenUsers[recipient.UserID]; exists {
				return nil, ErrInvalidRecipient
			}
			seenUsers[recipient.UserID] = struct{}{}
		case RecipientEmail:
			if recipient.UserID != "" || !validBareEmail(recipient.Email) {
				return nil, ErrInvalidRecipient
			}
			recipient.Email = strings.ToLower(recipient.Email)
			if _, exists := seenEmails[recipient.Email]; exists {
				return nil, ErrInvalidRecipient
			}
			seenEmails[recipient.Email] = struct{}{}
		default:
			return nil, ErrInvalidRecipient
		}
		normalized = append(normalized, recipient)
	}
	return normalized, nil
}

func validBareEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Name == "" && strings.EqualFold(parsed.Address, value)
}
