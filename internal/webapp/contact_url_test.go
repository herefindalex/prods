package webapp

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"
	"testing"

	"prods/internal/localization"
	"prods/internal/site"
)

func TestSafeContactURLForPublicTemplates(t *testing.T) {
	for _, value := range []string{"https://example.com/contact", "mailto:sales@example.com", "tel:+12125550100"} {
		if got := string(safeContactURL(value)); got != value {
			t.Fatalf("safeContactURL(%q) = %q", value, got)
		}
	}
	for _, value := range []string{"javascript:alert(1)", "mailto:sales@example.com?subject=Hello", "tel:123;ext=456"} {
		if got := string(safeContactURL(value)); got != "" {
			t.Fatalf("safeContactURL(%q) accepted unsafe value %q", value, got)
		}
	}
}

func TestPublicHeaderRendersSafeContactLink(t *testing.T) {
	configuration := site.DefaultConfiguration()
	configuration.Header.Rows = []site.HeaderRow{{
		ID:        "utility",
		Type:      "utility",
		SortOrder: 10,
		Items: []site.HeaderItem{{
			ID:        "phone",
			Kind:      "link",
			Label:     "+1 212 555 0100",
			URL:       "tel:+12125550100",
			SortOrder: 10,
		}},
	}}
	if err := configuration.Prepare(); err != nil {
		t.Fatal(err)
	}

	tmpl, err := template.New("pages").Funcs(template.FuncMap{
		"urlquery":   url.QueryEscape,
		"contactURL": safeContactURL,
		"headerURL":  safeHeaderURL,
	}).ParseFS(content, "templates/*.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	data := catalogPage{
		publicPage: publicPage{
			Site:       configuration,
			Language:   "en-US",
			Text:       localization.For("en-US"),
			CatalogURL: "/catalog",
			SearchURL:  "/search",
			RFQURL:     "/rfq",
		},
		Title:     "Catalog",
		NoResults: true,
	}
	var body bytes.Buffer
	if err := tmpl.ExecuteTemplate(&body, "catalog-listing", data); err != nil {
		t.Fatal(err)
	}
	if rendered := body.String(); !strings.Contains(rendered, `href="tel:&#43;12125550100"`) || strings.Contains(rendered, "ZgotmplZ") {
		t.Fatalf("public Header contact link was not rendered safely: %s", rendered)
	}
}
