package brandcapture

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"prods/internal/site"
)

var nonIDCharacter = regexp.MustCompile(`[^a-z0-9]+`)

type FieldChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func BuildProposal(current site.Configuration, candidate Candidate) (site.Configuration, []FieldChange, error) {
	proposed := cloneConfiguration(current)
	if candidate.Organization != "" {
		proposed.Organization.DisplayName = candidate.Organization
	}
	if official := sourceOrigin(candidate.SourceURL); official != "" {
		proposed.Organization.OfficialWebsite = official
	}
	if len(candidate.Colors) > 0 {
		proposed.Theme.PrimaryColor = candidate.Colors[0]
	}
	if len(candidate.Colors) > 1 {
		proposed.Theme.SecondaryColor = candidate.Colors[1]
	}
	if len(candidate.Fonts) > 0 {
		proposed.Theme.FontFamily = candidate.Fonts[0]
	}
	if len(candidate.Navigation) > 0 {
		proposed.Navigation = proposedNavigation(current.Navigation, candidate.Navigation)
	}
	ensureCoreNavigation(&proposed.Navigation)
	if err := proposed.Prepare(); err != nil {
		return site.Configuration{}, nil, fmt.Errorf("captured proposal is invalid: %w", err)
	}
	return proposed, configurationDiff(current, proposed), nil
}

func cloneConfiguration(configuration site.Configuration) site.Configuration {
	cloned := configuration
	cloned.Organization.ContactLinks = append([]site.ContactLink(nil), configuration.Organization.ContactLinks...)
	cloned.Navigation = append([]site.NavigationItem(nil), configuration.Navigation...)
	if configuration.CategoryListingProfiles != nil {
		cloned.CategoryListingProfiles = make(map[string]site.CategoryListingProfile, len(configuration.CategoryListingProfiles))
		for categoryID, profile := range configuration.CategoryListingProfiles {
			profile.VisibleColumns = append([]string(nil), profile.VisibleColumns...)
			profile.MobileKeySpecs = append([]string(nil), profile.MobileKeySpecs...)
			cloned.CategoryListingProfiles[categoryID] = profile
		}
	}
	return cloned
}

func proposedNavigation(current []site.NavigationItem, captured []Navigation) []site.NavigationItem {
	result := make([]site.NavigationItem, 0, len(captured)+2)
	usedIDs := make(map[string]struct{}, len(captured)+2)
	coreTargets := make(map[string]struct{}, 2)
	for index, item := range captured {
		target := mapCatalogTarget(item.Label, item.URL)
		if target == "" {
			continue
		}
		if target == "/catalog" || target == "/rfq" {
			if _, exists := coreTargets[target]; exists {
				continue
			}
			coreTargets[target] = struct{}{}
		}
		id := existingNavigationID(current, item.Label, target)
		if id == "" {
			id = navigationID(item.Label, index+1)
		}
		id = uniqueNavigationID(id, usedIDs)
		usedIDs[id] = struct{}{}
		result = append(result, site.NavigationItem{
			ID: id, Label: strings.TrimSpace(item.Label), URL: target,
			SortOrder: (index + 1) * 10, Visible: true,
		})
	}
	return result
}

func ensureCoreNavigation(navigation *[]site.NavigationItem) {
	core := []struct {
		id, label, target string
	}{
		{"catalog", "Catalog", "/catalog"},
		{"rfq", "Request quote", "/rfq"},
	}
	usedIDs := make(map[string]struct{}, len(*navigation)+2)
	for _, item := range *navigation {
		usedIDs[item.ID] = struct{}{}
	}
	for _, required := range core {
		found := false
		for index := range *navigation {
			if (*navigation)[index].URL == required.target {
				(*navigation)[index].Visible = true
				found = true
				break
			}
		}
		if found {
			continue
		}
		id := uniqueNavigationID(required.id, usedIDs)
		usedIDs[id] = struct{}{}
		*navigation = append(*navigation, site.NavigationItem{
			ID: id, Label: required.label, URL: required.target,
			SortOrder: (len(*navigation) + 1) * 10, Visible: true,
		})
	}
}

func mapCatalogTarget(label, target string) string {
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	lowerLabel := strings.ToLower(strings.TrimSpace(label))
	lowerPath := strings.ToLower(parsed.Path)
	if strings.Contains(lowerLabel, "product") || strings.Contains(lowerLabel, "catalog") ||
		strings.HasPrefix(lowerPath, "/products") || strings.HasPrefix(lowerPath, "/catalog") {
		return "/catalog"
	}
	return parsed.String()
}

func sourceOrigin(raw string) string {
	parsed, err := ParsePublicURL(raw)
	if err != nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func existingNavigationID(current []site.NavigationItem, label, target string) string {
	for _, item := range current {
		if item.URL == target && strings.EqualFold(item.Label, strings.TrimSpace(label)) {
			return item.ID
		}
	}
	return ""
}

func navigationID(label string, fallback int) string {
	id := strings.Trim(nonIDCharacter.ReplaceAllString(strings.ToLower(strings.TrimSpace(label)), "-"), "-")
	if id == "" {
		return fmt.Sprintf("capture-%02d", fallback)
	}
	return "capture-" + id
}

func uniqueNavigationID(candidate string, used map[string]struct{}) string {
	if _, exists := used[candidate]; !exists {
		return candidate
	}
	for suffix := 2; ; suffix++ {
		value := fmt.Sprintf("%s-%d", candidate, suffix)
		if _, exists := used[value]; !exists {
			return value
		}
	}
}

func configurationDiff(before, after site.Configuration) []FieldChange {
	changes := make([]FieldChange, 0, 6)
	appendChange := func(field string, oldValue, newValue any) {
		oldJSON, _ := json.Marshal(oldValue)
		newJSON, _ := json.Marshal(newValue)
		if string(oldJSON) != string(newJSON) {
			changes = append(changes, FieldChange{Field: field, Before: string(oldJSON), After: string(newJSON)})
		}
	}
	appendChange("organization.display_name", before.Organization.DisplayName, after.Organization.DisplayName)
	appendChange("organization.official_website", before.Organization.OfficialWebsite, after.Organization.OfficialWebsite)
	appendChange("theme.primary_color", before.Theme.PrimaryColor, after.Theme.PrimaryColor)
	appendChange("theme.secondary_color", before.Theme.SecondaryColor, after.Theme.SecondaryColor)
	appendChange("theme.font_family", before.Theme.FontFamily, after.Theme.FontFamily)
	appendChange("navigation", before.Navigation, after.Navigation)
	return changes
}
