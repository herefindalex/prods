package site

import (
	"strings"
	"testing"
)

func TestVisibleNavigationTreePreservesFiveLevelHierarchy(t *testing.T) {
	configuration := DefaultConfiguration()
	configuration.Navigation = []NavigationItem{
		{ID: "level1", Label: "Level 1", TargetType: "group", Presentation: "dropdown", SortOrder: 10, Visible: true},
		{ID: "level2", ParentID: "level1", Label: "Level 2", TargetType: "group", Presentation: "dropdown", SortOrder: 10, Visible: true},
		{ID: "level3", ParentID: "level2", Label: "Level 3", TargetType: "group", Presentation: "dropdown", SortOrder: 10, Visible: true},
		{ID: "level4", ParentID: "level3", Label: "Level 4", TargetType: "group", Presentation: "dropdown", SortOrder: 10, Visible: true},
		{ID: "level5", ParentID: "level4", Label: "Level 5", URL: "/catalog", TargetType: "link", Presentation: "direct", SortOrder: 10, Visible: true},
	}
	if err := configuration.Prepare(); err != nil {
		t.Fatal(err)
	}
	tree := configuration.VisibleNavigationTree()
	if len(tree) != 1 || len(tree[0].Children) != 1 || len(tree[0].Children[0].Children) != 1 || len(tree[0].Children[0].Children[0].Children) != 1 || len(tree[0].Children[0].Children[0].Children[0].Children) != 1 {
		t.Fatalf("navigation hierarchy was not preserved: %+v", tree)
	}

	configuration.Navigation = append(configuration.Navigation, NavigationItem{ID: "level6", ParentID: "level5", Label: "Level 6", URL: "/catalog", TargetType: "link", Presentation: "direct", SortOrder: 10, Visible: true})
	if err := configuration.Prepare(); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("six-level navigation error=%v", err)
	}
}

func TestLegacyConfigurationKeepsHeaderLocaleAndBrandDefaults(t *testing.T) {
	configuration := Configuration{
		Organization: Organization{DisplayName: "Catalog"},
		Theme:        Theme{PrimaryColor: "#112233", SecondaryColor: "#445566", FontFamily: "system-ui", ContentWidthPX: 1120},
	}
	if err := configuration.Prepare(); err != nil {
		t.Fatal(err)
	}
	if !configuration.ShowHeaderLocale() || configuration.ShowFooterLocale() {
		t.Fatalf("legacy locale placement changed: %+v", configuration.Footer.LocaleControl)
	}
	if configuration.Header.Layout == "" || configuration.Footer.Layout == "" || configuration.Theme.HeaderBackground == "" || configuration.Theme.FooterBackground == "" {
		t.Fatalf("brand layout defaults were not prepared: %+v", configuration)
	}
}

func TestConfigurationAcceptsSafeContactSchemes(t *testing.T) {
	for _, contactURL := range []string{
		"https://example.com/contact",
		"mailto:sales@example.com",
		"tel:+1-212-555-0100",
	} {
		configuration := DefaultConfiguration()
		configuration.Organization.Contacts = []ContactMethod{{
			ID: "sales", Type: "sales", Label: "Sales", Value: "Contact sales", URL: contactURL, SortOrder: 10,
		}}
		if err := configuration.Prepare(); err != nil {
			t.Fatalf("contact URL %q rejected: %v", contactURL, err)
		}
	}
}

func TestConfigurationRejectsUnsafeContactSchemes(t *testing.T) {
	for _, contactURL := range []string{
		"javascript:alert(1)",
		"mailto:Sales Team <sales@example.com>",
		"mailto:sales@example.com?subject=Hello",
		"tel:123;ext=456",
	} {
		configuration := DefaultConfiguration()
		configuration.Organization.Contacts = []ContactMethod{{
			ID: "sales", Type: "sales", Label: "Sales", Value: "Contact sales", URL: contactURL, SortOrder: 10,
		}}
		if err := configuration.Prepare(); err == nil {
			t.Fatalf("unsafe contact URL %q was accepted", contactURL)
		}
	}
}

func TestHeaderLinksAcceptSafeContactSchemes(t *testing.T) {
	for _, contactURL := range []string{
		"mailto:sales@example.com",
		"tel:+1-212-555-0100",
	} {
		configuration := DefaultConfiguration()
		configuration.Header.Rows = []HeaderRow{{
			ID:        "utility",
			Type:      "utility",
			SortOrder: 10,
			Items: []HeaderItem{{
				ID:        "contact",
				Kind:      "link",
				Label:     "Contact",
				URL:       contactURL,
				SortOrder: 10,
			}},
		}}
		if err := configuration.Prepare(); err != nil {
			t.Fatalf("header contact URL %q rejected: %v", contactURL, err)
		}
	}
}

func TestHeaderLinksRejectUnsafeContactSchemes(t *testing.T) {
	for _, contactURL := range []string{
		"javascript:alert(1)",
		"mailto:sales@example.com?subject=Hello",
		"tel:123;ext=456",
	} {
		configuration := DefaultConfiguration()
		configuration.Header.Rows = []HeaderRow{{
			ID:        "utility",
			Type:      "utility",
			SortOrder: 10,
			Items: []HeaderItem{{
				ID:        "contact",
				Kind:      "link",
				Label:     "Contact",
				URL:       contactURL,
				SortOrder: 10,
			}},
		}}
		if err := configuration.Prepare(); err == nil {
			t.Fatalf("unsafe header contact URL %q was accepted", contactURL)
		}
	}
}
