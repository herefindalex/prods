package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/mod/semver"
)

const (
	DefaultRepositoryURL = "https://github.com/herefindalex/prods"
	DefaultAPIBase       = "https://api.github.com"
	SampleAssetName      = "prods-sample-data-v1.json"
	SampleChecksumName   = SampleAssetName + ".sha256"
	maxSampleBytes       = 16 << 20
)

var ErrUnavailable = errors.New("distribution service is unavailable")

type Product struct {
	ID                string      `json:"id"`
	PartNumber        string      `json:"part_number"`
	Name              string      `json:"name,omitempty"`
	SourceLocale      string      `json:"source_locale,omitempty"`
	ManufacturerID    string      `json:"manufacturer_id,omitempty"`
	Manufacturer      string      `json:"manufacturer,omitempty"`
	BrandID           string      `json:"brand_id,omitempty"`
	Brand             string      `json:"brand,omitempty"`
	LifecycleID       string      `json:"lifecycle_id,omitempty"`
	ApplicationIDs    []string    `json:"application_ids,omitempty"`
	CategoryID        string      `json:"category_id,omitempty"`
	PackageFormFactor string      `json:"package_form_factor,omitempty"`
	Description       string      `json:"description,omitempty"`
	Features          string      `json:"features,omitempty"`
	Specification     string      `json:"specification,omitempty"`
	DocumentURL       string      `json:"document_url,omitempty"`
	Status            string      `json:"status,omitempty"`
	RecordState       string      `json:"record_state,omitempty"`
	SpecValues        []SpecValue `json:"spec_values,omitempty"`
}

type Category struct {
	ID           string `json:"id"`
	ParentID     string `json:"parent_id,omitempty"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	SourceLocale string `json:"source_locale,omitempty"`
	Slug         string `json:"slug"`
	Status       string `json:"status,omitempty"`
}

type DictionaryEntry struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	SourceLocale string `json:"source_locale,omitempty"`
	Slug         string `json:"slug,omitempty"`
	Status       string `json:"status,omitempty"`
}

type SpecDefinition struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	PreferredUnit   string `json:"preferred_unit,omitempty"`
	Filterable      bool   `json:"filterable"`
	SemanticVersion int64  `json:"semantic_version"`
	Status          string `json:"status,omitempty"`
}

type SpecSet struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Status  string   `json:"status,omitempty"`
	SpecIDs []string `json:"spec_ids"`
}

type CategorySpecSet struct {
	CategoryID string `json:"category_id"`
	SpecSetID  string `json:"spec_set_id"`
}

type SpecValue struct {
	SpecID       string `json:"spec_id"`
	RawValue     string `json:"raw_value"`
	SourceLocale string `json:"source_locale,omitempty"`
}

type SampleData struct {
	SchemaVersion    int               `json:"schema_version"`
	ReleaseVersion   string            `json:"release_version"`
	DatasetVersion   string            `json:"dataset_version"`
	Seed             int64             `json:"seed"`
	SourceStatement  string            `json:"source_statement"`
	Dictionaries     []DictionaryEntry `json:"dictionaries,omitempty"`
	Categories       []Category        `json:"categories"`
	Specs            []SpecDefinition  `json:"specs,omitempty"`
	SpecSets         []SpecSet         `json:"spec_sets,omitempty"`
	CategorySpecSets []CategorySpecSet `json:"category_spec_sets,omitempty"`
	Products         []Product         `json:"products"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type Release struct {
	TagName    string  `json:"tag_name"`
	HTMLURL    string  `json:"html_url"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

type UpdateStatus struct {
	Status         string `json:"status"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	ReleasePageURL string `json:"release_page_url,omitempty"`
	DownloadURL    string `json:"download_url,omitempty"`
	CheckedUTC     string `json:"checked_utc"`
}

type Client struct {
	HTTPClient    *http.Client
	APIBase       string
	RepositoryURL string
	CacheTTL      time.Duration
	AllowTestHTTP bool

	mu    sync.Mutex
	cache map[string]cachedRelease
}

type cachedRelease struct {
	release Release
	at      time.Time
	err     error
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 4 * time.Second}
	}
	return &Client{HTTPClient: httpClient, APIBase: DefaultAPIBase, RepositoryURL: DefaultRepositoryURL, CacheTTL: 10 * time.Minute, cache: make(map[string]cachedRelease)}
}

func (c *Client) LatestRelease(ctx context.Context) (Release, error) {
	return c.release(ctx, "latest", strings.TrimRight(c.apiBase(), "/")+"/repos/herefindalex/prods/releases/latest")
}

func (c *Client) release(ctx context.Context, cacheKey, endpoint string) (Release, error) {
	c.mu.Lock()
	if cached, ok := c.cache[cacheKey]; ok && time.Since(cached.at) < c.cacheTTL() {
		c.mu.Unlock()
		return cached.release, cached.err
	}
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Prods-update-checker")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return c.cacheFailure(cacheKey, fmt.Errorf("%w: %v", ErrUnavailable, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return c.cacheFailure(cacheKey, fmt.Errorf("%w: GitHub returned HTTP %d", ErrUnavailable, resp.StatusCode))
	}
	var release Release
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(&release); err != nil {
		return c.cacheFailure(cacheKey, fmt.Errorf("%w: decode release metadata: %v", ErrUnavailable, err))
	}
	if release.Draft || release.TagName == "" || !c.validRepositoryURL(release.HTMLURL) {
		return c.cacheFailure(cacheKey, fmt.Errorf("%w: invalid release metadata", ErrUnavailable))
	}
	for _, asset := range release.Assets {
		if asset.Name == "" || !c.validAssetURL(asset.BrowserDownloadURL) {
			return c.cacheFailure(cacheKey, fmt.Errorf("%w: invalid release asset URL", ErrUnavailable))
		}
	}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[string]cachedRelease)
	}
	c.cache[cacheKey] = cachedRelease{release: release, at: time.Now()}
	c.mu.Unlock()
	return release, nil
}

func validateSampleData(sample SampleData, version string) error {
	if sample.SchemaVersion != 1 || sample.ReleaseVersion != version ||
		strings.TrimSpace(sample.DatasetVersion) == "" || strings.TrimSpace(sample.SourceStatement) == "" || len(sample.Products) == 0 {
		return errors.New("sample data format is invalid")
	}
	dictionaryIDs := make(map[string]string, len(sample.Dictionaries))
	for _, entry := range sample.Dictionaries {
		if !validSampleID(entry.ID) || !validSampleText(entry.Name, 1, 500) ||
			!validSampleText(entry.Description, 0, 4000) || !validOptionalSampleID(entry.Slug) ||
			!validOptionalLocale(entry.SourceLocale) || !oneOf(entry.Kind, "manufacturer", "brand", "application", "lifecycle") ||
			(entry.Status != "" && !oneOf(entry.Status, "active", "disabled")) {
			return fmt.Errorf("sample data dictionary entry %q is invalid", entry.ID)
		}
		if _, duplicate := dictionaryIDs[entry.ID]; duplicate {
			return fmt.Errorf("sample data dictionary entry %q is duplicated", entry.ID)
		}
		dictionaryIDs[entry.ID] = entry.Kind
	}
	categoryIDs := map[string]struct{}{"cat_root": {}, "cat_uncategorized": {}}
	for _, category := range sample.Categories {
		if !validSampleID(category.ID) || (category.ParentID != "" && !validSampleID(category.ParentID)) ||
			!validSampleText(category.Name, 1, 240) || !validSampleText(category.Description, 0, 4000) ||
			!validSampleID(category.Slug) || !validOptionalLocale(category.SourceLocale) ||
			(category.Status != "" && category.Status != "active" && category.Status != "disabled") {
			return fmt.Errorf("sample data category %q is invalid", category.ID)
		}
		if _, duplicate := categoryIDs[category.ID]; duplicate {
			return fmt.Errorf("sample data category %q is duplicated", category.ID)
		}
		parentID := category.ParentID
		if parentID == "" {
			parentID = "cat_root"
		}
		if _, exists := categoryIDs[parentID]; !exists {
			return fmt.Errorf("sample data category %q parent is missing or out of order", category.ID)
		}
		categoryIDs[category.ID] = struct{}{}
	}
	specIDs := make(map[string]struct{}, len(sample.Specs))
	for _, spec := range sample.Specs {
		if !validSampleID(spec.ID) || !validSampleText(spec.Name, 1, 500) ||
			!validSampleText(spec.PreferredUnit, 0, 120) || spec.SemanticVersion < 1 ||
			(spec.Status != "" && !oneOf(spec.Status, "active", "disabled")) {
			return fmt.Errorf("sample data specification %q is invalid", spec.ID)
		}
		if _, duplicate := specIDs[spec.ID]; duplicate {
			return fmt.Errorf("sample data specification %q is duplicated", spec.ID)
		}
		specIDs[spec.ID] = struct{}{}
	}
	specSetIDs := make(map[string]map[string]struct{}, len(sample.SpecSets))
	for _, set := range sample.SpecSets {
		if !validSampleID(set.ID) || !validSampleText(set.Name, 1, 500) || len(set.SpecIDs) == 0 ||
			(set.Status != "" && !oneOf(set.Status, "active", "disabled")) {
			return fmt.Errorf("sample data specification set %q is invalid", set.ID)
		}
		if _, duplicate := specSetIDs[set.ID]; duplicate {
			return fmt.Errorf("sample data specification set %q is duplicated", set.ID)
		}
		members := make(map[string]struct{}, len(set.SpecIDs))
		for _, specID := range set.SpecIDs {
			if _, exists := specIDs[specID]; !exists {
				return fmt.Errorf("sample data specification set %q references missing specification %q", set.ID, specID)
			}
			if _, duplicate := members[specID]; duplicate {
				return fmt.Errorf("sample data specification set %q duplicates specification %q", set.ID, specID)
			}
			members[specID] = struct{}{}
		}
		specSetIDs[set.ID] = members
	}
	categorySets := make(map[string]string, len(sample.CategorySpecSets))
	for _, assignment := range sample.CategorySpecSets {
		if _, exists := categoryIDs[assignment.CategoryID]; !exists || assignment.CategoryID == "cat_root" || assignment.CategoryID == "cat_uncategorized" {
			return fmt.Errorf("sample data category specification assignment references invalid category %q", assignment.CategoryID)
		}
		if _, exists := specSetIDs[assignment.SpecSetID]; !exists {
			return fmt.Errorf("sample data category %q references missing specification set %q", assignment.CategoryID, assignment.SpecSetID)
		}
		if _, duplicate := categorySets[assignment.CategoryID]; duplicate {
			return fmt.Errorf("sample data category %q has duplicate specification set assignments", assignment.CategoryID)
		}
		categorySets[assignment.CategoryID] = assignment.SpecSetID
	}
	productIDs := make(map[string]struct{}, len(sample.Products))
	for _, product := range sample.Products {
		if !validSampleID(product.ID) || !validSampleText(product.PartNumber, 1, 240) ||
			!validSampleText(product.Name, 0, 500) || !validOptionalLocale(product.SourceLocale) ||
			!validOptionalDictionaryReference(dictionaryIDs, product.ManufacturerID, "manufacturer") ||
			!validSampleText(product.Manufacturer, 0, 500) || !validOptionalDictionaryReference(dictionaryIDs, product.BrandID, "brand") ||
			!validSampleText(product.Brand, 0, 500) || !validOptionalDictionaryReference(dictionaryIDs, product.LifecycleID, "lifecycle") ||
			(product.CategoryID != "" && !validSampleID(product.CategoryID)) ||
			!validSampleText(product.PackageFormFactor, 0, 500) || !validSampleText(product.Description, 0, 20000) ||
			!validSampleText(product.Features, 0, 20000) || !validSampleText(product.Specification, 0, 20000) ||
			!validSampleText(product.DocumentURL, 0, 2000) ||
			(product.Status != "" && product.Status != "hidden" && product.Status != "published") ||
			(product.RecordState != "" && product.RecordState != "current" && product.RecordState != "archived") {
			return fmt.Errorf("sample data product %q is invalid", product.ID)
		}
		if _, duplicate := productIDs[product.ID]; duplicate {
			return fmt.Errorf("sample data product %q is duplicated", product.ID)
		}
		productIDs[product.ID] = struct{}{}
		categoryID := product.CategoryID
		if categoryID == "" {
			categoryID = "cat_uncategorized"
		}
		if _, exists := categoryIDs[categoryID]; !exists {
			return fmt.Errorf("sample data product %q references missing category %q", product.ID, categoryID)
		}
		applicationIDs := make(map[string]struct{}, len(product.ApplicationIDs))
		for _, applicationID := range product.ApplicationIDs {
			if !validOptionalDictionaryReference(dictionaryIDs, applicationID, "application") {
				return fmt.Errorf("sample data product %q references invalid application %q", product.ID, applicationID)
			}
			if _, duplicate := applicationIDs[applicationID]; duplicate {
				return fmt.Errorf("sample data product %q duplicates application %q", product.ID, applicationID)
			}
			applicationIDs[applicationID] = struct{}{}
		}
		productSpecIDs := make(map[string]struct{}, len(product.SpecValues))
		setMembers := specSetIDs[categorySets[categoryID]]
		for _, value := range product.SpecValues {
			if !validSampleID(value.SpecID) || !validSampleText(value.RawValue, 1, 20000) || !validOptionalLocale(value.SourceLocale) {
				return fmt.Errorf("sample data product %q has an invalid specification value", product.ID)
			}
			if _, exists := specIDs[value.SpecID]; !exists {
				return fmt.Errorf("sample data product %q references missing specification %q", product.ID, value.SpecID)
			}
			if _, applicable := setMembers[value.SpecID]; !applicable {
				return fmt.Errorf("sample data product %q specification %q is not assigned to its category", product.ID, value.SpecID)
			}
			if _, duplicate := productSpecIDs[value.SpecID]; duplicate {
				return fmt.Errorf("sample data product %q duplicates specification %q", product.ID, value.SpecID)
			}
			productSpecIDs[value.SpecID] = struct{}{}
		}
		if product.DocumentURL != "" {
			parsed, err := url.ParseRequestURI(product.DocumentURL)
			if err != nil || !parsed.IsAbs() {
				return fmt.Errorf("sample data product %q document URL is invalid", product.ID)
			}
		}
	}
	return nil
}

func validOptionalDictionaryReference(entries map[string]string, id, kind string) bool {
	if id == "" {
		return true
	}
	return entries[id] == kind
}

func validOptionalLocale(value string) bool {
	return value == "" || validSampleText(value, 2, 35)
}

func validOptionalSampleID(value string) bool {
	return value == "" || validSampleID(value)
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func validSampleText(value string, minimum, maximum int) bool {
	length := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && length >= minimum && length <= maximum
}

func validSampleID(value string) bool {
	if len(value) < 1 || len(value) > 120 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') ||
			char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func (c *Client) UpdateStatus(ctx context.Context, currentVersion, goos, goarch string) UpdateStatus {
	status := UpdateStatus{Status: "unavailable", CurrentVersion: currentVersion, CheckedUTC: time.Now().UTC().Format(time.RFC3339Nano)}
	current := normalizedVersion(currentVersion)
	if current == "" {
		status.Status = "development"
		return status
	}
	release, err := c.LatestRelease(ctx)
	if err != nil || release.Prerelease {
		return status
	}
	latest := normalizedVersion(release.TagName)
	if latest == "" {
		return status
	}
	status.LatestVersion = release.TagName
	status.ReleasePageURL = release.HTMLURL
	if semver.Compare(latest, current) <= 0 {
		status.Status = "current"
		return status
	}
	assetName := platformAssetName(goos, goarch)
	asset, ok := assetNamed(release, assetName)
	if !ok {
		return status
	}
	status.Status = "update_available"
	status.DownloadURL = asset.BrowserDownloadURL
	return status
}

func RuntimeUpdateStatus(ctx context.Context, client *Client, currentVersion string) UpdateStatus {
	return client.UpdateStatus(ctx, currentVersion, runtime.GOOS, runtime.GOARCH)
}

func platformAssetName(goos, goarch string) string {
	name := "prods-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func normalizedVersion(value string) string {
	value = strings.TrimSpace(value)
	if semver.IsValid(value) {
		return value
	}
	if semver.IsValid("v" + value) {
		return "v" + value
	}
	return ""
}

func assetNamed(release Release, name string) (Asset, bool) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return Asset{}, false
}

func (c *Client) cacheFailure(cacheKey string, err error) (Release, error) {
	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[string]cachedRelease)
	}
	c.cache[cacheKey] = cachedRelease{at: time.Now(), err: err}
	c.mu.Unlock()
	return Release{}, err
}

func (c *Client) cacheTTL() time.Duration {
	if c.CacheTTL <= 0 {
		return 10 * time.Minute
	}
	return c.CacheTTL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient == nil {
		return &http.Client{Timeout: 4 * time.Second}
	}
	return c.HTTPClient
}

func (c *Client) apiBase() string {
	if c.APIBase == "" {
		return DefaultAPIBase
	}
	return c.APIBase
}

func (c *Client) validRepositoryURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if c.AllowTestHTTP && parsed.Scheme == "http" {
		return parsed.Host == mustURL(c.apiBase()).Host
	}
	return parsed.Scheme == "https" && parsed.Host == "github.com" && strings.HasPrefix(parsed.Path, "/herefindalex/prods/")
}

func (c *Client) validAssetURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if c.AllowTestHTTP && parsed.Scheme == "http" {
		return parsed.Host == mustURL(c.apiBase()).Host
	}
	return parsed.Scheme == "https" && parsed.Host == "github.com" && strings.HasPrefix(parsed.Path, "/herefindalex/prods/releases/download/")
}

func mustURL(raw string) *url.URL {
	parsed, _ := url.Parse(raw)
	return parsed
}
