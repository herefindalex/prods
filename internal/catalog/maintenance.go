package catalog

import (
	"errors"
	"net/url"
	"strings"
)

var (
	ErrInvalidCategory     = errors.New("invalid category")
	ErrCategoryCycle       = errors.New("category move would create a cycle")
	ErrSystemCategory      = errors.New("system category is protected")
	ErrInvalidDictionary   = errors.New("invalid dictionary entry")
	ErrDisabledReference   = errors.New("disabled dictionary value cannot be selected")
	ErrInvalidSpec         = errors.New("invalid specification")
	ErrManualNormalization = errors.New("automatic normalization cannot overwrite a manual result")
	ErrInvalidDocument     = errors.New("invalid product document")
)

type EntryStatus string

const (
	EntryActive   EntryStatus = "active"
	EntryDisabled EntryStatus = "disabled"
)

type DictionaryKind string

const (
	DictionaryManufacturer DictionaryKind = "manufacturer"
	DictionaryBrand        DictionaryKind = "brand"
	DictionaryApplication  DictionaryKind = "application"
	DictionaryLifecycle    DictionaryKind = "lifecycle"
	DictionaryDocumentType DictionaryKind = "document_type"
)

type Category struct {
	ID        string      `json:"id"`
	ParentID  string      `json:"parent_id,omitempty"`
	SystemKey string      `json:"system_key,omitempty"`
	Name      string      `json:"name"`
	Slug      string      `json:"slug"`
	Status    EntryStatus `json:"status"`
	Revision  int64       `json:"revision"`
}

func (c *Category) Prepare() error {
	c.ID = strings.TrimSpace(c.ID)
	c.ParentID = strings.TrimSpace(c.ParentID)
	c.SystemKey = strings.TrimSpace(c.SystemKey)
	c.Name = strings.TrimSpace(c.Name)
	c.Slug = strings.TrimSpace(c.Slug)
	if c.ID == "" || c.Name == "" || c.Slug == "" || !ValidSlug(c.Slug) {
		return ErrInvalidCategory
	}
	if c.Status == "" {
		c.Status = EntryActive
	}
	if c.Status != EntryActive && c.Status != EntryDisabled {
		return ErrInvalidCategory
	}
	if c.Revision < 1 {
		c.Revision = 1
	}
	return nil
}

type DictionaryEntry struct {
	ID       string         `json:"id"`
	Kind     DictionaryKind `json:"kind"`
	Name     string         `json:"name"`
	Slug     string         `json:"slug,omitempty"`
	Status   EntryStatus    `json:"status"`
	Revision int64          `json:"revision"`
}

// TaxonomyProductImpact describes a Product whose public representation may
// change when a Category or dictionary entry changes. Routes are included so
// callers can present URL impact before accepting the mutation.
type TaxonomyProductImpact struct {
	ProductID     string `json:"product_id"`
	PartNumber    string `json:"part_number"`
	CurrentRoute  string `json:"current_route,omitempty"`
	ProposedRoute string `json:"proposed_route,omitempty"`
}

type TaxonomyImpact struct {
	EntityType       string                  `json:"entity_type"`
	EntityID         string                  `json:"entity_id"`
	AffectedProducts []TaxonomyProductImpact `json:"affected_products"`
}

func (e *DictionaryEntry) Prepare() error {
	e.ID = strings.TrimSpace(e.ID)
	e.Name = strings.TrimSpace(e.Name)
	e.Slug = strings.TrimSpace(e.Slug)
	if e.ID == "" || e.Name == "" || !validDictionaryKind(e.Kind) {
		return ErrInvalidDictionary
	}
	if e.Status == "" {
		e.Status = EntryActive
	}
	if e.Status != EntryActive && e.Status != EntryDisabled {
		return ErrInvalidDictionary
	}
	if e.Revision < 1 {
		e.Revision = 1
	}
	return nil
}

func validDictionaryKind(kind DictionaryKind) bool {
	switch kind {
	case DictionaryManufacturer, DictionaryBrand, DictionaryApplication, DictionaryLifecycle, DictionaryDocumentType:
		return true
	default:
		return false
	}
}

type SpecDefinition struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	PreferredUnit string      `json:"preferred_unit,omitempty"`
	Filterable    bool        `json:"filterable"`
	SemanticVer   int64       `json:"semantic_version"`
	Status        EntryStatus `json:"status"`
	Revision      int64       `json:"revision"`
}

type SpecSet struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Status   EntryStatus `json:"status"`
	Revision int64       `json:"revision"`
	SpecIDs  []string    `json:"spec_ids"`
}

type SpecValue struct {
	ID             string `json:"id"`
	ProductID      string `json:"product_id"`
	SpecID         string `json:"spec_id"`
	RawValue       string `json:"raw_value"`
	SourceLocale   string `json:"source_locale,omitempty"`
	SourceRevision int64  `json:"source_revision"`
	Active         bool   `json:"active"`
}

type NormalizedSource string

const (
	NormalizedAutomatic NormalizedSource = "automatic"
	NormalizedManual    NormalizedSource = "manual"
)

type NormalizedStatus string

const (
	NormalizedCurrent NormalizedStatus = "current"
	NormalizedStale   NormalizedStatus = "stale"
	NormalizedFailed  NormalizedStatus = "failed"
)

type NormalizedValue struct {
	ID                  string           `json:"id"`
	SpecValueID         string           `json:"spec_value_id"`
	ValueJSON           string           `json:"value_json,omitempty"`
	SourceRevision      int64            `json:"source_revision"`
	SpecSemanticVersion int64            `json:"spec_semantic_version"`
	NormalizerVersion   string           `json:"normalizer_version"`
	Source              NormalizedSource `json:"source"`
	Status              NormalizedStatus `json:"status"`
	FailureReason       string           `json:"failure_reason,omitempty"`
}

// SpecValueDetail is the Admin read model for an authoritative raw value and
// its derived normalization history. Public rendering does not consume it.
type SpecValueDetail struct {
	Value      SpecValue         `json:"value"`
	Normalized []NormalizedValue `json:"normalized"`
}

type ProductDocument struct {
	ID             string `json:"id"`
	ProductID      string `json:"product_id"`
	Label          string `json:"label"`
	DocumentTypeID string `json:"document_type_id"`
	AssetID        string `json:"asset_id,omitempty"`
	ExternalURL    string `json:"external_url,omitempty"`
	Language       string `json:"language,omitempty"`
	SortOrder      int    `json:"sort_order"`
}

type ProductImage struct {
	ID          string `json:"id"`
	ProductID   string `json:"product_id"`
	AssetID     string `json:"asset_id,omitempty"`
	ExternalURL string `json:"external_url,omitempty"`
	AltText     string `json:"alt_text,omitempty"`
	SortOrder   int    `json:"sort_order"`
	Primary     bool   `json:"primary"`
}

func (i *ProductImage) Prepare() error {
	i.ID = strings.TrimSpace(i.ID)
	i.ProductID = strings.TrimSpace(i.ProductID)
	i.AssetID = strings.TrimSpace(i.AssetID)
	i.ExternalURL = strings.TrimSpace(i.ExternalURL)
	i.AltText = strings.TrimSpace(i.AltText)
	if i.ID == "" || i.ProductID == "" || i.SortOrder < 0 || (i.AssetID == "") == (i.ExternalURL == "") {
		return ErrInvalidAsset
	}
	if i.ExternalURL != "" {
		location, err := url.Parse(i.ExternalURL)
		if err != nil || location.Host == "" || (location.Scheme != "http" && location.Scheme != "https") {
			return ErrInvalidAsset
		}
	}
	return nil
}

func (d *ProductDocument) Prepare() error {
	d.ID = strings.TrimSpace(d.ID)
	d.ProductID = strings.TrimSpace(d.ProductID)
	d.Label = strings.TrimSpace(d.Label)
	d.DocumentTypeID = strings.TrimSpace(d.DocumentTypeID)
	d.AssetID = strings.TrimSpace(d.AssetID)
	d.ExternalURL = strings.TrimSpace(d.ExternalURL)
	d.Language = strings.TrimSpace(d.Language)
	if d.ID == "" || d.ProductID == "" || d.Label == "" || d.DocumentTypeID == "" || (d.AssetID == "") == (d.ExternalURL == "") {
		return ErrInvalidDocument
	}
	if d.ExternalURL != "" {
		parsed, err := url.Parse(d.ExternalURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil {
			return ErrInvalidDocument
		}
	}
	return nil
}
