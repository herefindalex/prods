package brandcapture

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCaptureExtractsBoundedSemanticCandidateWithoutSourceIdentity(t *testing.T) {
	markup := `<!doctype html><html><head>
		<title>Acme Components | Electronics</title>
		<meta property="og:site_name" content="Acme Components">
		<meta name="theme-color" content="#1A2B3C">
		<link rel="canonical" href="https://other.example/canonical">
		<link rel="stylesheet" href="/brand.css">
		<script src="https://tracker.example/analytics.js"></script>
		<style>body{font-family:"Inter", sans-serif;color:#abc}</style>
	</head><body>
		<header><nav><a href="/products">Products</a><a href="/support">Support</a><a href="http://127.0.0.1/private">Private</a></nav></header>
		<img class="company-logo" src="/logo.svg"><footer>Copyright Acme</footer>
	</body></html>`
	candidate, err := Capture("https://www.acme.example/about", strings.NewReader(markup))
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Organization != "Acme Components" || candidate.Title != "Acme Components | Electronics" {
		t.Fatalf("unexpected identity candidate: %+v", candidate)
	}
	if len(candidate.Navigation) != 2 || candidate.Navigation[0].URL != "https://www.acme.example/products" {
		t.Fatalf("unexpected navigation: %+v", candidate.Navigation)
	}
	if len(candidate.StylesheetURLs) != 1 || candidate.StylesheetURLs[0] != "https://www.acme.example/brand.css" {
		t.Fatalf("unexpected stylesheets: %+v", candidate.StylesheetURLs)
	}
	if len(candidate.LogoURLs) != 1 || candidate.LogoURLs[0] != "https://www.acme.example/logo.svg" {
		t.Fatalf("unexpected logo: %+v", candidate.LogoURLs)
	}
	if candidate.Colors[0] != "#1a2b3c" || candidate.Colors[1] != "#aabbcc" || candidate.Fonts[0] != `"Inter", sans-serif` {
		t.Fatalf("unexpected theme: colors=%v fonts=%v", candidate.Colors, candidate.Fonts)
	}
	if !candidate.NeedsRightsConfirmation || !candidate.WorkingCopyOnly {
		t.Fatalf("candidate safety flags missing: %+v", candidate)
	}
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"canonical", "analytics.js", "<script", "raw_html", "raw_css", "noindex", "hreflang"} {
		if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
			t.Fatalf("candidate leaked forbidden source data %q: %s", forbidden, encoded)
		}
	}
}

func TestCaptureMalformedAndJSShellRemainManualCandidates(t *testing.T) {
	malformed, err := Capture("https://example.com", strings.NewReader(`<html><head><title>例子企業 - 產品</title><style>:root{--x:#fff}</style><body><footer>聯絡我們`))
	if err != nil {
		t.Fatal(err)
	}
	if malformed.Organization != "例子企業" || malformed.FooterText != "聯絡我們" {
		t.Fatalf("malformed semantic extraction failed: %+v", malformed)
	}

	shell, err := Capture("https://example.com", strings.NewReader(`<html><head><title>App</title><script src="/app.js"></script></head><body><div id="root"></div></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if !shell.RequiresBrowser || len(shell.UnavailableDynamicParts) != 1 || shell.UnavailableDynamicParts[0] != "navigation" {
		t.Fatalf("JS-only shell was not identified: %+v", shell)
	}
	if len(shell.ManualCorrections) == 0 {
		t.Fatal("JS-only shell must expose manual correction work")
	}
}

func TestCaptureRejectsOversizedHTML(t *testing.T) {
	_, err := Capture("https://example.com", strings.NewReader(strings.Repeat("x", int(MaxHTMLBytes)+1)))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized HTML error=%v", err)
	}
}

func TestCaptureDoesNotLeakScriptTextFromSemanticRegions(t *testing.T) {
	candidate, err := Capture("https://example.com", strings.NewReader(`<html><head><title>`+strings.Repeat("界", 100)+`</title></head><body><footer>Visible<script>secretToken()</script><style>.private{}</style> End</footer></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(candidate.FooterText, "secretToken") || strings.Contains(candidate.FooterText, ".private") || candidate.FooterText != "Visible End" {
		t.Fatalf("footer leaked executable source text: %q", candidate.FooterText)
	}
	if !utf8.ValidString(candidate.Title) || len(candidate.Title) > 200 {
		t.Fatalf("bounded title is not valid UTF-8: %q", candidate.Title)
	}
}
