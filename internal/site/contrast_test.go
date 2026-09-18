package site

import (
	"strings"
	"testing"
)

func TestStylesheetDerivesReadablePrimaryContrast(t *testing.T) {
	configuration := DefaultConfiguration()
	configuration.Theme.PrimaryColor = "#222222"
	if stylesheet := configuration.Stylesheet(); !strings.Contains(stylesheet, "--prods-primary-contrast:#FFFFFF") {
		t.Fatalf("dark primary contrast missing from %q", stylesheet)
	}

	configuration.Theme.PrimaryColor = "#F4F4F4"
	if stylesheet := configuration.Stylesheet(); !strings.Contains(stylesheet, "--prods-primary-contrast:#161616") {
		t.Fatalf("light primary contrast missing from %q", stylesheet)
	}
}

func TestStylesheetDerivesReadableAccentContrast(t *testing.T) {
	configuration := DefaultConfiguration()
	configuration.Theme.AccentColor = "#C8102E"
	if stylesheet := configuration.Stylesheet(); !strings.Contains(stylesheet, "--prods-accent-contrast:#FFFFFF") {
		t.Fatalf("dark accent did not receive light contrast: %s", stylesheet)
	}

	configuration.Theme.AccentColor = "#F5E8C8"
	if stylesheet := configuration.Stylesheet(); !strings.Contains(stylesheet, "--prods-accent-contrast:#161616") {
		t.Fatalf("light accent did not receive dark contrast: %s", stylesheet)
	}
}
