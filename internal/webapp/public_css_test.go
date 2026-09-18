package webapp

import (
	"regexp"
	"strings"
	"testing"
)

func TestPublicHeaderRowLinksInheritReadableRowTextColor(t *testing.T) {
	stylesheet, err := content.ReadFile("static/public/public.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stylesheet), ".site-header-row > a {\n  color: inherit;\n}") {
		t.Fatalf("public stylesheet does not keep direct header-row links on the row's readable text color")
	}
}

func TestPublicHeaderRFQUsesPrimaryContrastColor(t *testing.T) {
	stylesheet, err := content.ReadFile("static/public/public.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`(?s)\.site-header-row > a\.header-rfq \{[^}]*color: var\(--prods-primary-contrast, #fff\);[^}]*\}`)
	if !rule.Match(stylesheet) {
		t.Fatalf("public stylesheet does not override inherited header link color with the primary contrast token")
	}
}
