package site

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	DefaultMaintenanceMessage  = "This catalog is temporarily unavailable for maintenance. Please try again later."
	MaxMaintenanceMessageRunes = 280
)

var (
	ErrInvalidMaintenance  = errors.New("invalid site maintenance settings")
	ErrMaintenanceConflict = errors.New("site maintenance revision conflict")
)

type Maintenance struct {
	Active    bool   `json:"active"`
	Message   string `json:"message"`
	Revision  int64  `json:"revision"`
	UpdatedBy string `json:"updated_by,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

func (maintenance *Maintenance) Prepare() error {
	maintenance.Message = strings.TrimSpace(maintenance.Message)
	if maintenance.Active && maintenance.Message == "" {
		maintenance.Message = DefaultMaintenanceMessage
	}
	if maintenance.Revision < 1 || utf8.RuneCountInString(maintenance.Message) > MaxMaintenanceMessageRunes {
		return ErrInvalidMaintenance
	}
	return nil
}
