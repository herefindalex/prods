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

func TestPublicHeaderRendersCatalogSearchInUtilityRow(t *testing.T) {
	configuration := site.DefaultConfiguration()
	configuration.Header.Rows = []site.HeaderRow{{
		ID:        "utility",
		Type:      "utility",
		SortOrder: 10,
		Items: []site.HeaderItem{{
			ID:           "search",
			Kind:         "system_action",
			SystemAction: "catalog_search",
			SortOrder:    10,
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
	rendered := body.String()
	if !strings.Contains(rendered, `<div class="site-header-row utility-row"><a href="/search">Search</a></div>`) {
		t.Fatalf("utility-row catalog search action was not rendered: %s", rendered)
	}
}
