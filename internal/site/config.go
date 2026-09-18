package site

import (
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MaxNavigationItems       = 200
	MaxNavigationDepth       = 5
	MaxHeaderRows            = 4
	MaxHeaderItemsPerRow     = 20
	MaxFooterSections        = 6
	MaxFooterItemsPerSection = 20
	MaxCustomCSSBytes        = 64 << 10
	MaxListingProfiles       = 500
	MaxListingColumns        = 16
	MaxMobileKeySpecs        = 5
)

var (
	colorPattern        = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	fontPattern         = regexp.MustCompile(`^[A-Za-z0-9 ,.'"_-]{1,160}$`)
	specColumnPattern   = regexp.MustCompile(`^spec:[A-Za-z0-9_-]{1,128}$`)
	contactPhonePattern = regexp.MustCompile(`^\+?[0-9][0-9(). -]{2,39}$`)
)

type ContactLink struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	SortOrder int    `json:"sort_order"`
}

type ContactMethod struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Label     string `json:"label"`
	Value     string `json:"value"`
	URL       string `json:"url,omitempty"`
	SortOrder int    `json:"sort_order"`
}

type SocialLink struct {
	ID        string `json:"id"`
	Network   string `json:"network"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	SortOrder int    `json:"sort_order"`
}

type Organization struct {
	DisplayName       string          `json:"display_name"`
	LegalName         string          `json:"legal_name,omitempty"`
	OfficialWebsite   string          `json:"official_website,omitempty"`
	Description       string          `json:"description,omitempty"`
	PrivacyURL        string          `json:"privacy_url,omitempty"`
	TermsURL          string          `json:"terms_url,omitempty"`
	PrimaryLogoAsset  string          `json:"primary_logo_asset_id,omitempty"`
	DarkLogoAsset     string          `json:"dark_logo_asset_id,omitempty"`
	FaviconAsset      string          `json:"favicon_asset_id,omitempty"`
	SocialImageAsset  string          `json:"social_image_asset_id,omitempty"`
	ContactLinks      []ContactLink   `json:"contact_links,omitempty"`
	Contacts          []ContactMethod `json:"contacts,omitempty"`
	SocialLinks       []SocialLink    `json:"social_links,omitempty"`
	RegistrationLines []string        `json:"registration_lines,omitempty"`
}

type NavigationItem struct {
	ID            string `json:"id"`
	ParentID      string `json:"parent_id,omitempty"`
	Label         string `json:"label"`
	URL           string `json:"url"`
	TargetType    string `json:"target_type,omitempty"`
	SystemAction  string `json:"system_action,omitempty"`
	Presentation  string `json:"presentation,omitempty"`
	SortOrder     int    `json:"sort_order"`
	OpenNewWindow bool   `json:"open_new_window"`
	Visible       bool   `json:"visible"`
}

type HeaderItem struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Label        string `json:"label,omitempty"`
	URL          string `json:"url,omitempty"`
	SystemAction string `json:"system_action,omitempty"`
	Text         string `json:"text,omitempty"`
	SortOrder    int    `json:"sort_order"`
}

type HeaderRow struct {
	ID        string       `json:"id"`
	Type      string       `json:"type"`
	SortOrder int          `json:"sort_order"`
	Items     []HeaderItem `json:"items,omitempty"`
}

type Header struct {
	Layout            string      `json:"layout"`
	Sticky            bool        `json:"sticky"`
	BrandPresentation string      `json:"brand_presentation"`
	Rows              []HeaderRow `json:"rows,omitempty"`
}

type FooterItem struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Label        string `json:"label,omitempty"`
	URL          string `json:"url,omitempty"`
	Text         string `json:"text,omitempty"`
	ContactID    string `json:"contact_id,omitempty"`
	SystemAction string `json:"system_action,omitempty"`
	SortOrder    int    `json:"sort_order"`
}

type FooterSection struct {
	ID                  string       `json:"id"`
	Heading             string       `json:"heading"`
	SortOrder           int          `json:"sort_order"`
	CollapsibleOnMobile bool         `json:"collapsible_on_mobile"`
	Items               []FooterItem `json:"items,omitempty"`
}

type FooterBrandBlock struct {
	ShowBrand       bool     `json:"show_brand"`
	ShowDescription bool     `json:"show_description"`
	ContactIDs      []string `json:"contact_ids,omitempty"`
}

type FooterLocaleControl struct {
	Enabled   bool   `json:"enabled"`
	Placement string `json:"placement"`
}

type Footer struct {
	Layout            string              `json:"layout"`
	BrandBlock        FooterBrandBlock    `json:"brand_block"`
	Sections          []FooterSection     `json:"sections,omitempty"`
	ShowSocialLinks   bool                `json:"show_social_links"`
	LegalLinks        []ContactLink       `json:"legal_links,omitempty"`
	CopyrightText     string              `json:"copyright_text,omitempty"`
	RegistrationLines []string            `json:"registration_lines,omitempty"`
	Disclaimer        string              `json:"disclaimer,omitempty"`
	LocaleControl     FooterLocaleControl `json:"locale_control"`
}

type BrandImportProvenance struct {
	SourceURL     string    `json:"source_url"`
	ObservedURL   string    `json:"observed_url"`
	SourceLocale  string    `json:"source_locale"`
	PromptVersion string    `json:"prompt_version"`
	ImportedAt    time.Time `json:"imported_at"`
}

type Theme struct {
	PrimaryColor      string `json:"primary_color"`
	SecondaryColor    string `json:"secondary_color"`
	AccentColor       string `json:"accent_color,omitempty"`
	BodyTextColor     string `json:"body_text_color,omitempty"`
	BorderColor       string `json:"border_color,omitempty"`
	HeaderBackground  string `json:"header_background,omitempty"`
	HeaderTextColor   string `json:"header_text_color,omitempty"`
	FooterBackground  string `json:"footer_background,omitempty"`
	FooterTextColor   string `json:"footer_text_color,omitempty"`
	TypographyProfile string `json:"typography_profile,omitempty"`
	Density           string `json:"density,omitempty"`
	Radius            string `json:"radius,omitempty"`
	FontFamily        string `json:"font_family"`
	ContentWidthPX    int    `json:"content_width_px"`
	CustomCSS         string `json:"custom_css,omitempty"`
	CustomCSSEnabled  bool   `json:"custom_css_enabled"`
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
	Header                  Header                            `json:"header"`
	Navigation              []NavigationItem                  `json:"navigation"`
	Footer                  Footer                            `json:"footer"`
	Theme                   Theme                             `json:"theme"`
	SEO                     SEO                               `json:"seo"`
	BrandImport             *BrandImportProvenance            `json:"brand_import,omitempty"`
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
		Header: Header{
			Layout:            "commerce",
			BrandPresentation: "existing_logo_or_display_name",
			Rows: []HeaderRow{
				{ID: "main", Type: "main", SortOrder: 10, Items: []HeaderItem{
					{ID: "search", Kind: "system_action", SystemAction: "catalog_search", SortOrder: 10},
					{ID: "rfq", Kind: "system_action", SystemAction: "rfq", SortOrder: 20},
				}},
				{ID: "primary_navigation", Type: "primary_navigation", SortOrder: 20},
			},
		},
		Navigation: []NavigationItem{
			{ID: "catalog", Label: "Catalog", URL: "/catalog", TargetType: "system_action", SystemAction: "catalog", Presentation: "direct", SortOrder: 10, Visible: true},
			{ID: "rfq", Label: "Request quote", URL: "/rfq", TargetType: "system_action", SystemAction: "rfq", Presentation: "direct", SortOrder: 20, Visible: true},
		},
		Footer: Footer{
			Layout:        "compact",
			BrandBlock:    FooterBrandBlock{ShowBrand: true},
			LocaleControl: FooterLocaleControl{Enabled: true, Placement: "header"},
		},
		Theme: Theme{
			PrimaryColor: "#1677ff", SecondaryColor: "#475569", AccentColor: "#1677ff",
			BodyTextColor: "#172033", BorderColor: "#d7dde7",
			HeaderBackground: "#ffffff", HeaderTextColor: "#172033",
			FooterBackground: "#f6f8fb", FooterTextColor: "#172033",
			TypographyProfile: "system_sans", Density: "comfortable", Radius: "small",
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
	if err := prepareBrandLayout(configuration); err != nil {
		return err
	}

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
		if item.ID == "" || item.Label == "" || !validNavigationItem(item) {
			return errors.New("every navigation item requires a unique ID, label, and valid target")
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
		depth := 1
		for parentID != "" {
			depth++
			if depth > MaxNavigationDepth {
				return fmt.Errorf("navigation depth must not exceed %d", MaxNavigationDepth)
			}
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
	return fmt.Sprintf(`:root{--prods-primary:%s;--prods-primary-contrast:%s;--prods-secondary:%s;--prods-accent:%s;--prods-accent-contrast:%s;--prods-text:%s;--prods-border:%s;--prods-header-bg:%s;--prods-header-text:%s;--prods-footer-bg:%s;--prods-footer-text:%s;--prods-font:%s;--prods-content-width:%dpx;--prods-radius:%s;--prods-space:%s}body{font-family:var(--prods-font);margin:0;color:var(--prods-text)}main{max-width:var(--prods-content-width);margin:auto;padding:var(--prods-space)}.site-header{color:var(--prods-header-text);background:var(--prods-header-bg);border-bottom:1px solid var(--prods-border)}.site-footer{color:var(--prods-footer-text);background:var(--prods-footer-bg);border-top:1px solid var(--prods-border)}.site-header-row,.primary-navigation>ul,.footer-grid,.footer-social,.footer-legal,.footer-company{max-width:var(--prods-content-width);margin-inline:auto}a{color:var(--prods-primary)}button,.button{border-radius:var(--prods-radius)}`,
		configuration.Theme.PrimaryColor, primaryContrastColor(configuration.Theme.PrimaryColor), configuration.Theme.SecondaryColor, configuration.Theme.AccentColor, primaryContrastColor(configuration.Theme.AccentColor),
		configuration.Theme.BodyTextColor, configuration.Theme.BorderColor, configuration.Theme.HeaderBackground,
		configuration.Theme.HeaderTextColor, configuration.Theme.FooterBackground, configuration.Theme.FooterTextColor,
		configuration.Theme.FontFamily, configuration.Theme.ContentWidthPX, themeRadiusValue(configuration.Theme.Radius), themeSpacingValue(configuration.Theme.Density))
}

func primaryContrastColor(value string) string {
	if len(value) != 7 || value[0] != '#' {
		return "#FFFFFF"
	}
	parsed, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return "#FFFFFF"
	}
	r := (parsed >> 16) & 0xff
	g := (parsed >> 8) & 0xff
	b := parsed & 0xff
	if (r*299+g*587+b*114)/1000 >= 128 {
		return "#161616"
	}
	return "#FFFFFF"
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

// ValidContactURL reports whether a prepared contact link can be emitted as an
// HTML href. Contact links additionally support the safe mailto and tel
// schemes used by imported Website footers.
func ValidContactURL(value string) bool {
	if validExternalURL(value) {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.Host != "" || parsed.Opaque == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	switch parsed.Scheme {
	case "mailto":
		address, err := mail.ParseAddress(parsed.Opaque)
		return err == nil && address.Name == "" && address.Address == parsed.Opaque
	case "tel":
		return contactPhonePattern.MatchString(parsed.Opaque)
	default:
		return false
	}
}

// ValidHeaderLinkURL reports whether a prepared Header link can be emitted as
// an HTML href. Header links support normal navigation targets plus the safe
// mailto and tel schemes accepted for organization contacts.
func ValidHeaderLinkURL(value string) bool {
	return validNavigationURL(value) || ValidContactURL(value)
}

func validNavigationURL(value string) bool {
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		parsed, err := url.Parse(value)
		return err == nil && parsed.Host == "" && parsed.Scheme == "" && !strings.Contains(parsed.Path, "..")
	}
	return validExternalURL(value)
}
