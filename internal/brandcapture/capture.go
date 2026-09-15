package brandcapture

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	nethtml "golang.org/x/net/html"
)

const (
	MaxHTMLBytes       int64 = 1 << 20
	MaxStylesheetBytes int64 = 256 << 10
	MaxStylesheets           = 4
	maxCandidateValues       = 20
)

var (
	hexColorPattern = regexp.MustCompile(`(?i)#[0-9a-f]{3}(?:[0-9a-f]{3})?\b`)
	fontPattern     = regexp.MustCompile(`(?i)font-family\s*:\s*([^;}]+)`)
)

type Navigation struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Candidate contains suggestions only. It intentionally has no script,
// canonical, SEO, raw HTML, or raw CSS field.
type Candidate struct {
	SourceURL               string       `json:"source_url"`
	Organization            string       `json:"organization,omitempty"`
	Title                   string       `json:"title,omitempty"`
	LogoURLs                []string     `json:"logo_urls,omitempty"`
	Colors                  []string     `json:"colors,omitempty"`
	Fonts                   []string     `json:"fonts,omitempty"`
	Navigation              []Navigation `json:"navigation,omitempty"`
	FooterText              string       `json:"footer_text,omitempty"`
	StylesheetURLs          []string     `json:"stylesheet_urls,omitempty"`
	Warnings                []string     `json:"warnings,omitempty"`
	RequiresBrowser         bool         `json:"requires_browser"`
	UnavailableDynamicParts []string     `json:"unavailable_dynamic_parts,omitempty"`
	ManualCorrections       []string     `json:"manual_corrections,omitempty"`
	NeedsRightsConfirmation bool         `json:"needs_rights_confirmation"`
	WorkingCopyOnly         bool         `json:"working_copy_only"`
}

func Capture(sourceURL string, reader io.Reader) (Candidate, error) {
	base, err := ParsePublicURL(sourceURL)
	if err != nil {
		return Candidate{}, err
	}
	body, err := io.ReadAll(io.LimitReader(reader, MaxHTMLBytes+1))
	if err != nil {
		return Candidate{}, err
	}
	if int64(len(body)) > MaxHTMLBytes {
		return Candidate{}, errors.New("HTML exceeds capture limit")
	}
	document, err := nethtml.Parse(bytes.NewReader(body))
	if err != nil {
		return Candidate{}, errors.New("HTML could not be parsed")
	}
	candidate := Candidate{
		SourceURL: sourceURL, NeedsRightsConfirmation: true, WorkingCopyOnly: true,
	}
	var styles []string
	var scriptCount, navigationAnchors int
	var walk func(*nethtml.Node, bool, bool)
	walk = func(node *nethtml.Node, inNavigation, inFooter bool) {
		if node.Type == nethtml.ElementNode {
			tag := strings.ToLower(node.Data)
			attrs := nodeAttributes(node)
			role := strings.ToLower(attrs["role"])
			classID := strings.ToLower(attrs["class"] + " " + attrs["id"])
			if tag == "nav" || role == "navigation" || (tag == "header" && strings.Contains(classID, "nav")) {
				inNavigation = true
			}
			if tag == "footer" || role == "contentinfo" {
				inFooter = true
			}
			switch tag {
			case "title":
				candidate.Title = bounded(firstNonEmpty(candidate.Title, nodeText(node)), 200)
			case "meta":
				key := strings.ToLower(firstNonEmpty(attrs["property"], attrs["name"]))
				value := bounded(strings.TrimSpace(attrs["content"]), 200)
				switch key {
				case "og:site_name", "application-name":
					candidate.Organization = firstNonEmpty(candidate.Organization, value)
				case "theme-color":
					candidate.Colors = appendColor(candidate.Colors, value)
				}
			case "style":
				styles = append(styles, rawNodeText(node))
			case "script":
				scriptCount++
			case "link":
				rel := strings.ToLower(attrs["rel"])
				if strings.Contains(rel, "stylesheet") {
					candidate.StylesheetURLs = appendUnique(candidate.StylesheetURLs, resolveURL(base, attrs["href"]), MaxStylesheets)
				} else if strings.Contains(rel, "icon") {
					candidate.LogoURLs = appendUnique(candidate.LogoURLs, resolveURL(base, attrs["href"]), maxCandidateValues)
				}
			case "img":
				meaning := strings.ToLower(attrs["alt"] + " " + attrs["class"] + " " + attrs["id"])
				if strings.Contains(meaning, "logo") || strings.Contains(meaning, "brand") {
					candidate.LogoURLs = appendUnique(candidate.LogoURLs,
						resolveURL(base, firstNonEmpty(attrs["src"], attrs["data-src"])), maxCandidateValues)
				}
			case "a":
				if inNavigation && len(candidate.Navigation) < maxCandidateValues {
					label := bounded(nodeText(node), 120)
					href := resolveURL(base, attrs["href"])
					if label != "" && href != "" && !containsNavigation(candidate.Navigation, label, href) {
						candidate.Navigation = append(candidate.Navigation, Navigation{Label: label, URL: href})
						navigationAnchors++
					}
				}
			}
			if inline := attrs["style"]; inline != "" {
				styles = append(styles, inline)
			}
			if tag == "script" || tag == "style" || tag == "template" || tag == "noscript" {
				return
			}
		}
		if inFooter && node.Type == nethtml.TextNode && len(candidate.FooterText) < 500 {
			candidate.FooterText += " " + strings.TrimSpace(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inNavigation, inFooter)
		}
	}
	walk(document, false, false)
	candidate.FooterText = bounded(strings.Join(strings.Fields(candidate.FooterText), " "), 500)
	if candidate.Organization == "" {
		candidate.Organization = bounded(organizationFromTitle(candidate.Title), 200)
	}
	for _, style := range styles {
		ApplyStyles(&candidate, style)
	}
	if scriptCount > 0 && navigationAnchors == 0 {
		candidate.RequiresBrowser = true
		candidate.UnavailableDynamicParts = appendUnique(candidate.UnavailableDynamicParts, "navigation", maxCandidateValues)
	}
	Finalize(&candidate)
	return candidate, nil
}

func ApplyStyles(candidate *Candidate, css string) {
	if candidate == nil {
		return
	}
	for _, color := range hexColorPattern.FindAllString(css, -1) {
		candidate.Colors = appendColor(candidate.Colors, color)
	}
	for _, match := range fontPattern.FindAllStringSubmatch(css, -1) {
		font := strings.TrimSpace(match[1])
		if len(font) <= 160 && validFontCandidate(font) {
			candidate.Fonts = appendUnique(candidate.Fonts, font, maxCandidateValues)
		}
	}
}

func Finalize(candidate *Candidate) {
	if candidate == nil {
		return
	}
	candidate.ManualCorrections = candidate.ManualCorrections[:0]
	for _, item := range []struct {
		missing bool
		name    string
	}{
		{candidate.Organization == "", "organization"},
		{len(candidate.LogoURLs) == 0, "logo"},
		{len(candidate.Colors) == 0, "colors"},
		{len(candidate.Fonts) == 0, "fonts"},
		{len(candidate.Navigation) == 0, "navigation"},
	} {
		if item.missing {
			candidate.ManualCorrections = append(candidate.ManualCorrections, item.name)
		}
	}
}

func nodeAttributes(node *nethtml.Node) map[string]string {
	result := make(map[string]string, len(node.Attr))
	for _, attribute := range node.Attr {
		result[strings.ToLower(attribute.Key)] = attribute.Val
	}
	return result
}

func nodeText(node *nethtml.Node) string {
	var parts []string
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current.Type == nethtml.ElementNode {
			switch strings.ToLower(current.Data) {
			case "script", "style", "template", "noscript":
				return
			}
		}
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

func rawNodeText(node *nethtml.Node) string {
	var value strings.Builder
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current.Type == nethtml.TextNode {
			value.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return value.String()
}

func resolveURL(base *url.URL, raw string) string {
	reference, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || raw == "" {
		return ""
	}
	resolved := base.ResolveReference(reference)
	validated, err := ParsePublicURL(resolved.String())
	if err != nil {
		return ""
	}
	return validated.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendUnique(values []string, candidate string, limit int) []string {
	if candidate == "" || len(values) >= limit {
		return values
	}
	for _, value := range values {
		if strings.EqualFold(value, candidate) {
			return values
		}
	}
	return append(values, candidate)
}

func appendColor(values []string, candidate string) []string {
	candidate = normalizeHexColor(candidate)
	if candidate == "" {
		return values
	}
	return appendUnique(values, candidate, maxCandidateValues)
}

func normalizeHexColor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) == 4 && value[0] == '#' {
		return "#" + strings.Repeat(value[1:2], 2) + strings.Repeat(value[2:3], 2) + strings.Repeat(value[3:4], 2)
	}
	if len(value) == 7 && hexColorPattern.MatchString(value) {
		return value
	}
	return ""
}

func validFontCandidate(value string) bool {
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && !strings.ContainsRune(" ,.'\"_-", character) {
			return false
		}
	}
	return value != ""
}

func organizationFromTitle(title string) string {
	for _, separator := range []string{" | ", " – ", " — ", " - "} {
		if before, _, ok := strings.Cut(title, separator); ok {
			return strings.TrimSpace(before)
		}
	}
	return strings.TrimSpace(title)
}

func containsNavigation(values []Navigation, label, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value.Label, label) && value.URL == target {
			return true
		}
	}
	return false
}

func bounded(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maximum {
		return value
	}
	for maximum > 0 && !isUTF8RuneStart(value[maximum]) {
		maximum--
	}
	return value[:maximum]
}

func isUTF8RuneStart(value byte) bool {
	return value&0xc0 != 0x80
}
