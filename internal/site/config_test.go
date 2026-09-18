package site

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigurationJSONIncludesEmptyNavigation(t *testing.T) {
	configuration := DefaultConfiguration()
	configuration.Navigation = []NavigationItem{}

	encoded, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	navigation, exists := decoded["navigation"]
	if !exists {
		t.Fatal("empty navigation was omitted from configuration JSON")
	}
	items, ok := navigation.([]any)
	if !ok || len(items) != 0 {
		t.Fatalf("navigation = %#v, want empty array", navigation)
	}
}

func TestConfigurationPrepareValidatesNavigationAndCustomCSS(t *testing.T) {
	configuration := DefaultConfiguration()
	configuration.Organization.DisplayName = "  Example Components  "
	configuration.Navigation = append(configuration.Navigation,
		NavigationItem{ID: "support", Label: "Support", URL: "https://example.test/support", ParentID: "catalog", Visible: true},
	)
	if err := configuration.Prepare(); err != nil {
		t.Fatal(err)
	}
	if configuration.Organization.DisplayName != "Example Components" || len(configuration.VisibleNavigation()) != 3 {
		t.Fatalf("unexpected prepared configuration: %+v", configuration)
	}

	cycle := DefaultConfiguration()
	cycle.Navigation = []NavigationItem{
		{ID: "a", ParentID: "b", Label: "A", URL: "/a", Visible: true},
		{ID: "b", ParentID: "a", Label: "B", URL: "/b", Visible: true},
	}
	if err := cycle.Prepare(); err == nil {
		t.Fatal("expected navigation cycle rejection")
	}

	unsafeCSS := DefaultConfiguration()
	unsafeCSS.Theme.CustomCSSEnabled = true
	unsafeCSS.Theme.CustomCSS = `@import url("https://tracker.example/style.css");`
	if err := unsafeCSS.Prepare(); err == nil {
		t.Fatal("expected remote-loading CSS rejection")
	}
}

func TestCategoryListingProfilePrepare(t *testing.T) {
	configuration := DefaultConfiguration()
	configuration.CategoryListingProfiles = map[string]CategoryListingProfile{
		"cat_power": {
			VisibleColumns:       []string{" part_number ", "spec:vin", "rfq"},
			DefaultSort:          " name ",
			DefaultSortDirection: " DESC ",
			MobileKeySpecs:       []string{" spec:vin "},
		},
	}
	if err := configuration.Prepare(); err != nil {
		t.Fatal(err)
	}
	profile := configuration.CategoryListingProfiles["cat_power"]
	if got := strings.Join(profile.VisibleColumns, ","); got != "part_number,spec:vin,rfq" {
		t.Fatalf("visible columns = %q", got)
	}
	if profile.DefaultSort != "name" || profile.DefaultSortDirection != "desc" {
		t.Fatalf("sort = %q %q", profile.DefaultSort, profile.DefaultSortDirection)
	}
	if len(profile.MobileKeySpecs) != 1 || profile.MobileKeySpecs[0] != "spec:vin" {
		t.Fatalf("mobile keys = %v", profile.MobileKeySpecs)
	}
}

func TestCategoryListingProfileRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name     string
		category string
		profile  CategoryListingProfile
	}{
		{name: "category whitespace", category: " cat_power", profile: CategoryListingProfile{VisibleColumns: []string{"part_number"}}},
		{name: "missing part number", category: "cat_power", profile: CategoryListingProfile{VisibleColumns: []string{"name"}}},
		{name: "duplicate column", category: "cat_power", profile: CategoryListingProfile{VisibleColumns: []string{"part_number", "part_number"}}},
		{name: "unsupported column", category: "cat_power", profile: CategoryListingProfile{VisibleColumns: []string{"part_number", "price"}}},
		{name: "spec sort", category: "cat_power", profile: CategoryListingProfile{VisibleColumns: []string{"part_number", "spec:vin"}, DefaultSort: "spec:vin"}},
		{name: "hidden mobile key", category: "cat_power", profile: CategoryListingProfile{VisibleColumns: []string{"part_number"}, MobileKeySpecs: []string{"spec:vin"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := DefaultConfiguration()
			configuration.CategoryListingProfiles = map[string]CategoryListingProfile{test.category: test.profile}
			if err := configuration.Prepare(); err == nil {
				t.Fatal("expected invalid listing profile")
			}
		})
	}
}
