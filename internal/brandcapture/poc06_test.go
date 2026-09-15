//go:build poc

package brandcapture

import (
	"bytes"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"

	nethtml "golang.org/x/net/html"
)

const poc06MaxHTML = 1 << 20

type poc06Navigation struct {
	Label string
	URL   string
}

type poc06Candidate struct {
	Organization            string
	Title                   string
	LogoURLs                []string
	Colors                  []string
	Fonts                   []string
	Navigation              []poc06Navigation
	FooterText              string
	RequiresBrowser         bool
	UnavailableDynamicParts []string
	ManualCorrections       []string
	NeedsRightsConfirmation bool
	WorkingCopyOnly         bool
}

var (
	poc06HexColor = regexp.MustCompile(`(?i)#[0-9a-f]{3}(?:[0-9a-f]{3})?\b`)
	poc06Font     = regexp.MustCompile(`(?i)font-family\s*:\s*([^;}]+)`)
)

func validatePOC06FetchURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("absolute URL required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("only HTTP/HTTPS is allowed")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return errors.New("local destination is forbidden")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()) {
		return errors.New("non-public destination is forbidden")
	}
	return nil
}

func resolvePOC06URL(base *url.URL, raw string) string {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || raw == "" {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func capturePOC06(sourceURL string, reader io.Reader) (poc06Candidate, error) {
	if err := validatePOC06FetchURL(sourceURL); err != nil {
		return poc06Candidate{}, err
	}
	base, _ := url.Parse(sourceURL)
	limited := io.LimitReader(reader, poc06MaxHTML+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return poc06Candidate{}, err
	}
	if len(body) > poc06MaxHTML {
		return poc06Candidate{}, errors.New("HTML exceeds capture limit")
	}
	document, err := nethtml.Parse(bytes.NewReader(body))
	if err != nil {
		return poc06Candidate{}, err
	}
	candidate := poc06Candidate{NeedsRightsConfirmation: true, WorkingCopyOnly: true}
	var styles []string
	var scriptCount, navAnchorCount int
	var walk func(*nethtml.Node, bool, bool)
	walk = func(node *nethtml.Node, inNav, inFooter bool) {
		if node.Type == nethtml.ElementNode {
			tag := strings.ToLower(node.Data)
			attrs := attributeMap(node)
			role := strings.ToLower(attrs["role"])
			classID := strings.ToLower(attrs["class"] + " " + attrs["id"])
			if tag == "nav" || role == "navigation" || (tag == "header" && strings.Contains(classID, "nav")) {
				inNav = true
			}
			if tag == "footer" || role == "contentinfo" {
				inFooter = true
			}
			switch tag {
			case "title":
				candidate.Title = poc06FirstNonEmpty(candidate.Title, cleanText(node))
			case "meta":
				key := strings.ToLower(poc06FirstNonEmpty(attrs["property"], attrs["name"]))
				value := strings.TrimSpace(attrs["content"])
				switch key {
				case "og:site_name", "application-name":
					candidate.Organization = poc06FirstNonEmpty(candidate.Organization, value)
				case "theme-color":
					candidate.Colors = poc06AppendUnique(candidate.Colors, value)
				}
			case "style":
				styles = append(styles, cleanText(node))
			case "script":
				scriptCount++
			case "link":
				rel := strings.ToLower(attrs["rel"])
				if strings.Contains(rel, "icon") {
					candidate.LogoURLs = poc06AppendUnique(candidate.LogoURLs, resolvePOC06URL(base, attrs["href"]))
				}
			case "img":
				meaning := strings.ToLower(attrs["alt"] + " " + attrs["class"] + " " + attrs["id"])
				if strings.Contains(meaning, "logo") || strings.Contains(meaning, "brand") {
					candidate.LogoURLs = poc06AppendUnique(candidate.LogoURLs, resolvePOC06URL(base, poc06FirstNonEmpty(attrs["src"], attrs["data-src"])))
				}
			case "a":
				if inNav {
					label := cleanText(node)
					href := resolvePOC06URL(base, attrs["href"])
					if label != "" && href != "" {
						candidate.Navigation = append(candidate.Navigation, poc06Navigation{Label: label, URL: href})
						navAnchorCount++
					}
				}
			}
			if inline := attrs["style"]; inline != "" {
				styles = append(styles, inline)
			}
		}
		if inFooter && node.Type == nethtml.TextNode {
			candidate.FooterText += " " + strings.TrimSpace(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inNav, inFooter)
		}
	}
	walk(document, false, false)
	candidate.FooterText = strings.Join(strings.Fields(candidate.FooterText), " ")
	if candidate.Organization == "" {
		candidate.Organization = poc06OrganizationFromTitle(candidate.Title)
	}
	for _, style := range styles {
		for _, color := range poc06HexColor.FindAllString(style, -1) {
			candidate.Colors = poc06AppendUnique(candidate.Colors, strings.ToLower(color))
		}
		for _, match := range poc06Font.FindAllStringSubmatch(style, -1) {
			candidate.Fonts = poc06AppendUnique(candidate.Fonts, strings.Trim(strings.TrimSpace(match[1]), `"'`))
		}
	}
	if scriptCount > 0 && navAnchorCount == 0 {
		candidate.RequiresBrowser = true
		candidate.UnavailableDynamicParts = append(candidate.UnavailableDynamicParts, "navigation")
	}
	if candidate.Organization == "" {
		candidate.ManualCorrections = append(candidate.ManualCorrections, "organization")
	}
	if len(candidate.LogoURLs) == 0 {
		candidate.ManualCorrections = append(candidate.ManualCorrections, "logo")
	}
	if len(candidate.Colors) == 0 {
		candidate.ManualCorrections = append(candidate.ManualCorrections, "colors")
	}
	if len(candidate.Fonts) == 0 {
		candidate.ManualCorrections = append(candidate.ManualCorrections, "fonts")
	}
	if len(candidate.Navigation) == 0 {
		candidate.ManualCorrections = append(candidate.ManualCorrections, "navigation")
	}
	return candidate, nil
}

func attributeMap(node *nethtml.Node) map[string]string {
	result := make(map[string]string, len(node.Attr))
	for _, attr := range node.Attr {
		result[strings.ToLower(attr.Key)] = attr.Val
	}
	return result
}

func cleanText(node *nethtml.Node) string {
	var parts []string
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current.Type == nethtml.TextNode {
			parts = append(parts, current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func poc06FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func poc06AppendUnique(values []string, candidate string) []string {
	if candidate == "" {
		return values
	}
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return values
		}
	}
	return append(values, candidate)
}

func poc06OrganizationFromTitle(title string) string {
	for _, separator := range []string{" | ", " – ", " — ", " - "} {
		if before, _, ok := strings.Cut(title, separator); ok {
			return strings.TrimSpace(before)
		}
	}
	return strings.TrimSpace(title)
}

func renderPOC06Preview(candidate poc06Candidate) string {
	var navigation strings.Builder
	for _, item := range candidate.Navigation {
		fmt.Fprintf(&navigation, `<a href=%q>%s</a>`, item.URL, stdhtml.EscapeString(item.Label))
	}
	return `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + stdhtml.EscapeString(candidate.Organization) + ` product catalog</title></head><body>` +
		`<header><nav aria-label="Primary">` + navigation.String() + `</nav></header>` +
		`<main><form action="/search" method="get"><label>Search products<input name="q"></label><button>Search</button></form>` +
		`<article><h1>Example Product</h1><a href="/rfq?product_id=example">Request for quotation</a></article></main>` +
		`<footer>` + stdhtml.EscapeString(candidate.FooterText) + `</footer></body></html>`
}

func inspectPOC06Preview(markup string) (viewport, search, rfq, semantic bool, err error) {
	document, err := nethtml.Parse(strings.NewReader(markup))
	if err != nil {
		return false, false, false, false, err
	}
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node.Type == nethtml.ElementNode {
			attrs := attributeMap(node)
			switch strings.ToLower(node.Data) {
			case "meta":
				viewport = viewport || strings.EqualFold(attrs["name"], "viewport")
			case "form":
				search = search || attrs["action"] == "/search" && strings.EqualFold(attrs["method"], "get")
			case "a":
				rfq = rfq || strings.HasPrefix(attrs["href"], "/rfq")
			case "main", "article", "nav":
				semantic = true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return viewport, search, rfq, semantic, nil
}

func TestPOC06RepresentativeControlledFixtures(t *testing.T) {
	fixtures := []struct {
		name         string
		html         string
		wantOrg      string
		wantNav      int
		maxManual    int
		needsBrowser bool
		unavailable  string
	}{
		{
			name:    "large-manufacturer-semantic",
			html:    `<!doctype html><html><head><title>Acme Semi | Components</title><meta property="og:site_name" content="Acme Semiconductor"><meta name="theme-color" content="#0057b8"><link rel="icon" href="/favicon.svg"><style>:root{--brand:#0057b8;font-family:Inter, sans-serif}</style></head><body><header><img class="brand-logo" src="/logo.svg"><nav><a href="/products">Products</a><a href="/support">Support</a></nav></header><footer>© Acme Semiconductor</footer></body></html>`,
			wantOrg: "Acme Semiconductor", wantNav: 2, maxManual: 0,
		},
		{
			name:    "distributor-role-navigation",
			html:    `<html><head><title>Beta Distribution – Electronic Parts</title><style>body{color:#223344;font-family:'Noto Sans TC'}</style></head><body><div id="masthead"><img alt="Beta logo" data-src="assets/beta.png"><div role="navigation"><a href="catalog.html">產品</a><a href="contact.html">聯絡我們</a></div></div><div role="contentinfo">台北 · Singapore</div></body></html>`,
			wantOrg: "Beta Distribution", wantNav: 2, maxManual: 0,
		},
		{
			name:    "malformed-multilingual-html",
			html:    `<html lang=ja><head><title>Gamma Devices - ガンマ</title><meta name=theme-color content=#b30></head><body><header class=nav><img id=logo src=/g.svg><a href=/ja/products>製品<a href=/en/products>Products</header><footer>Gamma Devices`,
			wantOrg: "Gamma Devices", wantNav: 2, maxManual: 1,
		},
		{
			name:    "complex-mega-menu",
			html:    `<html><head><meta name="application-name" content="Delta Controls"><style>.x{background:#102030;font-family:Roboto}</style></head><body><header><img class="logo" src="/delta.svg"><nav aria-label="Mega"><section><h2>Products</h2><a href="/power">Power</a><a href="/sensors">Sensors</a></section><section><h2>Company</h2><a href="/about">About</a></section></nav></header><footer>Delta Controls</footer></body></html>`,
			wantOrg: "Delta Controls", wantNav: 3, maxManual: 0,
		},
		{
			name:    "dynamic-shell-no-browser-runtime",
			html:    `<html><head><title>Epsilon Systems</title><meta name="theme-color" content="#456789"></head><body><div id="app"></div><script src="/runtime.js"></script></body></html>`,
			wantOrg: "Epsilon Systems", wantNav: 0, maxManual: 3, needsBrowser: true, unavailable: "navigation",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			candidate, err := capturePOC06("https://fixture.example.test/start", strings.NewReader(fixture.html))
			if err != nil {
				t.Fatal(err)
			}
			if candidate.Organization != fixture.wantOrg {
				t.Fatalf("organization = %q, want %q", candidate.Organization, fixture.wantOrg)
			}
			if len(candidate.Navigation) != fixture.wantNav {
				t.Fatalf("navigation = %d, want %d: %#v", len(candidate.Navigation), fixture.wantNav, candidate.Navigation)
			}
			if len(candidate.ManualCorrections) > fixture.maxManual {
				t.Fatalf("manual corrections = %v, max %d", candidate.ManualCorrections, fixture.maxManual)
			}
			if candidate.RequiresBrowser != fixture.needsBrowser {
				t.Fatalf("requires browser = %v", candidate.RequiresBrowser)
			}
			if fixture.unavailable != "" && !containsPOC06(candidate.UnavailableDynamicParts, fixture.unavailable) {
				t.Fatalf("unavailable parts = %v", candidate.UnavailableDynamicParts)
			}
			if !candidate.NeedsRightsConfirmation || !candidate.WorkingCopyOnly {
				t.Fatal("capture candidate bypassed rights confirmation or working-copy review")
			}
			preview := renderPOC06Preview(candidate)
			viewport, search, rfq, semantic, err := inspectPOC06Preview(preview)
			if err != nil || !viewport || !search || !rfq || !semantic {
				t.Fatalf("preview usability viewport=%v search=%v rfq=%v semantic=%v err=%v", viewport, search, rfq, semantic, err)
			}
			if strings.Contains(preview, "<script") {
				t.Fatal("captured script leaked into preview")
			}
		})
	}
}

func containsPOC06(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPOC06ManualCorrectionCountsAreExplicit(t *testing.T) {
	candidate, err := capturePOC06("https://fixture.example.test", strings.NewReader(`<html><body><p>Plain company page</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(candidate.ManualCorrections)
	want := []string{"colors", "fonts", "logo", "navigation", "organization"}
	if fmt.Sprint(candidate.ManualCorrections) != fmt.Sprint(want) {
		t.Fatalf("manual correction fields = %v, want %v", candidate.ManualCorrections, want)
	}
}

func TestPOC06FetchBoundaryRevalidatesEveryRedirect(t *testing.T) {
	for _, raw := range []string{
		"file:///etc/passwd",
		"http://127.0.0.1/admin",
		"http://10.0.0.8/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/",
		"http://localhost/",
		"http://printer.local/",
	} {
		if err := validatePOC06FetchURL(raw); err == nil {
			t.Fatalf("unsafe fetch target admitted: %s", raw)
		}
	}
	chain := []string{"https://fixture.example.test/start", "https://cdn.example.test/landing", "http://192.168.1.20/internal"}
	for index, target := range chain {
		err := validatePOC06FetchURL(target)
		if index < 2 && err != nil {
			t.Fatalf("public redirect %s: %v", target, err)
		}
		if index == 2 && err == nil {
			t.Fatal("redirect into private network was not revalidated")
		}
	}
}

func TestPOC06DownloadLimitAndNoCanonicalOrScriptCapture(t *testing.T) {
	tooLarge := strings.NewReader(strings.Repeat("x", poc06MaxHTML+1))
	if _, err := capturePOC06("https://fixture.example.test", tooLarge); err == nil {
		t.Fatal("oversized HTML admitted")
	}
	markup := `<html><head><title>Zeta Parts</title><link rel="canonical" href="https://other.invalid/stolen"><style>body{color:#123456;font-family:Arial}</style></head><body><img alt="logo" src="/zeta.svg"><nav><a href="/products">Products</a></nav><script>alert(1)</script></body></html>`
	candidate, err := capturePOC06("https://fixture.example.test", strings.NewReader(markup))
	if err != nil {
		t.Fatal(err)
	}
	encoded := fmt.Sprintf("%#v", candidate)
	if strings.Contains(encoded, "other.invalid") || strings.Contains(encoded, "alert(1)") {
		t.Fatal("source canonical/script entered Brand Profile candidate")
	}
}
