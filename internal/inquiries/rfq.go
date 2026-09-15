package inquiries

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrInvalidRFQ          = errors.New("invalid RFQ submission")
	ErrIdempotencyConflict = errors.New("idempotency key was already used with a different payload")
)

const CanonicalVersion = "rfq-submission-v1"

type Item struct {
	Kind      string `json:"kind"`
	ProductID string `json:"product_id,omitempty"`
	Requested string `json:"requested,omitempty"`
	RawQuery  string `json:"raw_query,omitempty"`
	Quantity  string `json:"quantity,omitempty"`
	Notes     string `json:"notes,omitempty"`
}

type Submission struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Company        string `json:"company,omitempty"`
	Phone          string `json:"phone,omitempty"`
	Country        string `json:"country,omitempty"`
	GeneralMessage string `json:"general_message,omitempty"`
	Items          []Item `json:"items"`
}

type Receipt struct {
	RFQID   string `json:"rfq_id"`
	Message string `json:"message"`
	Replay  bool   `json:"replay"`
}

func (s *Submission) CanonicalHash() (string, error) {
	s.Name = strings.TrimSpace(s.Name)
	s.Email = strings.TrimSpace(s.Email)
	s.Company = strings.TrimSpace(s.Company)
	s.Phone = strings.TrimSpace(s.Phone)
	s.Country = strings.TrimSpace(s.Country)
	s.GeneralMessage = strings.TrimSpace(s.GeneralMessage)
	for i := range s.Items {
		s.Items[i].Kind = strings.TrimSpace(s.Items[i].Kind)
		s.Items[i].ProductID = strings.TrimSpace(s.Items[i].ProductID)
		s.Items[i].Requested = strings.TrimSpace(s.Items[i].Requested)
		s.Items[i].RawQuery = strings.TrimSpace(s.Items[i].RawQuery)
		s.Items[i].Quantity = strings.TrimSpace(s.Items[i].Quantity)
		s.Items[i].Notes = strings.TrimSpace(s.Items[i].Notes)
		if s.Items[i].Kind != "catalog" && s.Items[i].Kind != "requested" {
			return "", ErrInvalidRFQ
		}
	}
	if s.Name == "" || s.Email == "" || len(s.Items) == 0 {
		return "", ErrInvalidRFQ
	}
	payload := struct {
		Version    string     `json:"version"`
		Submission Submission `json:"submission"`
	}{CanonicalVersion, *s}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
