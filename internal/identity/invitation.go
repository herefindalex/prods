package identity

type InvitationMailStatus string

const (
	InvitationMailPending  InvitationMailStatus = "pending"
	InvitationMailAccepted InvitationMailStatus = "accepted"
	InvitationMailFailed   InvitationMailStatus = "failed"
	InvitationMailUnknown  InvitationMailStatus = "unknown"
)

const InvitationMailContentVersion = "user-set-password-mail-v1"

type InvitationMailAttempt struct {
	ID             string               `json:"id"`
	UserID         string               `json:"user_id"`
	AuthRevision   int64                `json:"auth_revision"`
	RecipientEmail string               `json:"recipient_email"`
	ContentVersion string               `json:"content_version"`
	Status         InvitationMailStatus `json:"status"`
	ErrorClass     string               `json:"error_class,omitempty"`
	ErrorMessage   string               `json:"error_message,omitempty"`
	CreatedAt      string               `json:"created_at"`
	CompletedAt    string               `json:"completed_at,omitempty"`
}
