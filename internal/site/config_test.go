package site

import "testing"

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
