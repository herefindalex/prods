package publishing

import (
	"strings"
	"testing"

	"prods/internal/site"
)

func TestHTMLRendersImportedHeaderNavigationFooterAndTheme(t *testing.T) {
	configuration := site.DefaultConfiguration()
	configuration.Organization.DisplayName = "Example Components"
	configuration.Organization.Description = "Electronic components and technical support."
	configuration.Organization.Contacts = []site.ContactMethod{{ID: "support", Type: "support", Label: "Support", Value: "+1 212 555 0100", URL: "tel:12125550100"}}
	configuration.Header.Sticky = true
	configuration.Header.Rows = []site.HeaderRow{{
		ID:        "utility",
		Type:      "utility",
		SortOrder: 10,
		Items: []site.HeaderItem{{
			ID:        "phone",
			Kind:      "link",
			Label:     "+1 212 555 0100",
			URL:       "tel:12125550100",
			SortOrder: 10,
		}},
	}}
	configuration.Navigation = []site.NavigationItem{
		{ID: "products", Label: "Products", TargetType: "group", Presentation: "dropdown", SortOrder: 10, Visible: true},
		{ID: "connectors", ParentID: "products", Label: "Connectors", URL: "https://example.com/connectors", TargetType: "link", Presentation: "dropdown", SortOrder: 10, Visible: true},
		{ID: "board-connectors", ParentID: "connectors", Label: "Board connectors", URL: "https://example.com/connectors/board", TargetType: "link", Presentation: "direct", SortOrder: 10, Visible: true},
	}
	configuration.Footer = site.Footer{
		Layout:     "brand_columns",
		BrandBlock: site.FooterBrandBlock{ShowBrand: true, ShowDescription: true, ContactIDs: []string{"support"}},
		Sections: []site.FooterSection{{
			ID: "products", Heading: "Products", SortOrder: 10, CollapsibleOnMobile: true,
			Items: []site.FooterItem{{ID: "catalog", Kind: "system_action", SystemAction: "catalog", SortOrder: 10}},
		}},
		CopyrightText: "Copyright Example Components",
		Disclaimer:    "Product information is subject to change.",
		LocaleControl: site.FooterLocaleControl{Enabled: true, Placement: "both"},
	}
	configuration.Theme.HeaderBackground = "#F0F1F2"
	configuration.Theme.FooterBackground = "#102030"

	body, err := HTML(PublicView{
		ID: "product_1", Revision: 3, SiteEpoch: 4, PartNumber: "ABC-123", Name: "Connector",
		CanonicalURL: "https://catalog.example.test/products/abc-123", Language: "en-US", DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US", "zh-TW"}, RFQURL: "/rfq", Site: configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	for _, expected := range []string{
		`class="site-header site-header-commerce is-sticky"`,
		`class="site-header-row utility-row"><a href="tel:12125550100">&#43;1 212 555 0100</a>`,
		`<summary>Products</summary>`,
		`<summary>Connectors</summary><a class="site-nav-overview" href="https://example.com/connectors" aria-label="Connectors" title="Connectors"><span aria-hidden="true">→</span></a>`,
		`href="https://example.com/connectors/board">Board connectors</a>`,
		`class="site-footer site-footer-brand_columns"`,
		`Electronic components and technical support.`,
		`href="tel:12125550100">Support: &#43;1 212 555 0100</a>`,
		`class="footer-section footer-section-collapsible"`,
		`Product information is subject to change.`,
		`--prods-header-bg:#F0F1F2`,
		`--prods-footer-bg:#102030`,
		`class="language-menu footer-language"`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered HTML does not contain %q:\n%s", expected, html)
		}
	}
	if strings.Contains(html, "ZgotmplZ") {
		t.Fatal("safe contact URL was sanitized by html/template")
	}
}
