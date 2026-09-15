package brandcapture

import (
	"reflect"
	"testing"

	"prods/internal/site"
)

func TestBuildProposalChangesOnlyCapturedFieldsAndKeepsCorePaths(t *testing.T) {
	current := site.DefaultConfiguration()
	current.Organization.LegalName = "Existing Legal LLC"
	current.Organization.PrimaryLogoAsset = "asset-logo"
	current.Organization.DarkLogoAsset = "asset-dark"
	current.Organization.ContactLinks = []site.ContactLink{{ID: "sales", Label: "Sales", URL: "https://old.example/sales", SortOrder: 1}}
	current.Theme.CustomCSS = "h1{letter-spacing:.02em}"
	current.Theme.CustomCSSEnabled = true
	current.Theme.ContentWidthPX = 1440
	current.SEO = site.SEO{DefaultTitle: "Manual title", DefaultDescription: "Manual description"}
	candidate := Candidate{
		SourceURL: "https://www.new.example/about", Organization: "New Example",
		Colors: []string{"#112233", "#445566"}, Fonts: []string{"Inter, sans-serif"},
		Navigation: []Navigation{
			{Label: "Products", URL: "https://www.new.example/products"},
			{Label: "Catalog", URL: "https://www.new.example/catalog"},
			{Label: "Support", URL: "https://www.new.example/support"},
		},
		NeedsRightsConfirmation: true, WorkingCopyOnly: true,
	}
	proposed, changes, err := BuildProposal(current, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Organization.DisplayName != "New Example" || proposed.Organization.OfficialWebsite != "https://www.new.example" {
		t.Fatalf("organization proposal=%+v", proposed.Organization)
	}
	if proposed.Theme.PrimaryColor != "#112233" || proposed.Theme.SecondaryColor != "#445566" || proposed.Theme.FontFamily != "Inter, sans-serif" {
		t.Fatalf("theme proposal=%+v", proposed.Theme)
	}
	if proposed.Organization.LegalName != current.Organization.LegalName ||
		proposed.Organization.PrimaryLogoAsset != current.Organization.PrimaryLogoAsset ||
		proposed.Organization.DarkLogoAsset != current.Organization.DarkLogoAsset ||
		!reflect.DeepEqual(proposed.Organization.ContactLinks, current.Organization.ContactLinks) ||
		proposed.Theme.CustomCSS != current.Theme.CustomCSS || proposed.Theme.CustomCSSEnabled != current.Theme.CustomCSSEnabled ||
		proposed.Theme.ContentWidthPX != current.Theme.ContentWidthPX || !reflect.DeepEqual(proposed.SEO, current.SEO) {
		t.Fatalf("proposal overwrote unrelated manual state: %+v", proposed)
	}
	if navigationURLCount(proposed.Navigation, "/search") != 1 || navigationURLCount(proposed.Navigation, "/rfq") != 1 {
		t.Fatalf("core navigation was not preserved exactly once: %+v", proposed.Navigation)
	}
	if navigationURLCount(proposed.Navigation, "https://www.new.example/support") != 1 {
		t.Fatalf("captured navigation missing: %+v", proposed.Navigation)
	}
	changedFields := make(map[string]bool, len(changes))
	for _, change := range changes {
		changedFields[change.Field] = true
	}
	for _, field := range []string{"organization.display_name", "organization.official_website", "theme.primary_color", "theme.secondary_color", "theme.font_family", "navigation"} {
		if !changedFields[field] {
			t.Errorf("diff missing %s: %+v", field, changes)
		}
	}
	for _, forbidden := range []string{"seo", "custom_css", "primary_logo_asset_id"} {
		if changedFields[forbidden] {
			t.Errorf("diff unexpectedly contains %s", forbidden)
		}
	}
}

func TestBuildProposalWithoutCapturedNavigationPreservesManualNavigation(t *testing.T) {
	current := site.DefaultConfiguration()
	current.Navigation = append(current.Navigation, site.NavigationItem{ID: "manual", Label: "Manual", URL: "https://example.com/manual", SortOrder: 30, Visible: false})
	proposed, _, err := BuildProposal(current, Candidate{SourceURL: "https://example.com", WorkingCopyOnly: true, NeedsRightsConfirmation: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(proposed.Navigation, current.Navigation) {
		t.Fatalf("missing capture replaced manual navigation: before=%+v after=%+v", current.Navigation, proposed.Navigation)
	}
}

func navigationURLCount(items []site.NavigationItem, target string) int {
	count := 0
	for _, item := range items {
		if item.URL == target {
			count++
		}
	}
	return count
}
