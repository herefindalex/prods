package publishing

import (
	"fmt"
	"strings"

	"prods/internal/catalog"
	"prods/internal/localization"
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

type ApplicableSpecification struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PreferredUnit string `json:"preferred_unit,omitempty"`
}

type PublicView struct {
	PackageFormFactor        string                             `json:"package_form_factor,omitempty"`
	ApplicableSpecifications []ApplicableSpecification          `json:"applicable_specifications,omitempty"`
	ID                       string                             `json:"id"`
	Revision                 int64                              `json:"revision"`
	SiteEpoch                int64                              `json:"site_epoch"`
	PartNumber               string                             `json:"part_number"`
	Name                     string                             `json:"name,omitempty"`
	Manufacturer             string                             `json:"manufacturer"`
	ManufacturerID           string                             `json:"manufacturer_id,omitempty"`
	ManufacturerURL          string                             `json:"manufacturer_url,omitempty"`
	Brand                    string                             `json:"brand,omitempty"`
	BrandID                  string                             `json:"brand_id,omitempty"`
	BrandURL                 string                             `json:"brand_url,omitempty"`
	Category                 string                             `json:"category,omitempty"`
	CategoryID               string                             `json:"category_id,omitempty"`
	CategoryURL              string                             `json:"category_url,omitempty"`
	CategoryTrail            []CategoryRef                      `json:"category_trail,omitempty"`
	Lifecycle                string                             `json:"lifecycle,omitempty"`
	LifecycleID              string                             `json:"lifecycle_id,omitempty"`
	Applications             []Application                      `json:"applications,omitempty"`
	Images                   []Image                            `json:"images,omitempty"`
	Description              string                             `json:"description,omitempty"`
	Features                 string                             `json:"features,omitempty"`
	Specification            string                             `json:"specification,omitempty"`
	Specifications           []Specification                    `json:"specifications,omitempty"`
	Documents                []Document                         `json:"documents,omitempty"`
	CanonicalURL             string                             `json:"canonical_url"`
	Language                 string                             `json:"language"`
	DefaultLocale            string                             `json:"default_locale,omitempty"`
	SupportedLocales         []string                           `json:"supported_locales,omitempty"`
	FieldLocalization        map[string]FieldLocalization       `json:"field_localization,omitempty"`
	PublicCopy               map[string]string                  `json:"public_copy,omitempty"`
	RFQURL                   string                             `json:"rfq_url"`
	Site                     site.Configuration                 `json:"site"`
	Localizations            map[string]LocalizedContent        `json:"-"`
	SourceLocale             string                             `json:"-"`
	SourceLocales            map[string]string                  `json:"-"`
	LabelLocalizations       map[string]LocalizedLabel          `json:"-"`
	PublicCopyDefaults       map[string]map[string]string       `json:"-"`
	PublicCopyOverrides      localization.PublicCopyOverrideMap `json:"-"`
}

type FieldLocalization struct {
	RequestedLocale string                  `json:"requested_locale"`
	EffectiveLocale string                  `json:"effective_locale,omitempty"`
	Provenance      localization.Provenance `json:"provenance"`
}

type LocalizedContent struct {
	Name          string
	Description   string
	Features      string
	Specification string
}

type LocalizedLabel struct {
	SourceLocale string
	Values       map[string]string
}

func (v PublicView) ForLocale(locale string) PublicView {
	defaultLocale := v.DefaultLocale
	if defaultLocale == "" && len(v.SupportedLocales) > 0 {
		defaultLocale = v.SupportedLocales[0]
	}
	if defaultLocale == "" {
		defaultLocale = v.SourceLocale
	}
	v.Language = locale
	v.FieldLocalization = make(map[string]FieldLocalization, len(catalog.ProductTranslatableFields))
	fields := map[string]*string{
		"name": &v.Name, "description": &v.Description, "features": &v.Features, "specification": &v.Specification,
	}
	for field, target := range fields {
		values := make(map[string]string, len(v.Localizations)+1)
		sourceLocale := v.SourceLocales[field]
		if sourceLocale == "" {
			sourceLocale = v.SourceLocale
		}
		if sourceLocale == "" {
			sourceLocale = defaultLocale
		}
		values[sourceLocale] = *target
		for translationLocale, item := range v.Localizations {
			switch field {
			case "name":
				values[translationLocale] = item.Name
			case "description":
				values[translationLocale] = item.Description
			case "features":
				values[translationLocale] = item.Features
			case "specification":
				values[translationLocale] = item.Specification
			}
		}
		resolved := localization.ResolveCustomerValue(locale, defaultLocale, sourceLocale, v.SupportedLocales, values)
		*target = resolved.Value
		v.FieldLocalization[field] = FieldLocalization{
			RequestedLocale: resolved.RequestedLocale,
			EffectiveLocale: resolved.EffectiveLocale,
			Provenance:      resolved.Provenance,
		}
	}
	resolveLabel := func(key string, target *string) {
		label, ok := v.LabelLocalizations[key]
		if !ok || target == nil {
			return
		}
		resolved := localization.ResolveCustomerValue(locale, defaultLocale, label.SourceLocale, v.SupportedLocales, label.Values)
		*target = resolved.Value
		v.FieldLocalization[key] = FieldLocalization{RequestedLocale: resolved.RequestedLocale, EffectiveLocale: resolved.EffectiveLocale, Provenance: resolved.Provenance}
	}
	resolveLabel("manufacturer.name", &v.Manufacturer)
	resolveLabel("brand.name", &v.Brand)
	resolveLabel("category.name", &v.Category)
	resolveLabel("lifecycle.name", &v.Lifecycle)
	for index := range v.CategoryTrail {
		resolveLabel("category."+v.CategoryTrail[index].ID+".name", &v.CategoryTrail[index].Name)
	}
	for index := range v.Applications {
		resolveLabel("application."+v.Applications[index].ID+".name", &v.Applications[index].Name)
	}
	v.PublicCopy = make(map[string]string, len(v.PublicCopyDefaults))
	for key, byLocale := range v.PublicCopyDefaults {
		defaults := make([]localization.PublicCopyDefault, 0, len(byLocale))
		for itemLocale, value := range byLocale {
			defaults = append(defaults, localization.PublicCopyDefault{Key: key, Locale: itemLocale, Value: value, DefinitionVersion: 1, OfficialBundle: localization.OfficialBundleVersion})
		}
		resolved := localization.ResolvePublicCopy(key, locale, defaultLocale, v.SupportedLocales, v.PublicCopyOverrides, defaults)
		if resolved.Value != "" {
			v.PublicCopy[key] = resolved.Value
			v.FieldLocalization["public_copy."+key] = FieldLocalization{RequestedLocale: resolved.RequestedLocale, EffectiveLocale: resolved.EffectiveLocale, Provenance: resolved.Provenance}
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
		CanonicalURL:  canonical, Language: "en-US", DefaultLocale: "en-US", SupportedLocales: []string{"en-US"},
		SourceLocale: "en-US", SourceLocales: map[string]string{
			"name": "en-US", "description": "en-US", "features": "en-US", "specification": "en-US",
		},
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
