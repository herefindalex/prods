package publishing

import (
	"fmt"
	"strings"

	"prods/internal/catalog"
	"prods/internal/site"
)

type Document struct {
	ID       string `json:"id,omitempty"`
	Label    string `json:"label"`
	Type     string `json:"type,omitempty"`
	AssetID  string `json:"asset_id,omitempty"`
	URL      string `json:"url"`
	Language string `json:"language,omitempty"`
}

type Specification struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RawValue      string `json:"raw_value"`
	PreferredUnit string `json:"preferred_unit,omitempty"`
	Language      string `json:"language,omitempty"`
}

type Application struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Image struct {
	ID      string `json:"id"`
	AssetID string `json:"asset_id,omitempty"`
	URL     string `json:"url"`
	AltText string `json:"alt_text"`
	Primary bool   `json:"primary"`
}

type PublicView struct {
	ID               string                      `json:"id"`
	Revision         int64                       `json:"revision"`
	SiteEpoch        int64                       `json:"site_epoch"`
	PartNumber       string                      `json:"part_number"`
	Name             string                      `json:"name,omitempty"`
	Manufacturer     string                      `json:"manufacturer"`
	ManufacturerID   string                      `json:"manufacturer_id,omitempty"`
	ManufacturerURL  string                      `json:"manufacturer_url,omitempty"`
	Brand            string                      `json:"brand,omitempty"`
	BrandID          string                      `json:"brand_id,omitempty"`
	BrandURL         string                      `json:"brand_url,omitempty"`
	Category         string                      `json:"category,omitempty"`
	CategoryID       string                      `json:"category_id,omitempty"`
	CategoryURL      string                      `json:"category_url,omitempty"`
	CategoryTrail    []CategoryRef               `json:"category_trail,omitempty"`
	Lifecycle        string                      `json:"lifecycle,omitempty"`
	LifecycleID      string                      `json:"lifecycle_id,omitempty"`
	Applications     []Application               `json:"applications,omitempty"`
	Images           []Image                     `json:"images,omitempty"`
	Description      string                      `json:"description,omitempty"`
	Features         string                      `json:"features,omitempty"`
	Specification    string                      `json:"specification,omitempty"`
	Specifications   []Specification             `json:"specifications,omitempty"`
	Documents        []Document                  `json:"documents,omitempty"`
	CanonicalURL     string                      `json:"canonical_url"`
	Language         string                      `json:"language"`
	SupportedLocales []string                    `json:"supported_locales,omitempty"`
	RFQURL           string                      `json:"rfq_url"`
	Site             site.Configuration          `json:"site"`
	Localizations    map[string]LocalizedContent `json:"-"`
}

type LocalizedContent struct {
	Name          string
	Description   string
	Features      string
	Specification string
}

func (v PublicView) ForLocale(locale string) PublicView {
	v.Language = locale
	if item, ok := v.Localizations[locale]; ok {
		if item.Name != "" {
			v.Name = item.Name
		}
		if item.Description != "" {
			v.Description = item.Description
		}
		if item.Features != "" {
			v.Features = item.Features
		}
		if item.Specification != "" {
			v.Specification = item.Specification
		}
	}
	return v
}

type CategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func FromProduct(product catalog.Product, baseURL string) (PublicView, error) {
	if product.Status != catalog.Published {
		return PublicView{}, fmt.Errorf("product is not public")
	}
	canonical := strings.TrimRight(baseURL, "/") + "/products/" + product.ID
	view := PublicView{
		Site: site.DefaultConfiguration(),
		ID:   product.ID, Revision: product.Revision, PartNumber: product.PartNumber,
		Name: product.Name, Manufacturer: product.Manufacturer,
		Brand: product.Brand, Lifecycle: product.Lifecycle, LifecycleID: product.LifecycleID,
		Description: product.Description, Features: product.Features,
		Specification: product.Specification,
		CanonicalURL:  canonical, Language: "en-US", SupportedLocales: []string{"en-US"},
		RFQURL: "/rfq?product_id=" + product.ID,
	}
	if product.DocumentURL != "" {
		view.Documents = []Document{{Label: "Datasheet", URL: product.DocumentURL}}
	}
	for _, application := range product.Applications {
		view.Applications = append(view.Applications, Application{
			ID: application.ID, Name: application.Name,
			URL: strings.TrimRight(baseURL, "/") + "/applications/" + application.Slug,
		})
	}
	return view, nil
}
