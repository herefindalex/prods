package publishing

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

func JSON(view PublicView) ([]byte, error) {
	return json.MarshalIndent(view, "", "  ")
}

func JSONLD(view PublicView) ([]byte, error) {
	documents := make([]string, 0, len(view.Documents))
	for _, document := range view.Documents {
		documents = append(documents, document.URL)
	}
	value := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Product",
		"@id":         view.CanonicalURL + "#product",
		"url":         view.CanonicalURL,
		"mpn":         view.PartNumber,
		"name":        view.Name,
		"description": view.Description,
		"subjectOf":   documents,
	}
	if view.Manufacturer != "" {
		value["manufacturer"] = map[string]string{"@type": "Organization", "name": view.Manufacturer}
	}
	if view.Brand != "" {
		value["brand"] = map[string]string{"@type": "Brand", "name": view.Brand}
	}
	if len(view.Images) > 0 {
		images := make([]string, 0, len(view.Images))
		for _, image := range view.Images {
			images = append(images, image.URL)
		}
		value["image"] = images
	}
	if len(view.Applications) > 0 {
		applications := make([]string, 0, len(view.Applications))
		for _, application := range view.Applications {
			applications = append(applications, application.Name)
		}
		value["category"] = applications
	}
	if view.Site.Organization.DisplayName != "" {
		organization := map[string]string{
			"@type": "Organization",
			"name":  view.Site.Organization.DisplayName,
		}
		if view.Site.Organization.LegalName != "" {
			organization["legalName"] = view.Site.Organization.LegalName
		}
		if view.Site.Organization.OfficialWebsite != "" {
			organization["url"] = view.Site.Organization.OfficialWebsite
		}
		value["provider"] = organization
	}
	return json.Marshal(value)
}

func Markdown(view PublicView) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", view.PartNumber)
	if view.Name != "" {
		fmt.Fprintf(&out, "%s\n\n", view.Name)
	}
	fmt.Fprintf(&out, "- Product ID: `%s`\n- Public revision: `%d`\n- Manufacturer: %s\n- Canonical URL: %s\n",
		view.ID, view.Revision, view.Manufacturer, view.CanonicalURL)
	if view.Description != "" {
		fmt.Fprintf(&out, "\n## Description\n\n%s\n", view.Description)
	}
	if view.Features != "" {
		fmt.Fprintf(&out, "\n## Features\n\n%s\n", view.Features)
	}
	if len(view.Images) > 0 {
		out.WriteString("\n## Images\n\n")
		for _, image := range view.Images {
			fmt.Fprintf(&out, "- [%s](%s)\n", image.AltText, image.URL)
		}
	}
	if len(view.Applications) > 0 {
		out.WriteString("\n## Applications\n\n")
		for _, application := range view.Applications {
			fmt.Fprintf(&out, "- [%s](%s)\n", application.Name, application.URL)
		}
	}
	if view.Lifecycle != "" {
		fmt.Fprintf(&out, "\n## Lifecycle\n\n%s\n", view.Lifecycle)
	}
	if len(view.Specifications) > 0 {
		out.WriteString("\n## Specifications\n\n")
		for _, spec := range view.Specifications {
			fmt.Fprintf(&out, "- %s: %s", spec.Name, spec.RawValue)
			if spec.PreferredUnit != "" {
				fmt.Fprintf(&out, " %s", spec.PreferredUnit)
			}
			out.WriteByte('\n')
		}
	}
	if view.Specification != "" {
		fmt.Fprintf(&out, "\n## Specifications\n\n- Operating range: %s\n", view.Specification)
	}
	if len(view.Documents) > 0 {
		out.WriteString("\n## Documents\n\n")
		for _, document := range view.Documents {
			fmt.Fprintf(&out, "- [%s](%s)\n", document.Label, document.URL)
		}
	}
	fmt.Fprintf(&out, "\n[Request a quote](%s)\n", view.RFQURL)
	return []byte(out.String())
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	XMLNS   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Location string `xml:"loc"`
}

func Sitemap(views []PublicView) ([]byte, error) {
	set := sitemapURLSet{XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	locations := make(map[string]struct{})
	for _, view := range views {
		for _, location := range []string{view.CanonicalURL, view.CategoryURL, view.ManufacturerURL, view.BrandURL} {
			if location != "" {
				locations[location] = struct{}{}
			}
		}
		for _, application := range view.Applications {
			if application.URL != "" {
				locations[application.URL] = struct{}{}
			}
		}
	}
	ordered := make([]string, 0, len(locations))
	for location := range locations {
		ordered = append(ordered, location)
	}
	sort.Strings(ordered)
	for _, location := range ordered {
		set.URLs = append(set.URLs, sitemapURL{Location: location})
	}
	body, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}
