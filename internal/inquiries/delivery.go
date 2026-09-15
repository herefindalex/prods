package inquiries

import (
	"errors"
	"fmt"
	"strings"
)

const DeliveryContentVersion = "rfq-mail-v1"

var (
	ErrSMTPNotConfigured   = errors.New("SMTP is not configured")
	ErrDeliveryConflict    = errors.New("delivery key already used for another RFQ revision")
	ErrInvalidDelivery     = errors.New("invalid RFQ delivery request")
	ErrNoDeliveryRecipient = errors.New("RFQ has no recipients")
)

type DeliveryStatus string

const (
	DeliveryPending  DeliveryStatus = "pending"
	DeliverySending  DeliveryStatus = "sending"
	DeliveryAccepted DeliveryStatus = "accepted"
	DeliveryPartial  DeliveryStatus = "partial"
	DeliveryFailed   DeliveryStatus = "failed"
	DeliveryUnknown  DeliveryStatus = "unknown"
)

type DeliveryRecipient struct {
	Index        int            `json:"index"`
	Kind         RecipientKind  `json:"kind"`
	SourceUserID string         `json:"source_user_id,omitempty"`
	Email        string         `json:"email"`
	Status       DeliveryStatus `json:"status"`
	ErrorClass   string         `json:"error_class,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	StartedAt    string         `json:"started_at,omitempty"`
	CompletedAt  string         `json:"completed_at,omitempty"`
}

type DeliveryAttempt struct {
	ID             string              `json:"id"`
	RFQID          string              `json:"rfq_id"`
	RFQRevision    int64               `json:"rfq_revision"`
	ContentVersion string              `json:"content_version"`
	ContentHash    string              `json:"content_hash"`
	Status         DeliveryStatus      `json:"status"`
	CreatedBy      string              `json:"created_by"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
	Recipients     []DeliveryRecipient `json:"recipients"`
	Replay         bool                `json:"replay,omitempty"`
}

type DeliveryItem struct {
	Kind               string `json:"kind"`
	ProductID          string `json:"product_id,omitempty"`
	Requested          string `json:"requested,omitempty"`
	RawQuery           string `json:"raw_query,omitempty"`
	Quantity           string `json:"quantity,omitempty"`
	Notes              string `json:"notes,omitempty"`
	PublicSnapshotJSON string `json:"public_snapshot_json,omitempty"`
}

type DeliveryContent struct {
	Version        string         `json:"version"`
	RFQID          string         `json:"rfq_id"`
	Name           string         `json:"name"`
	Email          string         `json:"email"`
	Company        string         `json:"company,omitempty"`
	Phone          string         `json:"phone,omitempty"`
	Country        string         `json:"country,omitempty"`
	GeneralMessage string         `json:"general_message,omitempty"`
	Items          []DeliveryItem `json:"items"`
}

type DeliveryWork struct {
	AttemptID      string
	RecipientIndex int
	To             string
	Subject        string
	Body           string
}

func RenderDelivery(content DeliveryContent) (subject, body string) {
	subject = "Prods RFQ " + content.RFQID
	var text strings.Builder
	fmt.Fprintf(&text, "RFQ: %s\nName: %s\nEmail: %s\n", content.RFQID, content.Name, content.Email)
	if content.Company != "" {
		fmt.Fprintf(&text, "Company: %s\n", content.Company)
	}
	if content.Phone != "" {
		fmt.Fprintf(&text, "Phone: %s\n", content.Phone)
	}
	if content.Country != "" {
		fmt.Fprintf(&text, "Country/Region: %s\n", content.Country)
	}
	text.WriteString("\nItems:\n")
	for index, item := range content.Items {
		label := item.Requested
		if label == "" {
			label = item.ProductID
		}
		if label == "" {
			label = item.RawQuery
		}
		fmt.Fprintf(&text, "%d. [%s] %s", index+1, item.Kind, label)
		if item.Quantity != "" {
			fmt.Fprintf(&text, " — quantity: %s", item.Quantity)
		}
		text.WriteByte('\n')
		if item.Notes != "" {
			fmt.Fprintf(&text, "   Notes: %s\n", item.Notes)
		}
	}
	if content.GeneralMessage != "" {
		fmt.Fprintf(&text, "\nMessage:\n%s\n", content.GeneralMessage)
	}
	return subject, text.String()
}
