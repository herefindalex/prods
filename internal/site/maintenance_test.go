package site

import (
	"errors"
	"strings"
	"testing"
)

func TestMaintenancePrepareDefaultsAndBoundsPublicMessage(t *testing.T) {
	maintenance := Maintenance{Active: true, Revision: 1}
	if err := maintenance.Prepare(); err != nil || maintenance.Message != DefaultMaintenanceMessage {
		t.Fatalf("default maintenance=%+v err=%v", maintenance, err)
	}
	maintenance.Message = strings.Repeat("界", MaxMaintenanceMessageRunes+1)
	if err := maintenance.Prepare(); !errors.Is(err, ErrInvalidMaintenance) {
		t.Fatalf("oversized maintenance message=%v", err)
	}
}
