package site

import (
	"errors"
	"testing"
)

func TestPrepareTimeZone(t *testing.T) {
	for _, value := range []string{"UTC", "America/New_York", "Asia/Taipei", "Local"} {
		if prepared, err := PrepareTimeZone("  " + value + "  "); err != nil || prepared != value {
			t.Fatalf("PrepareTimeZone(%q) = %q, %v", value, prepared, err)
		}
	}
	for _, value := range []string{"", "Not/A_Time_Zone", "UTC\nInjected"} {
		if _, err := PrepareTimeZone(value); !errors.Is(err, ErrInvalidSettings) {
			t.Fatalf("PrepareTimeZone(%q) error = %v", value, err)
		}
	}
}
