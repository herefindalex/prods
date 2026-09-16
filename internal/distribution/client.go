package distribution

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	maxChecksumBytes     = 1024
)

var ErrUnavailable = errors.New("distribution service is unavailable")

type Product struct {
	ID                string `json:"id"`
	PartNumber        string `json:"part_number"`
	Name              string `json:"name,omitempty"`
	Manufacturer      string `json:"manufacturer,omitempty"`
	CategoryID        string `json:"category_id,omitempty"`
	PackageFormFactor string `json:"package_form_factor,omitempty"`
	Description       string `json:"description,omitempty"`
	Features          string `json:"features,omitempty"`
	Specification     string `json:"specification,omitempty"`
	DocumentURL       string `json:"document_url,omitempty"`
	Status            string `json:"status,omitempty"`
	RecordState       string `json:"record_state,omitempty"`
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

type SampleData struct {
	SchemaVersion   int        `json:"schema_version"`
	ReleaseVersion  string     `json:"release_version"`
	DatasetVersion  string     `json:"dataset_version"`
	Seed            int64      `json:"seed"`
	SourceStatement string     `json:"source_statement"`
	Categories      []Category `json:"categories"`
	Products        []Product  `json:"products"`
}

type DownloadedSample struct {
	Data      SampleData
	SourceURL string
	SHA256    string
	Path      string
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

func (c *Client) ReleaseForVersion(ctx context.Context, version string) (Release, error) {
	version = strings.TrimSpace(version)
	if version == "" || version == "dev" || strings.ContainsAny(version, "\r\n") {
		return Release{}, fmt.Errorf("%w: this build has no release version", ErrUnavailable)
	}
	endpoint := strings.TrimRight(c.apiBase(), "/") + "/repos/herefindalex/prods/releases/tags/" + url.PathEscape(version)
	release, err := c.release(ctx, "tag:"+version, endpoint)
	if err != nil {
		return Release{}, err
	}
	if release.TagName != version {
		return Release{}, fmt.Errorf("%w: release tag does not match this build", ErrUnavailable)
	}
	return release, nil
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

func (c *Client) SampleAvailable(ctx context.Context, version string) bool {
	release, err := c.ReleaseForVersion(ctx, version)
	if err != nil || release.Prerelease {
		return false
	}
	_, dataOK := assetNamed(release, SampleAssetName)
	_, checksumOK := assetNamed(release, SampleChecksumName)
	return dataOK && checksumOK
}

func (c *Client) DownloadSampleData(ctx context.Context, dataDir, version string) (DownloadedSample, error) {
	release, err := c.ReleaseForVersion(ctx, version)
	if err != nil {
		return DownloadedSample{}, err
	}
	if release.Prerelease {
		return DownloadedSample{}, fmt.Errorf("%w: matching release is a prerelease", ErrUnavailable)
	}
	dataAsset, ok := assetNamed(release, SampleAssetName)
	if !ok {
		return DownloadedSample{}, fmt.Errorf("%w: release has no %s", ErrUnavailable, SampleAssetName)
	}
	checksumAsset, ok := assetNamed(release, SampleChecksumName)
	if !ok {
		return DownloadedSample{}, fmt.Errorf("%w: release has no %s", ErrUnavailable, SampleChecksumName)
	}
	checksumBody, err := c.download(ctx, checksumAsset.BrowserDownloadURL, maxChecksumBytes)
	if err != nil {
		return DownloadedSample{}, fmt.Errorf("download sample checksum: %w", err)
	}
	want, err := parseChecksum(checksumBody)
	if err != nil {
		return DownloadedSample{}, err
	}
	body, err := c.download(ctx, dataAsset.BrowserDownloadURL, maxSampleBytes)
	if err != nil {
		return DownloadedSample{}, fmt.Errorf("download sample data: %w", err)
	}
	digest := sha256.Sum256(body)
	got := hex.EncodeToString(digest[:])
	if got != want {
		return DownloadedSample{}, errors.New("sample data checksum does not match the signed release manifest")
	}
	var sample SampleData
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sample); err != nil {
		return DownloadedSample{}, fmt.Errorf("decode sample data: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return DownloadedSample{}, errors.New("sample data format is invalid")
	}
	if err := validateSampleData(sample, strings.TrimSpace(version)); err != nil {
		return DownloadedSample{}, err
	}
	destinationDir := filepath.Join(dataDir, "sample-data")
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return DownloadedSample{}, fmt.Errorf("create sample data directory: %w", err)
	}
	destination := filepath.Join(destinationDir, SampleAssetName)
	if err := atomicWrite(destination, body); err != nil {
		return DownloadedSample{}, err
	}
	return DownloadedSample{Data: sample, SourceURL: dataAsset.BrowserDownloadURL, SHA256: got, Path: destination}, nil
}

func validateSampleData(sample SampleData, version string) error {
	if sample.SchemaVersion != 1 || sample.ReleaseVersion != version ||
		strings.TrimSpace(sample.DatasetVersion) == "" || strings.TrimSpace(sample.SourceStatement) == "" || len(sample.Products) == 0 {
		return errors.New("sample data format is invalid")
	}
	for _, category := range sample.Categories {
		if !validSampleID(category.ID) || (category.ParentID != "" && !validSampleID(category.ParentID)) ||
			!validSampleText(category.Name, 1, 240) || !validSampleText(category.Description, 0, 4000) ||
			!validSampleID(category.Slug) || (category.SourceLocale != "" && !validSampleText(category.SourceLocale, 2, 35)) ||
			(category.Status != "" && category.Status != "active" && category.Status != "disabled") {
			return fmt.Errorf("sample data category %q is invalid", category.ID)
		}
	}
	for _, product := range sample.Products {
		if !validSampleID(product.ID) || !validSampleText(product.PartNumber, 1, 240) ||
			!validSampleText(product.Name, 0, 500) || !validSampleText(product.Manufacturer, 0, 500) ||
			(product.CategoryID != "" && !validSampleID(product.CategoryID)) ||
			!validSampleText(product.PackageFormFactor, 0, 500) || !validSampleText(product.Description, 0, 20000) ||
			!validSampleText(product.Features, 0, 20000) || !validSampleText(product.Specification, 0, 20000) ||
			!validSampleText(product.DocumentURL, 0, 2000) ||
			(product.Status != "" && product.Status != "hidden" && product.Status != "published") ||
			(product.RecordState != "" && product.RecordState != "current" && product.RecordState != "archived") {
			return fmt.Errorf("sample data product %q is invalid", product.ID)
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

func parseChecksum(body []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	if !scanner.Scan() {
		return "", errors.New("sample checksum is empty")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", errors.New("sample checksum is invalid")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("sample checksum is invalid")
	}
	return strings.ToLower(fields[0]), nil
}

func (c *Client) download(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	if !c.validAssetURL(rawURL) {
		return nil, errors.New("release asset URL is not trusted")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Prods-sample-installer")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	reader := http.MaxBytesReader(nil, resp.Body, limit)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("asset exceeds %d bytes or could not be read: %w", limit, err)
	}
	return body, nil
}

func atomicWrite(destination string, body []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(destination), ".sample-data-*")
	if err != nil {
		return fmt.Errorf("create sample data file: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		return fmt.Errorf("publish sample data file: %w", err)
	}
	return nil
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
