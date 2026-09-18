package site

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func themeRadiusValue(radius string) string {
	switch radius {
	case "none":
		return "0"
	case "medium":
		return ".8rem"
	default:
		return ".4rem"
	}
}

func themeSpacingValue(density string) string {
	switch density {
	case "compact":
		return ".75rem"
	case "spacious":
		return "1.5rem"
	default:
		return "1rem"
	}
}

var brandKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

var allowedSystemActions = map[string]struct{}{
	"catalog": {}, "catalog_search": {}, "rfq": {}, "locale": {},
}

type NavigationNode struct {
	Item     NavigationItem
	Children []*NavigationNode
}

func (configuration Configuration) VisibleNavigationTree() []*NavigationNode {
	items := configuration.VisibleNavigation()
	nodes := make(map[string]*NavigationNode, len(items))
	for _, item := range items {
		copy := item
		nodes[item.ID] = &NavigationNode{Item: copy}
	}
	roots := make([]*NavigationNode, 0, len(items))
	for _, item := range items {
		node := nodes[item.ID]
		if item.ParentID == "" {
			roots = append(roots, node)
			continue
		}
		if parent := nodes[item.ParentID]; parent != nil {
			parent.Children = append(parent.Children, node)
		}
	}
	return roots
}

func prepareBrandLayout(configuration *Configuration) error {
	prepareThemeDefaults(&configuration.Theme)
	if err := validateThemeTokens(configuration.Theme); err != nil {
		return err
	}
	if err := prepareOrganization(&configuration.Organization); err != nil {
		return err
	}
	if err := prepareHeader(&configuration.Header); err != nil {
		return err
	}
	if err := prepareFooter(&configuration.Footer, configuration.Organization); err != nil {
		return err
	}
	if provenance := configuration.BrandImport; provenance != nil {
		provenance.SourceURL = strings.TrimSpace(provenance.SourceURL)
		provenance.ObservedURL = strings.TrimSpace(provenance.ObservedURL)
		provenance.SourceLocale = strings.TrimSpace(provenance.SourceLocale)
		provenance.PromptVersion = strings.TrimSpace(provenance.PromptVersion)
		if !validExternalURL(provenance.SourceURL) || !validExternalURL(provenance.ObservedURL) || provenance.SourceLocale == "" || len(provenance.SourceLocale) > 35 {
			return errors.New("brand import provenance is invalid")
		}
	}
	return nil
}

func prepareThemeDefaults(theme *Theme) {
	theme.AccentColor = strings.TrimSpace(theme.AccentColor)
	theme.BodyTextColor = strings.TrimSpace(theme.BodyTextColor)
	theme.BorderColor = strings.TrimSpace(theme.BorderColor)
	theme.HeaderBackground = strings.TrimSpace(theme.HeaderBackground)
	theme.HeaderTextColor = strings.TrimSpace(theme.HeaderTextColor)
	theme.FooterBackground = strings.TrimSpace(theme.FooterBackground)
	theme.FooterTextColor = strings.TrimSpace(theme.FooterTextColor)
	theme.TypographyProfile = strings.TrimSpace(theme.TypographyProfile)
	theme.Density = strings.TrimSpace(theme.Density)
	theme.Radius = strings.TrimSpace(theme.Radius)
	if theme.AccentColor == "" {
		theme.AccentColor = theme.PrimaryColor
	}
	if theme.BodyTextColor == "" {
		theme.BodyTextColor = "#172033"
	}
	if theme.BorderColor == "" {
		theme.BorderColor = "#d7dde7"
	}
	if theme.HeaderBackground == "" {
		theme.HeaderBackground = "#ffffff"
	}
	if theme.HeaderTextColor == "" {
		theme.HeaderTextColor = theme.BodyTextColor
	}
	if theme.FooterBackground == "" {
		theme.FooterBackground = "#f6f8fb"
	}
	if theme.FooterTextColor == "" {
		theme.FooterTextColor = theme.BodyTextColor
	}
	if theme.TypographyProfile == "" {
		theme.TypographyProfile = "system_sans"
	}
	if theme.Density == "" {
		theme.Density = "comfortable"
	}
	if theme.Radius == "" {
		theme.Radius = "small"
	}
}

func validateThemeTokens(theme Theme) error {
	for _, color := range []string{theme.AccentColor, theme.BodyTextColor, theme.BorderColor, theme.HeaderBackground, theme.HeaderTextColor, theme.FooterBackground, theme.FooterTextColor} {
		if !colorPattern.MatchString(color) {
			return errors.New("theme surface colors must use six-digit hexadecimal notation")
		}
	}
	if !oneOf(theme.TypographyProfile, "system_sans", "geometric_sans", "humanist_sans", "industrial_sans", "serif_accent") || !oneOf(theme.Density, "compact", "comfortable", "spacious") || !oneOf(theme.Radius, "none", "small", "medium") {
		return errors.New("theme typography, density, or radius is invalid")
	}
	return nil
}

func prepareOrganization(organization *Organization) error {
	organization.Description = strings.TrimSpace(organization.Description)
	if len(organization.Description) > 1000 {
		return errors.New("organization description is too long")
	}
	contactIDs := make(map[string]struct{}, len(organization.Contacts))
	for index := range organization.Contacts {
		contact := &organization.Contacts[index]
		contact.ID = strings.TrimSpace(contact.ID)
		contact.Type = strings.TrimSpace(contact.Type)
		contact.Label = strings.TrimSpace(contact.Label)
		contact.Value = strings.TrimSpace(contact.Value)
		contact.URL = strings.TrimSpace(contact.URL)
		if !validBrandKey(contact.ID) || !oneOf(contact.Type, "address", "phone", "email", "sales", "support", "other") || contact.Label == "" || contact.Value == "" || len(contact.Label) > 120 || len(contact.Value) > 500 {
			return errors.New("organization contact is invalid")
		}
		if contact.URL != "" && !ValidContactURL(contact.URL) {
			return errors.New("organization contact URL is invalid")
		}
		if _, exists := contactIDs[contact.ID]; exists {
			return fmt.Errorf("duplicate organization contact %q", contact.ID)
		}
		contactIDs[contact.ID] = struct{}{}
	}
	socialIDs := make(map[string]struct{}, len(organization.SocialLinks))
	for index := range organization.SocialLinks {
		link := &organization.SocialLinks[index]
		link.ID = strings.TrimSpace(link.ID)
		link.Network = strings.TrimSpace(link.Network)
		link.Label = strings.TrimSpace(link.Label)
		link.URL = strings.TrimSpace(link.URL)
		if !validBrandKey(link.ID) || link.Network == "" || link.Label == "" || !validExternalURL(link.URL) {
			return errors.New("organization social link is invalid")
		}
		if _, exists := socialIDs[link.ID]; exists {
			return fmt.Errorf("duplicate organization social link %q", link.ID)
		}
		socialIDs[link.ID] = struct{}{}
	}
	for index := range organization.RegistrationLines {
		organization.RegistrationLines[index] = strings.TrimSpace(organization.RegistrationLines[index])
		if len(organization.RegistrationLines[index]) > 300 {
			return errors.New("organization registration line is too long")
		}
	}
	return nil
}

func prepareHeader(header *Header) error {
	if header.Layout == "" {
		header.Layout = "commerce"
	}
	if !oneOf(header.Layout, "commerce", "corporate", "compact") {
		return errors.New("header layout is invalid")
	}
	if header.BrandPresentation == "" {
		header.BrandPresentation = "existing_logo_or_display_name"
	}
	if header.BrandPresentation != "existing_logo_or_display_name" {
		return errors.New("header brand presentation is invalid")
	}
	if len(header.Rows) == 0 {
		header.Rows = []HeaderRow{
			{ID: "main", Type: "main", SortOrder: 10, Items: []HeaderItem{{ID: "search", Kind: "system_action", SystemAction: "catalog_search", SortOrder: 10}, {ID: "rfq", Kind: "system_action", SystemAction: "rfq", SortOrder: 20}}},
			{ID: "primary_navigation", Type: "primary_navigation", SortOrder: 20},
		}
	}
	if len(header.Rows) > MaxHeaderRows {
		return fmt.Errorf("at most %d header rows are allowed", MaxHeaderRows)
	}
	rowIDs := make(map[string]struct{}, len(header.Rows))
	for rowIndex := range header.Rows {
		row := &header.Rows[rowIndex]
		row.ID = strings.TrimSpace(row.ID)
		row.Type = strings.TrimSpace(row.Type)
		if !validBrandKey(row.ID) || !oneOf(row.Type, "announcement", "utility", "main", "primary_navigation") {
			return errors.New("header row is invalid")
		}
		if _, exists := rowIDs[row.ID]; exists {
			return fmt.Errorf("duplicate header row %q", row.ID)
		}
		rowIDs[row.ID] = struct{}{}
		if len(row.Items) > MaxHeaderItemsPerRow {
			return fmt.Errorf("at most %d items are allowed in one header row", MaxHeaderItemsPerRow)
		}
		itemIDs := make(map[string]struct{}, len(row.Items))
		for itemIndex := range row.Items {
			item := &row.Items[itemIndex]
			item.ID = strings.TrimSpace(item.ID)
			item.Kind = strings.TrimSpace(item.Kind)
			item.Label = strings.TrimSpace(item.Label)
			item.URL = strings.TrimSpace(item.URL)
			item.SystemAction = strings.TrimSpace(item.SystemAction)
			item.Text = strings.TrimSpace(item.Text)
			if !validBrandKey(item.ID) || !validHeaderItem(*item) {
				return errors.New("header item is invalid")
			}
			if _, exists := itemIDs[item.ID]; exists {
				return fmt.Errorf("duplicate header item %q", item.ID)
			}
			itemIDs[item.ID] = struct{}{}
		}
		sort.SliceStable(row.Items, func(i, j int) bool {
			if row.Items[i].SortOrder == row.Items[j].SortOrder {
				return row.Items[i].ID < row.Items[j].ID
			}
			return row.Items[i].SortOrder < row.Items[j].SortOrder
		})
	}
	sort.SliceStable(header.Rows, func(i, j int) bool {
		if header.Rows[i].SortOrder == header.Rows[j].SortOrder {
			return header.Rows[i].ID < header.Rows[j].ID
		}
		return header.Rows[i].SortOrder < header.Rows[j].SortOrder
	})
	return nil
}

func validHeaderItem(item HeaderItem) bool {
	switch item.Kind {
	case "link":
		return item.Label != "" && len(item.Label) <= 120 && ValidHeaderLinkURL(item.URL)
	case "system_action":
		return validSystemAction(item.SystemAction)
	case "text":
		return item.Text != "" && len(item.Text) <= 300
	default:
		return false
	}
}

func prepareFooter(footer *Footer, organization Organization) error {
	if footer.Layout == "" {
		footer.Layout = "compact"
	}
	if !oneOf(footer.Layout, "brand_columns", "columns", "compact") {
		return errors.New("footer layout is invalid")
	}
	if footer.LocaleControl.Placement == "" {
		// Configurations saved before locale placement existed displayed the
		// language control in the Header. Preserve that behavior on upgrade.
		footer.LocaleControl.Enabled = true
		footer.LocaleControl.Placement = "header"
	}
	if !oneOf(footer.LocaleControl.Placement, "footer", "header", "both") {
		return errors.New("footer locale placement is invalid")
	}
	if len(footer.Sections) > MaxFooterSections {
		return fmt.Errorf("at most %d footer sections are allowed", MaxFooterSections)
	}
	contactIDs := make(map[string]struct{}, len(organization.Contacts))
	for _, contact := range organization.Contacts {
		contactIDs[contact.ID] = struct{}{}
	}
	for _, id := range footer.BrandBlock.ContactIDs {
		if _, ok := contactIDs[id]; !ok {
			return fmt.Errorf("footer contact %q does not exist", id)
		}
	}
	sectionIDs := make(map[string]struct{}, len(footer.Sections))
	for sectionIndex := range footer.Sections {
		section := &footer.Sections[sectionIndex]
		section.ID = strings.TrimSpace(section.ID)
		section.Heading = strings.TrimSpace(section.Heading)
		if !validBrandKey(section.ID) || section.Heading == "" || len(section.Heading) > 120 {
			return errors.New("footer section is invalid")
		}
		if _, exists := sectionIDs[section.ID]; exists {
			return fmt.Errorf("duplicate footer section %q", section.ID)
		}
		sectionIDs[section.ID] = struct{}{}
		if len(section.Items) > MaxFooterItemsPerSection {
			return fmt.Errorf("at most %d items are allowed in one footer section", MaxFooterItemsPerSection)
		}
		itemIDs := make(map[string]struct{}, len(section.Items))
		for itemIndex := range section.Items {
			item := &section.Items[itemIndex]
			item.ID = strings.TrimSpace(item.ID)
			item.Kind = strings.TrimSpace(item.Kind)
			item.Label = strings.TrimSpace(item.Label)
			item.URL = strings.TrimSpace(item.URL)
			item.Text = strings.TrimSpace(item.Text)
			item.ContactID = strings.TrimSpace(item.ContactID)
			item.SystemAction = strings.TrimSpace(item.SystemAction)
			if !validBrandKey(item.ID) || !validFooterItem(*item, contactIDs) {
				return errors.New("footer item is invalid")
			}
			if _, exists := itemIDs[item.ID]; exists {
				return fmt.Errorf("duplicate footer item %q", item.ID)
			}
			itemIDs[item.ID] = struct{}{}
		}
		sort.SliceStable(section.Items, func(i, j int) bool {
			if section.Items[i].SortOrder == section.Items[j].SortOrder {
				return section.Items[i].ID < section.Items[j].ID
			}
			return section.Items[i].SortOrder < section.Items[j].SortOrder
		})
	}
	for index := range footer.LegalLinks {
		link := &footer.LegalLinks[index]
		link.ID = strings.TrimSpace(link.ID)
		link.Label = strings.TrimSpace(link.Label)
		link.URL = strings.TrimSpace(link.URL)
		if !validBrandKey(link.ID) || link.Label == "" || !validNavigationURL(link.URL) {
			return errors.New("footer legal link is invalid")
		}
	}
	footer.CopyrightText = strings.TrimSpace(footer.CopyrightText)
	footer.Disclaimer = strings.TrimSpace(footer.Disclaimer)
	if len(footer.CopyrightText) > 500 || len(footer.Disclaimer) > 2000 {
		return errors.New("footer text is too long")
	}
	for index := range footer.RegistrationLines {
		footer.RegistrationLines[index] = strings.TrimSpace(footer.RegistrationLines[index])
		if len(footer.RegistrationLines[index]) > 300 {
			return errors.New("footer registration line is too long")
		}
	}
	sort.SliceStable(footer.Sections, func(i, j int) bool {
		if footer.Sections[i].SortOrder == footer.Sections[j].SortOrder {
			return footer.Sections[i].ID < footer.Sections[j].ID
		}
		return footer.Sections[i].SortOrder < footer.Sections[j].SortOrder
	})
	return nil
}

func (configuration Configuration) ShowHeaderLocale() bool {
	control := configuration.Footer.LocaleControl
	return control.Enabled && (control.Placement == "header" || control.Placement == "both")
}

func (configuration Configuration) ShowFooterLocale() bool {
	control := configuration.Footer.LocaleControl
	return control.Enabled && (control.Placement == "footer" || control.Placement == "both")
}

func validFooterItem(item FooterItem, contacts map[string]struct{}) bool {
	switch item.Kind {
	case "link":
		return item.Label != "" && len(item.Label) <= 120 && validNavigationURL(item.URL)
	case "text":
		return item.Text != "" && len(item.Text) <= 500
	case "contact_ref":
		_, ok := contacts[item.ContactID]
		return ok
	case "system_action":
		return validSystemAction(item.SystemAction)
	default:
		return false
	}
}

func validNavigationItem(item *NavigationItem) bool {
	if item.TargetType == "" {
		if item.URL == "" {
			item.TargetType = "group"
		} else {
			item.TargetType = "link"
		}
	}
	if item.Presentation == "" {
		item.Presentation = "direct"
	}
	if !oneOf(item.Presentation, "direct", "dropdown") {
		return false
	}
	switch item.TargetType {
	case "group":
		return item.URL == "" && item.SystemAction == ""
	case "link":
		return validNavigationURL(item.URL) && item.SystemAction == ""
	case "system_action":
		if !validSystemAction(item.SystemAction) || item.SystemAction == "locale" {
			return false
		}
		item.URL = systemActionURL(item.SystemAction)
		return true
	default:
		return false
	}
}

func systemActionURL(action string) string {
	switch action {
	case "catalog":
		return "/catalog"
	case "catalog_search":
		return "/search"
	case "rfq":
		return "/rfq"
	default:
		return ""
	}
}

func validSystemAction(action string) bool { _, ok := allowedSystemActions[action]; return ok }
func validBrandKey(value string) bool      { return brandKeyPattern.MatchString(value) }
func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
