package catalog

import (
	"errors"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const SearchProjectionVersion = "x-text-v0.25.0-nfc-casefold-fields-v2"

var (
	ErrInvalidProduct   = errors.New("invalid product")
	ErrIdentityConflict = errors.New("current product identity conflicts with another product")
	ErrRevisionConflict = errors.New("product revision conflict")
	ErrArchivedProduct  = errors.New("archived product is read-only")
	ErrRouteConflict    = errors.New("public route conflicts with another product")
)

const UncategorizedCategoryID = "cat_uncategorized"

type RecordState string

const (
	RecordCurrent  RecordState = "current"
	RecordArchived RecordState = "archived"
)

type Status string

const (
	Published Status = "published"
	Hidden    Status = "hidden"
)

type Product struct {
	ID                string        `json:"id"`
	Slug              string        `json:"slug"`
	CustomPath        string        `json:"custom_path,omitempty"`
	PartNumber        string        `json:"part_number"`
	Name              string        `json:"name"`
	SourceLocale      string        `json:"source_locale,omitempty"`
	ManufacturerID    string        `json:"manufacturer_id,omitempty"`
	Manufacturer      string        `json:"manufacturer,omitempty"`
	BrandID           string        `json:"brand_id,omitempty"`
	Brand             string        `json:"brand,omitempty"`
	LifecycleID       string        `json:"lifecycle_id,omitempty"`
	Lifecycle         string        `json:"lifecycle,omitempty"`
	ApplicationIDs    []string      `json:"application_ids,omitempty"`
	Applications      []Application `json:"applications,omitempty"`
	CategoryID        string        `json:"category_id"`
	PackageFormFactor string        `json:"package_form_factor,omitempty"`
	Description       string        `json:"description"`
	Features          string        `json:"features,omitempty"`
	Specification     string        `json:"specification"`
	DocumentURL       string        `json:"document_url"`
	RecordState       RecordState   `json:"record_state"`
	Status            Status        `json:"status"`
	Revision          int64         `json:"revision"`
	ClonedFromID      string        `json:"cloned_from_id,omitempty"`
	CreatedBy         string        `json:"created_by,omitempty"`
	UpdatedBy         string        `json:"updated_by,omitempty"`
	SearchFolded      string        `json:"-"`
	ProjectionVer     string        `json:"-"`
	IdentityPart      string        `json:"-"`
	IdentityMaker     string        `json:"-"`
}

type Application struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type ProductTranslation struct {
	Locale        string `json:"locale"`
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Features      string `json:"features,omitempty"`
	Specification string `json:"specification,omitempty"`
	Revision      int64  `json:"revision"`
	UpdatedBy     string `json:"updated_by,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type ProductContent struct {
	ProductID       string               `json:"product_id"`
	ProductRevision int64                `json:"product_revision"`
	SourceLocale    string               `json:"source_locale"`
	Translations    []ProductTranslation `json:"translations"`
}

func (p *Product) Prepare() error {
	p.ID = strings.TrimSpace(p.ID)
	p.Slug = strings.TrimSpace(p.Slug)
	p.CustomPath = strings.TrimSpace(p.CustomPath)
	p.IdentityPart = strings.TrimSpace(p.PartNumber)
	p.Name = strings.TrimSpace(p.Name)
	p.SourceLocale = strings.TrimSpace(p.SourceLocale)
	p.ManufacturerID = strings.TrimSpace(p.ManufacturerID)
	p.BrandID = strings.TrimSpace(p.BrandID)
	p.Brand = strings.TrimSpace(p.Brand)
	p.LifecycleID = strings.TrimSpace(p.LifecycleID)
	p.Lifecycle = strings.TrimSpace(p.Lifecycle)
	p.ApplicationIDs = normalizedIDs(p.ApplicationIDs)
	for index := range p.Applications {
		p.Applications[index].ID = strings.TrimSpace(p.Applications[index].ID)
		p.Applications[index].Name = strings.TrimSpace(p.Applications[index].Name)
		p.Applications[index].Slug = strings.TrimSpace(p.Applications[index].Slug)
	}
	p.CategoryID = strings.TrimSpace(p.CategoryID)
	p.PackageFormFactor = strings.TrimSpace(p.PackageFormFactor)
	p.Description = strings.TrimSpace(p.Description)
	p.Features = strings.TrimSpace(p.Features)
	p.Specification = strings.TrimSpace(p.Specification)
	p.DocumentURL = strings.TrimSpace(p.DocumentURL)
	p.ClonedFromID = strings.TrimSpace(p.ClonedFromID)
	p.CreatedBy = strings.TrimSpace(p.CreatedBy)
	p.UpdatedBy = strings.TrimSpace(p.UpdatedBy)
	if p.ManufacturerID != "" {
		p.IdentityMaker = "id:" + p.ManufacturerID
	} else if maker := strings.TrimSpace(p.Manufacturer); maker != "" {
		p.IdentityMaker = "text:" + maker
	} else {
		p.IdentityMaker = ""
	}
	if p.ID == "" || p.IdentityPart == "" {
		return ErrInvalidProduct
	}
	if p.Slug == "" {
		p.Slug = SuggestedSlug(p.PartNumber, p.ID)
	}
	if !ValidSlug(p.Slug) {
		return ErrInvalidProduct
	}
	if p.CustomPath != "" && !ValidCustomPath(p.CustomPath) {
		return ErrInvalidProduct
	}
	if p.CategoryID == "" {
		p.CategoryID = UncategorizedCategoryID
	}
	if p.RecordState == "" {
		p.RecordState = RecordCurrent
	}
	if p.Status == "" {
		p.Status = Hidden
	}
	if p.RecordState != RecordCurrent && p.RecordState != RecordArchived {
		return ErrInvalidProduct
	}
	if p.Status != Published && p.Status != Hidden {
		return ErrInvalidProduct
	}
	if p.RecordState == RecordArchived && p.Status != Hidden {
		return ErrInvalidProduct
	}
	if p.Revision < 1 {
		p.Revision = 1
	}
	searchable := []string{p.PartNumber, p.Name, p.Manufacturer, p.Brand, p.Lifecycle}
	for _, application := range p.Applications {
		searchable = append(searchable, application.Name)
	}
	p.SearchFolded = FoldSearch(strings.Join(searchable, "\n"))
	p.ProjectionVer = SearchProjectionVersion
	return nil
}

func normalizedIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func ValidSlug(value string) bool {
	if value == "" || len(value) > 120 || value == "." || value == ".." {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func ValidCustomPath(value string) bool {
	if len(value) < 2 || len(value) > 240 || value[0] != '/' || strings.HasSuffix(value, "/") || strings.ContainsAny(value, `\?#%`) {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(value, "/"), "/")
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if !ValidSlug(segment) {
			return false
		}
	}
	reserved := map[string]struct{}{
		"admin": {}, "api": {}, "assets": {}, "static": {}, "health": {}, "search": {}, "rfq": {},
		"categories": {}, "manufacturers": {}, "brands": {}, "applications": {}, "set-password": {}, "sitemap.xml": {}, "site.css": {},
	}
	_, blocked := reserved[strings.ToLower(segments[0])]
	return !blocked
}

func SuggestedSlug(value, stableID string) string {
	value = strings.TrimSpace(value)
	var slug strings.Builder
	pendingSeparator := false
	for _, character := range value {
		switch {
		case (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '.':
			if pendingSeparator && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(character)
			pendingSeparator = false
		case character == '-' || character == ' ':
			pendingSeparator = slug.Len() > 0
		default:
			pendingSeparator = slug.Len() > 0
		}
		if slug.Len() >= 100 {
			break
		}
	}
	result := strings.Trim(slug.String(), "-.")
	if ValidSlug(result) {
		return result
	}
	fallback := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(stableID)), "prd_")
	if len(fallback) > 24 {
		fallback = fallback[:24]
	}
	if fallback == "" {
		fallback = "product"
	}
	return "p-" + fallback
}

func (p Product) DisplayName() string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return name
	}
	return strings.TrimSpace(p.PartNumber)
}

func FoldSearch(value string) string {
	return norm.NFC.String(cases.Fold().String(norm.NFC.String(value)))
}

func EscapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
