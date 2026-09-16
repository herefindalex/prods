package site

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	MaxNavigationItems = 200
	MaxCustomCSSBytes  = 64 << 10
	MaxListingProfiles = 500
	MaxListingColumns  = 16
	MaxMobileKeySpecs  = 5
)

var (
	colorPattern      = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	fontPattern       = regexp.MustCompile(`^[A-Za-z0-9 ,.'"_-]{1,160}$`)
	specColumnPattern = regexp.MustCompile(`^spec:[A-Za-z0-9_-]{1,128}$`)
)

type ContactLink struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	SortOrder int    `json:"sort_order"`
}

type Organization struct {
	DisplayName      string        `json:"display_name"`
	LegalName        string        `json:"legal_name,omitempty"`
	OfficialWebsite  string        `json:"official_website,omitempty"`
	PrivacyURL       string        `json:"privacy_url,omitempty"`
	TermsURL         string        `json:"terms_url,omitempty"`
	PrimaryLogoAsset string        `json:"primary_logo_asset_id,omitempty"`
	DarkLogoAsset    string        `json:"dark_logo_asset_id,omitempty"`
	FaviconAsset     string        `json:"favicon_asset_id,omitempty"`
	SocialImageAsset string        `json:"social_image_asset_id,omitempty"`
	ContactLinks     []ContactLink `json:"contact_links,omitempty"`
}

type NavigationItem struct {
	ID            string `json:"id"`
	ParentID      string `json:"parent_id,omitempty"`
	Label         string `json:"label"`
	URL           string `json:"url"`
	SortOrder     int    `json:"sort_order"`
	OpenNewWindow bool   `json:"open_new_window"`
	Visible       bool   `json:"visible"`
}

type Theme struct {
	PrimaryColor     string `json:"primary_color"`
	SecondaryColor   string `json:"secondary_color"`
	FontFamily       string `json:"font_family"`
	ContentWidthPX   int    `json:"content_width_px"`
	CustomCSS        string `json:"custom_css,omitempty"`
	CustomCSSEnabled bool   `json:"custom_css_enabled"`
}

type SEO struct {
	DefaultTitle       string `json:"default_title,omitempty"`
	DefaultDescription string `json:"default_description,omitempty"`
}

type CategoryListingProfile struct {
	VisibleColumns       []string `json:"visible_columns"`
	DefaultSort          string   `json:"default_sort,omitempty"`
	DefaultSortDirection string   `json:"default_sort_direction,omitempty"`
	MobileKeySpecs       []string `json:"mobile_key_specs,omitempty"`
}

type Configuration struct {
	CategoryListingProfiles map[string]CategoryListingProfile `json:"category_listing_profiles,omitempty"`
	Organization            Organization                      `json:"organization"`
	Navigation              []NavigationItem                  `json:"navigation,omitempty"`
	Theme                   Theme                             `json:"theme"`
	SEO                     SEO                               `json:"seo"`
}

type State struct {
	WorkingRevision     int64               `json:"working_revision"`
	ActiveVersion       int64               `json:"active_version"`
	ActiveEpoch         int64               `json:"active_epoch"`
	CustomCSSDisabled   bool                `json:"custom_css_disabled"`
	RuntimeGeneration   int64               `json:"runtime_generation"`
	Working             Configuration       `json:"working"`
	Active              Configuration       `json:"active"`
	WorkingLocalization WebsiteLocalization `json:"working_localization"`
	ActiveLocalization  WebsiteLocalization `json:"active_localization"`
}

type Version struct {
	Version               int64               `json:"version"`
	SourceWorkingRevision int64               `json:"source_working_revision"`
	SiteEpoch             int64               `json:"site_epoch"`
	Configuration         Configuration       `json:"configuration"`
	Localization          WebsiteLocalization `json:"localization"`
	CreatedAt             time.Time           `json:"created_at"`
}

func DefaultConfiguration() Configuration {
	return Configuration{
		Organization: Organization{DisplayName: "Product Catalog"},
		Navigation: []NavigationItem{
			{ID: "catalog", Label: "Catalog", URL: "/catalog", SortOrder: 10, Visible: true},
			{ID: "rfq", Label: "Request quote", URL: "/rfq", SortOrder: 20, Visible: true},
		},
		Theme: Theme{
			PrimaryColor: "#1677ff", SecondaryColor: "#475569",
			FontFamily: "system-ui, sans-serif", ContentWidthPX: 1120,
		},
		SEO: SEO{DefaultTitle: "Product Catalog"},
	}
}

func (configuration *Configuration) Prepare() error {
	configuration.Organization.DisplayName = strings.TrimSpace(configuration.Organization.DisplayName)
	configuration.Organization.LegalName = strings.TrimSpace(configuration.Organization.LegalName)
	configuration.Organization.OfficialWebsite = strings.TrimSpace(configuration.Organization.OfficialWebsite)
	configuration.Organization.PrivacyURL = strings.TrimSpace(configuration.Organization.PrivacyURL)
	configuration.Organization.TermsURL = strings.TrimSpace(configuration.Organization.TermsURL)
	configuration.Theme.PrimaryColor = strings.TrimSpace(configuration.Theme.PrimaryColor)
	configuration.Theme.SecondaryColor = strings.TrimSpace(configuration.Theme.SecondaryColor)
	configuration.Theme.FontFamily = strings.TrimSpace(configuration.Theme.FontFamily)
	configuration.Theme.CustomCSS = strings.TrimSpace(configuration.Theme.CustomCSS)
	configuration.SEO.DefaultTitle = strings.TrimSpace(configuration.SEO.DefaultTitle)
	configuration.SEO.DefaultDescription = strings.TrimSpace(configuration.SEO.DefaultDescription)

	if configuration.Organization.DisplayName == "" || len(configuration.Organization.DisplayName) > 200 {
		return errors.New("organization display name is required and must be at most 200 characters")
	}
	for field, value := range map[string]string{
		"official website": configuration.Organization.OfficialWebsite,
		"privacy URL":      configuration.Organization.PrivacyURL,
		"terms URL":        configuration.Organization.TermsURL,
	} {
		if value != "" && !validExternalURL(value) {
			return fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", field)
		}
	}
	if len(configuration.Organization.ContactLinks) > 50 {
		return errors.New("at most 50 organization contact links are allowed")
	}
	contactIDs := make(map[string]struct{}, len(configuration.Organization.ContactLinks))
	for index := range configuration.Organization.ContactLinks {
		contact := &configuration.Organization.ContactLinks[index]
		contact.ID = strings.TrimSpace(contact.ID)
		contact.Label = strings.TrimSpace(contact.Label)
		contact.URL = strings.TrimSpace(contact.URL)
		if contact.ID == "" || contact.Label == "" || !validExternalURL(contact.URL) {
			return errors.New("every contact link requires a unique ID, label, and absolute HTTP or HTTPS URL")
		}
		if _, exists := contactIDs[contact.ID]; exists {
			return fmt.Errorf("duplicate contact link ID %q", contact.ID)
		}
		contactIDs[contact.ID] = struct{}{}
	}

	if len(configuration.Navigation) > MaxNavigationItems {
		return fmt.Errorf("at most %d navigation items are allowed", MaxNavigationItems)
	}
	items := make(map[string]NavigationItem, len(configuration.Navigation))
	for index := range configuration.Navigation {
		item := &configuration.Navigation[index]
		item.ID = strings.TrimSpace(item.ID)
		item.ParentID = strings.TrimSpace(item.ParentID)
		item.Label = strings.TrimSpace(item.Label)
		item.URL = strings.TrimSpace(item.URL)
		if item.ID == "" || item.Label == "" || !validNavigationURL(item.URL) {
			return errors.New("every navigation item requires a unique ID, label, and safe internal or HTTP(S) URL")
		}
		if _, exists := items[item.ID]; exists {
			return fmt.Errorf("duplicate navigation item ID %q", item.ID)
		}
		items[item.ID] = *item
	}
	for _, item := range items {
		if item.ParentID != "" {
			if _, exists := items[item.ParentID]; !exists {
				return fmt.Errorf("navigation parent %q does not exist", item.ParentID)
			}
		}
		seen := map[string]struct{}{item.ID: {}}
		parentID := item.ParentID
		for parentID != "" {
			if _, cycle := seen[parentID]; cycle {
				return errors.New("navigation must not contain a cycle")
			}
			seen[parentID] = struct{}{}
			parentID = items[parentID].ParentID
		}
	}

	if !colorPattern.MatchString(configuration.Theme.PrimaryColor) || !colorPattern.MatchString(configuration.Theme.SecondaryColor) {
		return errors.New("theme colors must use six-digit hexadecimal notation")
	}
	if !fontPattern.MatchString(configuration.Theme.FontFamily) {
		return errors.New("theme font family is invalid")
	}
	if configuration.Theme.ContentWidthPX < 640 || configuration.Theme.ContentWidthPX > 1920 {
		return errors.New("theme content width must be between 640 and 1920 pixels")
	}
	if len(configuration.Theme.CustomCSS) > MaxCustomCSSBytes {
		return fmt.Errorf("custom CSS must be at most %d bytes", MaxCustomCSSBytes)
	}
	lowerCSS := strings.ToLower(configuration.Theme.CustomCSS)
	for _, forbidden := range []string{"</style", "@import", "url(", "expression(", "javascript:"} {
		if strings.Contains(lowerCSS, forbidden) {
			return fmt.Errorf("custom CSS contains forbidden construct %q", forbidden)
		}
	}
	if len(configuration.SEO.DefaultTitle) > 200 || len(configuration.SEO.DefaultDescription) > 500 {
		return errors.New("SEO title or description is too long")
	}
	if err := prepareListingProfiles(configuration.CategoryListingProfiles); err != nil {
		return err
	}
	return nil
}

func prepareListingProfiles(profiles map[string]CategoryListingProfile) error {
	if len(profiles) > MaxListingProfiles {
		return fmt.Errorf("at most %d category listing profiles allowed", MaxListingProfiles)
	}
	for categoryID, profile := range profiles {
		if strings.TrimSpace(categoryID) == "" || strings.TrimSpace(categoryID) != categoryID || len(profile.VisibleColumns) == 0 || len(profile.VisibleColumns) > MaxListingColumns {
			return errors.New("category listing profile requires a category and visible columns")
		}
		seen := make(map[string]struct{}, len(profile.VisibleColumns))
		partNumberVisible := false
		for index := range profile.VisibleColumns {
			key := strings.TrimSpace(profile.VisibleColumns[index])
			if !validListingColumn(key) {
				return fmt.Errorf("invalid listing column %q", key)
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate listing column %q", key)
			}
			seen[key] = struct{}{}
			profile.VisibleColumns[index] = key
			partNumberVisible = partNumberVisible || key == "part_number"
		}
		if !partNumberVisible {
			return errors.New("category listing profile must keep part_number visible")
		}
		profile.DefaultSort = strings.TrimSpace(profile.DefaultSort)
		profile.DefaultSortDirection = strings.ToLower(strings.TrimSpace(profile.DefaultSortDirection))
		if profile.DefaultSort == "" {
			profile.DefaultSort = "part_number"
		}
		if profile.DefaultSortDirection == "" {
			profile.DefaultSortDirection = "asc"
		}
		if !sortableListingColumn(profile.DefaultSort) || (profile.DefaultSortDirection != "asc" && profile.DefaultSortDirection != "desc") {
			return errors.New("category listing profile has unsupported default sort")
		}
		if len(profile.MobileKeySpecs) > MaxMobileKeySpecs {
			return fmt.Errorf("at most %d mobile key specs allowed", MaxMobileKeySpecs)
		}
		mobileSeen := make(map[string]struct{}, len(profile.MobileKeySpecs))
		for index := range profile.MobileKeySpecs {
			key := strings.TrimSpace(profile.MobileKeySpecs[index])
			if !specColumnPattern.MatchString(key) {
				return fmt.Errorf("mobile key spec %q is not a spec column", key)
			}
			if _, visible := seen[key]; !visible {
				return fmt.Errorf("mobile key spec %q is not visible", key)
			}
			if _, duplicate := mobileSeen[key]; duplicate {
				return fmt.Errorf("duplicate mobile key spec %q", key)
			}
			mobileSeen[key] = struct{}{}
			profile.MobileKeySpecs[index] = key
		}
		profiles[categoryID] = profile
	}
	return nil
}

func validListingColumn(key string) bool {
	switch key {
	case "part_number", "name", "manufacturer", "brand", "category", "package", "lifecycle", "documents", "rfq":
		return true
	default:
		return specColumnPattern.MatchString(key)
	}
}

func sortableListingColumn(key string) bool {
	switch key {
	case "part_number", "name", "manufacturer", "brand", "lifecycle":
		return true
	default:
		return false
	}
}

func (configuration Configuration) VisibleNavigation() []NavigationItem {
	items := make([]NavigationItem, 0, len(configuration.Navigation))
	for _, item := range configuration.Navigation {
		if item.Visible {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].SortOrder == items[j].SortOrder {
			return items[i].ID < items[j].ID
		}
		return items[i].SortOrder < items[j].SortOrder
	})
	return items
}

func (configuration Configuration) AssetIDs() []string {
	seen := make(map[string]struct{})
	assets := make([]string, 0, 4)
	for _, id := range []string{
		configuration.Organization.PrimaryLogoAsset,
		configuration.Organization.DarkLogoAsset,
		configuration.Organization.FaviconAsset,
		configuration.Organization.SocialImageAsset,
	} {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		assets = append(assets, id)
	}
	return assets
}

func (configuration Configuration) Stylesheet() string {
	return fmt.Sprintf(`:root{--prods-primary:%s;--prods-secondary:%s;--prods-font:%s;--prods-content-width:%dpx}body{font-family:var(--prods-font);margin:0;color:#172033}header,main,footer{max-width:var(--prods-content-width);margin:auto;padding:1rem}header{display:flex;justify-content:space-between;gap:1rem}nav a{margin-inline-start:1rem}a{color:var(--prods-primary)}`,
		configuration.Theme.PrimaryColor, configuration.Theme.SecondaryColor, configuration.Theme.FontFamily, configuration.Theme.ContentWidthPX)
}

func (configuration Configuration) CustomStylesheet() string {
	if !configuration.Theme.CustomCSSEnabled {
		return ""
	}
	return configuration.Theme.CustomCSS
}

func validExternalURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func validNavigationURL(value string) bool {
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		parsed, err := url.Parse(value)
		return err == nil && parsed.Host == "" && parsed.Scheme == "" && !strings.Contains(parsed.Path, "..")
	}
	return validExternalURL(value)
}
