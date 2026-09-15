package publishing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"prods/internal/catalog"
	"prods/internal/localization"
	"prods/internal/platform"
	"prods/internal/site"
)

var (
	ErrPublicUnavailable = errors.New("public index unavailable")
	ErrArtifactInvalid   = errors.New("public artifact is missing or invalid")
	ErrSiteRouteInvalid  = errors.New("site route candidate is invalid")
)

type Intent struct {
	ID              string
	ProductID       string
	DesiredRevision int64
	Cause           string
}

type SourceSpec struct {
	ID            string
	Name          string
	RawValue      string
	PreferredUnit string
	Language      string
}

type SourceDocument struct {
	ID       string
	Label    string
	Type     string
	AssetID  string
	URL      string
	Language string
}

type SourceImage struct {
	ID          string
	AssetID     string
	ExternalURL string
	AltText     string
	SortOrder   int
	Primary     bool
}

type Source struct {
	Product                    catalog.Product
	Translations               []catalog.ProductTranslation
	SourceLocale               string
	SourceLocales              map[string]string
	ContentMultilingualEnabled bool
	Site                       site.Configuration
	Category                   string
	CategoryPath               string
	CategoryTrail              []SourceCategory
	ManufacturerSlug           string
	BrandSlug                  string
	Specs                      []SourceSpec
	Documents                  []SourceDocument
	Images                     []SourceImage
	SiteEpoch                  int64
	Route                      string
	Language                   string
	SupportedLocales           []string
	LabelLocalizations         map[string]LocalizedLabel
	PublicCopyDefaults         map[string]map[string]string
	PublicCopyOverrides        localization.PublicCopyOverrideMap
}

type SourceCategory struct {
	ID   string
	Name string
	Path string
}

type ActivePublication struct {
	ProductID            string
	SourceRevision       int64
	PublicRevision       int64
	SiteEpoch            int64
	Route                string
	ArtifactID           string
	ManifestHash         string
	VisibilityGeneration int64
	ActivatedAt          time.Time
	View                 PublicView
}

type RevokeRequest struct {
	ActorID          string
	ProductID        string
	ExpectedRevision int64
	Archive          bool
}

type PublicAsset struct {
	ID          string
	StoragePath string
	MIMEType    string
	SizeBytes   int64
	Checksum    string
}

type RoutePattern string

const (
	RouteCompact      RoutePattern = "compact"
	RouteManufacturer RoutePattern = "manufacturer"
	RouteBrand        RoutePattern = "brand"
	RouteCategory     RoutePattern = "category"
)

type SiteRouteConfig struct {
	ProductPrefix string       `json:"product_prefix"`
	Pattern       RoutePattern `json:"url_pattern"`
}

type SiteRouteIssue struct {
	ProductID  string `json:"product_id"`
	PartNumber string `json:"part_number"`
	Route      string `json:"route,omitempty"`
	Reason     string `json:"reason"`
}

type SiteRoutePreview struct {
	CurrentEpoch    int64            `json:"current_epoch"`
	CandidateEpoch  int64            `json:"candidate_epoch"`
	WorkingRevision int64            `json:"working_revision"`
	Affected        int              `json:"affected"`
	Missing         []SiteRouteIssue `json:"missing"`
	Conflicts       []SiteRouteIssue `json:"conflicts"`
}

type SiteRouteRequest struct {
	ActorID                 string
	ExpectedEpoch           int64
	ExpectedWorkingRevision int64
	Config                  SiteRouteConfig
}

type RouteHistory struct {
	Route       string
	ProductID   string
	TargetRoute string
	State       string
	SiteEpoch   int64
}

// VisibilityCoordinator is called by the repository after it has acquired
// the bounded writer admission and before it begins the visibility-changing
// transaction. Install runs after a successful commit while admissions remain
// blocked. Implementations must fail closed if installation cannot complete.
type VisibilityCoordinator interface {
	LockVisibility()
	InstallVisibility(context.Context) error
	UnlockVisibility()
}

type Repository interface {
	ResetPublicationClaims(context.Context) error
	ClaimPublicationIntent(context.Context) (Intent, bool, error)
	PublicationSource(context.Context, string) (Source, error)
	ActivatePublication(context.Context, Intent, ActivePublication, VisibilityCoordinator) (bool, error)
	CompletePublicationIntent(context.Context, string, string) error
	FailPublicationIntent(context.Context, string, string) error
	ActivePublications(context.Context) ([]ActivePublication, error)
	ActiveWebsiteConfiguration(context.Context) (site.Configuration, int64, int64, error)
	WorkingWebsiteConfiguration(context.Context) (site.Configuration, int64, error)
	WebsiteCustomCSSState(context.Context) (bool, int64, error)
	SearchPublications(context.Context, string, string) ([]ActivePublication, error)
	RebuildSearchPublications(context.Context, []ActivePublication) error
	ConvergePublicationDependencies(context.Context, string, int64) error
	ActiveRouteHistory(context.Context) ([]RouteHistory, error)
	PublicAssets(context.Context, []string) ([]PublicAsset, error)
	PrepareSiteRoutes(context.Context, SiteRouteConfig) (SiteRoutePreview, []Source, error)
	ActivateSiteRoutes(context.Context, SiteRouteRequest, []ActivePublication, VisibilityCoordinator) error
	QueuePublicationRepair(context.Context, ActivePublication, string) error
	RevokeProduct(context.Context, RevokeRequest, VisibilityCoordinator) error
}

type Config struct {
	Root           string
	AssetRoot      string
	BaseURL        string
	ResourceGate   *platform.ResourceGate
	ByteHeadroom   uint64
	InodeHeadroom  uint64
	MinFreePercent uint8
}

type publicIndex struct {
	byID              map[string]ActivePublication
	byRoute           map[string]routeTarget
	byAsset           map[string]assetTarget
	history           map[string]RouteHistory
	views             []PublicView
	invalid           []ActivePublication
	unhealthy         error
	site              site.Configuration
	siteEpoch         int64
	customCSSDisabled bool
	runtimeGeneration int64
	machine           map[string]MachineRepresentation
}

type routeTarget struct {
	publication ActivePublication
	file        string
	contentType string
}

type assetTarget struct {
	asset PublicAsset
	path  string
}

type Engine struct {
	repository                 Repository
	root                       string
	assetRoot                  string
	baseURL                    string
	resources                  *platform.ResourceGate
	byteHeadroom               uint64
	inodeHeadroom              uint64
	minFreePercent             uint8
	gate                       sync.RWMutex
	contentLocales             []string
	contentMultilingualEnabled bool
	index                      *publicIndex
	wake                       chan struct{}
	cancel                     context.CancelFunc
	wg                         sync.WaitGroup
	draining                   atomic.Bool
}

func NewEngine(ctx context.Context, repository Repository, config Config) (*Engine, error) {
	if repository == nil || strings.TrimSpace(config.Root) == "" || strings.TrimSpace(config.BaseURL) == "" {
		return nil, errors.New("publishing repository, root, and base URL are required")
	}
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "units"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "staging"), 0o700); err != nil {
		return nil, err
	}
	assetRoot := strings.TrimSpace(config.AssetRoot)
	if assetRoot == "" {
		assetRoot = filepath.Join(filepath.Dir(root), "assets")
	}
	assetRoot, err = filepath.Abs(assetRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(assetRoot, 0o700); err != nil {
		return nil, err
	}
	if config.ResourceGate == nil {
		config.ResourceGate = platform.NewResourceGate(nil)
	}
	engine := &Engine{
		repository: repository, root: root, assetRoot: assetRoot, baseURL: strings.TrimRight(config.BaseURL, "/"),
		resources: config.ResourceGate, byteHeadroom: config.ByteHeadroom, inodeHeadroom: config.InodeHeadroom,
		minFreePercent: config.MinFreePercent, wake: make(chan struct{}, 1),
	}
	if err := repository.ResetPublicationClaims(ctx); err != nil {
		return nil, err
	}
	if err := engine.Reconcile(ctx); err != nil {
		return nil, err
	}
	workerContext, cancel := context.WithCancel(context.Background())
	engine.cancel = cancel
	engine.wg.Add(1)
	go engine.worker(workerContext)
	engine.Wake()
	return engine, nil
}

func (e *Engine) Close() {
	if e == nil || e.cancel == nil {
		return
	}
	e.BeginDrain()
	e.cancel()
	e.wg.Wait()
}

func (e *Engine) BeginDrain() {
	if e != nil {
		e.draining.Store(true)
	}
}

func (e *Engine) Wake() {
	if e == nil || e.draining.Load() {
		return
	}
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) worker(ctx context.Context) {
	defer e.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
		case <-ticker.C:
		}
		if e.draining.Load() {
			return
		}
		for {
			if e.draining.Load() {
				return
			}
			processed, err := e.ProcessOne(ctx)
			if err != nil || !processed {
				break
			}
		}
	}
}

func (e *Engine) ProcessOne(ctx context.Context) (bool, error) {
	if e == nil || e.draining.Load() {
		return false, nil
	}
	intent, found, err := e.repository.ClaimPublicationIntent(ctx)
	if err != nil || !found {
		return found, err
	}
	source, err := e.repository.PublicationSource(ctx, intent.ProductID)
	if err != nil {
		_ = e.repository.FailPublicationIntent(context.Background(), intent.ID, err.Error())
		return true, err
	}
	if source.Product.RecordState != catalog.RecordCurrent || source.Product.Status != catalog.Published || source.Product.Revision != intent.DesiredRevision {
		if err := e.repository.CompletePublicationIntent(ctx, intent.ID, "superseded"); err != nil {
			return true, err
		}
		return true, nil
	}
	view := e.viewFromSource(source)
	if _, err := e.assetsForView(ctx, view, true); err != nil {
		_ = e.repository.FailPublicationIntent(context.Background(), intent.ID, err.Error())
		return true, err
	}
	artifactID, manifestHash, err := e.stage(ctx, intent.ID, view)
	if err != nil {
		_ = e.repository.FailPublicationIntent(context.Background(), intent.ID, err.Error())
		return true, err
	}
	activation := ActivePublication{
		ProductID: source.Product.ID, SourceRevision: source.Product.Revision, PublicRevision: source.Product.Revision,
		SiteEpoch: source.SiteEpoch, Route: source.Route, ArtifactID: artifactID, ManifestHash: manifestHash,
		ActivatedAt: time.Now().UTC(), View: view,
	}
	activated, err := e.repository.ActivatePublication(ctx, intent, activation, e)
	if err != nil {
		return true, err
	}
	if !activated {
		_ = e.repository.CompletePublicationIntent(ctx, intent.ID, "superseded")
	} else if err := e.repository.ConvergePublicationDependencies(ctx, activation.ProductID, activation.SourceRevision); err != nil {
		return true, err
	}
	return true, nil
}

func (e *Engine) Revoke(ctx context.Context, request RevokeRequest) error {
	if err := e.repository.RevokeProduct(ctx, request, e); err != nil {
		return err
	}
	return e.repository.ConvergePublicationDependencies(ctx, request.ProductID, request.ExpectedRevision+1)
}

func (e *Engine) PreviewSiteRoutes(ctx context.Context, config SiteRouteConfig) (SiteRoutePreview, error) {
	preview, _, err := e.repository.PrepareSiteRoutes(ctx, config)
	return preview, err
}

func (e *Engine) PublishSiteRoutes(ctx context.Context, request SiteRouteRequest) (SiteRoutePreview, error) {
	preview, sources, err := e.repository.PrepareSiteRoutes(ctx, request.Config)
	if err != nil {
		return preview, err
	}
	if preview.CurrentEpoch != request.ExpectedEpoch || preview.WorkingRevision != request.ExpectedWorkingRevision || len(preview.Missing) != 0 || len(preview.Conflicts) != 0 {
		return preview, ErrSiteRouteInvalid
	}
	working, workingRevision, err := e.repository.WorkingWebsiteConfiguration(ctx)
	if err != nil {
		return preview, err
	}
	if workingRevision != request.ExpectedWorkingRevision {
		return preview, ErrSiteRouteInvalid
	}
	if _, err := e.assetsForSite(ctx, working, true); err != nil {
		return preview, err
	}
	activations := make([]ActivePublication, 0, len(sources))
	for index, source := range sources {
		view := e.viewFromSource(source)
		if _, err := e.assetsForView(ctx, view, true); err != nil {
			return preview, err
		}
		artifactID, manifestHash, err := e.stage(ctx, fmt.Sprintf("site-%d-%d", preview.CandidateEpoch, index), view)
		if err != nil {
			return preview, err
		}
		activations = append(activations, ActivePublication{
			ProductID: source.Product.ID, SourceRevision: source.Product.Revision, PublicRevision: source.Product.Revision,
			SiteEpoch: preview.CandidateEpoch, Route: source.Route, ArtifactID: artifactID, ManifestHash: manifestHash,
			ActivatedAt: time.Now().UTC(), View: view,
		})
	}
	if err := e.repository.ActivateSiteRoutes(ctx, request, activations, e); err != nil {
		return preview, err
	}
	for _, activation := range activations {
		if err := e.repository.ConvergePublicationDependencies(ctx, activation.ProductID, activation.SourceRevision); err != nil {
			return preview, err
		}
	}
	return preview, nil
}

func (e *Engine) Reconcile(ctx context.Context) error {
	e.gate.Lock()
	err := e.installLocked(ctx)
	var invalid []ActivePublication
	var active []ActivePublication
	if e.index != nil {
		invalid = append(invalid, e.index.invalid...)
		for _, activation := range e.index.byID {
			active = append(active, activation)
		}
	}
	e.gate.Unlock()
	if err != nil {
		return err
	}
	for _, activation := range invalid {
		if err := e.repository.QueuePublicationRepair(ctx, activation, "active artifact missing or invalid"); err != nil {
			return err
		}
	}
	if err := e.repository.RebuildSearchPublications(ctx, active); err != nil {
		return err
	}
	for _, activation := range active {
		if err := e.repository.ConvergePublicationDependencies(ctx, activation.ProductID, activation.SourceRevision); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) LockVisibility() { e.gate.Lock() }

func (e *Engine) InstallVisibility(ctx context.Context) error { return e.installLocked(ctx) }

func (e *Engine) UnlockVisibility() { e.gate.Unlock() }

func (e *Engine) SetContentLanguagePolicy(locales []string, enabled bool) {
	// Kept as a compatibility hook for older Admin clients. Public language
	// admission comes only from the active Website revision on each PublicView.
	_ = locales
	_ = enabled
}

func (e *Engine) installLocked(ctx context.Context) error {
	activations, err := e.repository.ActivePublications(ctx)
	if err != nil {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	configuration, _, siteEpoch, err := e.repository.ActiveWebsiteConfiguration(ctx)
	if err != nil {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	customCSSDisabled, runtimeGeneration, err := e.repository.WebsiteCustomCSSState(ctx)
	if err != nil {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	next := &publicIndex{byID: make(map[string]ActivePublication), byRoute: make(map[string]routeTarget), byAsset: make(map[string]assetTarget), history: make(map[string]RouteHistory), site: configuration, siteEpoch: siteEpoch, customCSSDisabled: customCSSDisabled, runtimeGeneration: runtimeGeneration}
	siteAssets, err := e.assetsForSite(ctx, configuration, false)
	if err != nil {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	for _, asset := range siteAssets {
		next.byAsset[asset.asset.ID] = asset
	}
	for _, activation := range activations {
		if err := e.validateUnit(activation); err != nil {
			next.invalid = append(next.invalid, activation)
			continue
		}
		assets, err := e.assetsForView(ctx, activation.View, false)
		if err != nil {
			next.invalid = append(next.invalid, activation)
			continue
		}
		if _, conflict := next.byRoute[activation.Route]; conflict {
			continue
		}
		next.byID[activation.ProductID] = activation
		next.views = append(next.views, activation.View)
		next.byRoute[activation.Route] = routeTarget{publication: activation, file: "index.html", contentType: "text/html; charset=utf-8"}
		next.byRoute[activation.Route+".json"] = routeTarget{publication: activation, file: "product.json", contentType: "application/json; charset=utf-8"}
		next.byRoute[activation.Route+".md"] = routeTarget{publication: activation, file: "product.md", contentType: "text/markdown; charset=utf-8"}
		for _, asset := range assets {
			next.byAsset[asset.asset.ID] = asset
		}
	}
	history, err := e.repository.ActiveRouteHistory(ctx)
	if err != nil {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	for _, item := range history {
		if _, active := next.byRoute[item.Route]; active {
			continue
		}
		if item.State == "redirect" {
			if _, targetActive := next.byRoute[item.TargetRoute]; !targetActive {
				continue
			}
		}
		if item.State == "redirect" || item.State == "gone" {
			next.history[item.Route] = item
		}
	}
	sort.Slice(next.views, func(i, j int) bool {
		if next.views[i].PartNumber == next.views[j].PartNumber {
			return next.views[i].ID < next.views[j].ID
		}
		return next.views[i].PartNumber < next.views[j].PartNumber
	})
	next.machine, err = MachineRepresentations(next.views, configuration, e.baseURL, siteEpoch)
	if err != nil {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	e.index = next
	return nil
}

func (e *Engine) viewFromSource(source Source) PublicView {
	view := PublicView{
		Site: source.Site,
		ID:   source.Product.ID, Revision: source.Product.Revision, SiteEpoch: source.SiteEpoch,
		PartNumber: source.Product.PartNumber, Name: source.Product.Name, Manufacturer: source.Product.Manufacturer,
		ManufacturerID: source.Product.ManufacturerID, Brand: source.Product.Brand, BrandID: source.Product.BrandID,
		Lifecycle: source.Product.Lifecycle, LifecycleID: source.Product.LifecycleID,
		Category: source.Category, CategoryID: source.Product.CategoryID, Description: source.Product.Description,
		Features: source.Product.Features, Specification: source.Product.Specification,
		CanonicalURL: e.baseURL + source.Route, Language: source.Language, DefaultLocale: source.Language,
		SupportedLocales:    append([]string(nil), source.SupportedLocales...),
		RFQURL:              "/rfq?product_id=" + source.Product.ID,
		Localizations:       make(map[string]LocalizedContent),
		SourceLocale:        source.SourceLocale,
		SourceLocales:       source.SourceLocales,
		LabelLocalizations:  source.LabelLocalizations,
		PublicCopyDefaults:  source.PublicCopyDefaults,
		PublicCopyOverrides: source.PublicCopyOverrides,
	}
	for _, item := range source.Translations {
		view.Localizations[item.Locale] = LocalizedContent{Name: item.Name, Description: item.Description, Features: item.Features, Specification: item.Specification}
	}
	if source.CategoryPath != "" {
		view.CategoryURL = e.baseURL + "/categories/" + source.CategoryPath
	}
	for _, category := range source.CategoryTrail {
		if category.ID == "" || category.Name == "" || category.Path == "" {
			continue
		}
		view.CategoryTrail = append(view.CategoryTrail, CategoryRef{
			ID: category.ID, Name: category.Name, URL: e.baseURL + "/categories/" + category.Path,
		})
	}
	if source.ManufacturerSlug != "" {
		view.ManufacturerURL = e.baseURL + "/manufacturers/" + source.ManufacturerSlug
	}
	if source.BrandSlug != "" {
		view.BrandURL = e.baseURL + "/brands/" + source.BrandSlug
	}
	for _, application := range source.Product.Applications {
		view.Applications = append(view.Applications, Application{
			ID: application.ID, Name: application.Name,
			URL: e.baseURL + "/applications/" + application.Slug,
		})
	}
	for _, image := range source.Images {
		location := image.ExternalURL
		if image.AssetID != "" {
			location = "/assets/" + image.AssetID
		}
		altText := image.AltText
		if altText == "" {
			altText = source.Product.DisplayName()
		}
		view.Images = append(view.Images, Image{
			ID: image.ID, AssetID: image.AssetID, URL: location, AltText: altText, Primary: image.Primary,
		})
	}
	if view.Language == "" {
		view.Language = "en-US"
	}
	for _, spec := range source.Specs {
		view.Specifications = append(view.Specifications, Specification(spec))
	}
	for _, document := range source.Documents {
		location := document.URL
		if document.AssetID != "" {
			location = "/assets/" + document.AssetID
		}
		view.Documents = append(view.Documents, Document{
			ID: document.ID, Label: document.Label, Type: document.Type, AssetID: document.AssetID, URL: location, Language: document.Language,
		})
	}
	if len(view.Documents) == 0 && source.Product.DocumentURL != "" {
		view.Documents = append(view.Documents, Document{Label: "Datasheet", URL: source.Product.DocumentURL})
	}
	return view
}

func (e *Engine) assetsForView(ctx context.Context, view PublicView, verifyChecksum bool) ([]assetTarget, error) {
	seen := make(map[string]struct{})
	ids := make([]string, 0, len(view.Documents)+len(view.Images))
	for _, document := range view.Documents {
		if document.AssetID == "" {
			continue
		}
		if _, exists := seen[document.AssetID]; exists {
			continue
		}
		seen[document.AssetID] = struct{}{}
		ids = append(ids, document.AssetID)
	}
	for _, image := range view.Images {
		if image.AssetID == "" {
			continue
		}
		if _, exists := seen[image.AssetID]; exists {
			continue
		}
		seen[image.AssetID] = struct{}{}
		ids = append(ids, image.AssetID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	records, err := e.repository.PublicAssets(ctx, ids)
	if err != nil || len(records) != len(ids) {
		return nil, ErrArtifactInvalid
	}
	targets := make([]assetTarget, 0, len(records))
	for _, asset := range records {
		path, err := e.validateAsset(asset, verifyChecksum)
		if err != nil {
			return nil, err
		}
		targets = append(targets, assetTarget{asset: asset, path: path})
	}
	return targets, nil
}

func (e *Engine) assetsForSite(ctx context.Context, configuration site.Configuration, verifyChecksum bool) ([]assetTarget, error) {
	ids := configuration.AssetIDs()
	if len(ids) == 0 {
		return nil, nil
	}
	records, err := e.repository.PublicAssets(ctx, ids)
	if err != nil || len(records) != len(ids) {
		return nil, ErrArtifactInvalid
	}
	targets := make([]assetTarget, 0, len(records))
	for _, asset := range records {
		path, err := e.validateAsset(asset, verifyChecksum)
		if err != nil {
			return nil, err
		}
		targets = append(targets, assetTarget{asset: asset, path: path})
	}
	return targets, nil
}

func (e *Engine) validateAsset(asset PublicAsset, verifyChecksum bool) (string, error) {
	checksum, checksumErr := hex.DecodeString(asset.Checksum)
	if asset.ID == "" || asset.StoragePath == "" || filepath.IsAbs(asset.StoragePath) || asset.SizeBytes < 0 || checksumErr != nil || len(checksum) != sha256.Size {
		return "", ErrArtifactInvalid
	}
	clean := filepath.Clean(asset.StoragePath)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrArtifactInvalid
	}
	path := filepath.Join(e.assetRoot, clean)
	resolvedRoot, err := filepath.EvalSymlinks(e.assetRoot)
	if err != nil {
		return "", ErrArtifactInvalid
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", ErrArtifactInvalid
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrArtifactInvalid
	}
	info, err := os.Lstat(resolvedPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.SizeBytes {
		return "", ErrArtifactInvalid
	}
	if verifyChecksum {
		file, err := os.Open(resolvedPath)
		if err != nil {
			return "", ErrArtifactInvalid
		}
		hasher := sha256.New()
		_, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || hex.EncodeToString(hasher.Sum(nil)) != asset.Checksum {
			return "", ErrArtifactInvalid
		}
	}
	return resolvedPath, nil
}

type unitManifest struct {
	ProductID string            `json:"product_id"`
	Revision  int64             `json:"revision"`
	SiteEpoch int64             `json:"site_epoch"`
	Files     map[string]string `json:"files"`
}

func (e *Engine) stage(ctx context.Context, operationID string, view PublicView) (string, string, error) {
	artifactID, err := randomArtifactID()
	if err != nil {
		return "", "", err
	}
	defaultView := view.ForLocale(view.Language)
	htmlBody, err := HTML(defaultView)
	if err != nil {
		return "", "", err
	}
	jsonBody, err := JSON(defaultView)
	if err != nil {
		return "", "", err
	}
	jsonLDBody, err := JSONLD(defaultView)
	if err != nil {
		return "", "", err
	}
	files := map[string][]byte{
		"index.html": htmlBody, "product.json": jsonBody, "product.jsonld": jsonLDBody, "product.md": Markdown(defaultView),
	}
	for _, locale := range publicSupportedLocales(view) {
		localizedView := view.ForLocale(locale)
		localizedHTML, err := HTMLForLocale(localizedView, locale)
		if err != nil {
			return "", "", err
		}
		localizedJSON, err := JSON(localizedView)
		if err != nil {
			return "", "", err
		}
		localizedJSONLD, err := JSONLD(localizedView)
		if err != nil {
			return "", "", err
		}
		files[localizedArtifactName(locale, "index.html")] = localizedHTML
		files[localizedArtifactName(locale, "product.json")] = localizedJSON
		files[localizedArtifactName(locale, "product.jsonld")] = localizedJSONLD
		files[localizedArtifactName(locale, "product.md")] = Markdown(localizedView)
		fallbackView := view
		fallbackView.Localizations = nil
		fallbackView = fallbackView.ForLocale(locale)
		fallbackHTML, err := HTMLForLocale(fallbackView, locale)
		if err != nil {
			return "", "", err
		}
		fallbackJSON, err := JSON(fallbackView)
		if err != nil {
			return "", "", err
		}
		fallbackJSONLD, err := JSONLD(fallbackView)
		if err != nil {
			return "", "", err
		}
		files[fallbackLocalizedArtifactName(locale, "index.html")] = fallbackHTML
		files[fallbackLocalizedArtifactName(locale, "product.json")] = fallbackJSON
		files[fallbackLocalizedArtifactName(locale, "product.jsonld")] = fallbackJSONLD
		files[fallbackLocalizedArtifactName(locale, "product.md")] = Markdown(fallbackView)
	}
	var requiredBytes uint64 = 16 << 10
	for _, body := range files {
		if uint64(len(body)) > ^uint64(0)-requiredBytes {
			return "", "", platform.ErrResourceCritical
		}
		requiredBytes += uint64(len(body))
	}
	reservation, _, err := e.resources.Admit(ctx, platform.ResourceRequest{
		Operation: "public product generation", Path: e.root,
		RequiredBytes: requiredBytes, ByteHeadroom: e.byteHeadroom, MinFreePercent: e.minFreePercent,
		RequiredInodes: uint64(len(files)) + 3, InodeHeadroom: e.inodeHeadroom,
	})
	if err != nil {
		return "", "", err
	}
	defer reservation.Release()
	staging := filepath.Join(e.root, "staging", operationID+"-"+artifactID)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return "", "", err
	}
	defer os.RemoveAll(staging)
	manifest := unitManifest{ProductID: view.ID, Revision: view.Revision, SiteEpoch: view.SiteEpoch, Files: make(map[string]string, len(files))}
	for name, body := range files {
		manifest.Files[name] = hashBytes(body)
		if err := writeDurable(filepath.Join(staging, name), body); err != nil {
			return "", "", err
		}
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return "", "", err
	}
	manifestHash := hashBytes(manifestBody)
	if err := writeDurable(filepath.Join(staging, "manifest.json"), manifestBody); err != nil {
		return "", "", err
	}
	if err := writeDurable(filepath.Join(staging, "prepared"), []byte(manifestHash)); err != nil {
		return "", "", err
	}
	destination := filepath.Join(e.root, "units", artifactID)
	if err := os.Rename(staging, destination); err != nil {
		return "", "", err
	}
	return artifactID, manifestHash, nil
}

func (e *Engine) validateUnit(activation ActivePublication) error {
	directory, err := e.unitDirectory(activation.ArtifactID)
	if err != nil {
		return err
	}
	prepared, err := os.ReadFile(filepath.Join(directory, "prepared"))
	if err != nil || string(prepared) != activation.ManifestHash {
		return ErrArtifactInvalid
	}
	manifestBody, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || hashBytes(manifestBody) != activation.ManifestHash {
		return ErrArtifactInvalid
	}
	var manifest unitManifest
	if json.Unmarshal(manifestBody, &manifest) != nil || manifest.ProductID != activation.ProductID || manifest.Revision != activation.PublicRevision || manifest.SiteEpoch != activation.SiteEpoch {
		return ErrArtifactInvalid
	}
	for name, expected := range manifest.Files {
		body, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || hashBytes(body) != expected {
			return ErrArtifactInvalid
		}
	}
	return nil
}

func (e *Engine) unitDirectory(artifactID string) (string, error) {
	if artifactID == "" || strings.ContainsAny(artifactID, `/\\`) || artifactID == "." || artifactID == ".." {
		return "", ErrArtifactInvalid
	}
	directory := filepath.Join(e.root, "units", artifactID)
	if filepath.Dir(directory) != filepath.Join(e.root, "units") {
		return "", ErrArtifactInvalid
	}
	return directory, nil
}

type Lease struct {
	Publication ActivePublication
	File        *os.File
	Name        string
	ContentType string
}

func (e *Engine) AdmitPath(requestPath string) (Lease, error) {
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return Lease{}, ErrPublicUnavailable
	}
	target, exists := e.index.byRoute[requestPath]
	if !exists {
		return Lease{}, os.ErrNotExist
	}
	directory, err := e.unitDirectory(target.publication.ArtifactID)
	if err != nil {
		return Lease{}, err
	}
	file, err := os.Open(filepath.Join(directory, target.file))
	if err != nil {
		return Lease{}, ErrArtifactInvalid
	}
	return Lease{Publication: target.publication, File: file, Name: target.file, ContentType: target.contentType}, nil
}

func (e *Engine) ServePath(w http.ResponseWriter, r *http.Request) bool {
	lease, err := e.AdmitPath(r.URL.Path)
	if errors.Is(err, os.ErrNotExist) {
		e.gate.RLock()
		history, exists := e.index.history[r.URL.Path]
		e.gate.RUnlock()
		if !exists {
			return false
		}
		if history.State == "redirect" {
			http.Redirect(w, r, history.TargetRoute, http.StatusPermanentRedirect)
		} else {
			http.Error(w, "gone", http.StatusGone)
		}
		return true
	}
	if err != nil {
		http.Error(w, "public representation temporarily unavailable", http.StatusServiceUnavailable)
		return true
	}
	locale := lease.Publication.View.Language
	if explicit := r.URL.Query().Get("lang"); explicit != "" {
		supported := publicSupportedLocales(lease.Publication.View)
		resolvedLocale, resolveErr := localization.ResolvePublishedLocale(lease.Publication.View.DefaultLocale, supported, explicit, "")
		err = resolveErr
		if err != nil {
			lease.File.Close()
			http.Error(w, "unsupported language", http.StatusBadRequest)
			return true
		}
		locale = resolvedLocale
		lease.File.Close()
		directory, directoryErr := e.unitDirectory(lease.Publication.ArtifactID)
		if directoryErr != nil {
			http.Error(w, "public representation temporarily unavailable", http.StatusServiceUnavailable)
			return true
		}
		lease.Name = localizedArtifactName(locale, lease.Name)
		lease.File, err = os.Open(filepath.Join(directory, lease.Name))
		if err != nil {
			http.Error(w, "public representation temporarily unavailable", http.StatusServiceUnavailable)
			return true
		}
	}
	defer lease.File.Close()
	etagMaterial := fmt.Sprintf("%s\x00%d\x00%d\x00%s\x00%s\x00%s", lease.Publication.ProductID, lease.Publication.PublicRevision, lease.Publication.SiteEpoch, lease.Publication.ManifestHash, locale, lease.Name)
	etag := `"p-` + hashBytes([]byte(etagMaterial))[:32] + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("Content-Type", lease.ContentType)
	w.Header().Set("Content-Language", locale)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	http.ServeContent(w, r, lease.Name, lease.Publication.ActivatedAt, lease.File)
	return true
}

func publicSupportedLocales(view PublicView) []string {
	if len(view.SupportedLocales) > 0 {
		return view.SupportedLocales
	}
	locale := view.Language
	if locale == "" {
		locale = "en-US"
	}
	return []string{locale}
}

func localizedArtifactName(locale, name string) string {
	return name + ".lang-" + hashBytes([]byte(locale))[:16]
}
func fallbackLocalizedArtifactName(locale, name string) string {
	return name + ".fallback-lang-" + hashBytes([]byte(locale))[:16]
}

func (e *Engine) ServeAsset(w http.ResponseWriter, r *http.Request, assetID string) bool {
	e.gate.RLock()
	if e.index == nil || e.index.unhealthy != nil {
		e.gate.RUnlock()
		http.Error(w, "public assets temporarily unavailable", http.StatusServiceUnavailable)
		return true
	}
	target, exists := e.index.byAsset[assetID]
	if !exists {
		e.gate.RUnlock()
		return false
	}
	file, err := os.Open(target.path)
	e.gate.RUnlock()
	if err != nil {
		http.Error(w, "public asset temporarily unavailable", http.StatusServiceUnavailable)
		return true
	}
	defer file.Close()

	etag := `"a-` + target.asset.Checksum + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("Content-Type", target.asset.MIMEType)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	http.ServeContent(w, r, target.asset.ID, time.Time{}, file)
	return true
}

func (e *Engine) ServeSiteCustomCSS(w http.ResponseWriter, r *http.Request) bool {
	e.gate.RLock()
	if e.index == nil || e.index.unhealthy != nil || e.index.customCSSDisabled {
		e.gate.RUnlock()
		return false
	}
	body := []byte(e.index.site.CustomStylesheet())
	if len(body) == 0 {
		e.gate.RUnlock()
		return false
	}
	epoch := e.index.siteEpoch
	generation := e.index.runtimeGeneration
	e.gate.RUnlock()
	etag := fmt.Sprintf(`"css-%d-%d-%s"`, epoch, generation, hashBytes(body)[:24])
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	_, _ = w.Write(body)
	return true
}

func (e *Engine) ServeMachinePath(w http.ResponseWriter, r *http.Request) bool {
	e.gate.RLock()
	if e.index == nil || e.index.unhealthy != nil {
		e.gate.RUnlock()
		http.Error(w, "public machine output temporarily unavailable", http.StatusServiceUnavailable)
		return true
	}
	representation, exists := e.index.machine[r.URL.Path]
	e.gate.RUnlock()
	if !exists {
		return false
	}
	representation.Serve(w, r, r.URL.Path)
	return true
}

func (e *Engine) Views() ([]PublicView, error) {
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return nil, ErrPublicUnavailable
	}
	views := make([]PublicView, len(e.index.views))
	for index, view := range e.index.views {
		views[index] = e.admittedViewLocked(view)
	}
	return views, nil
}

func (e *Engine) admittedViewLocked(view PublicView) PublicView {
	allowed := make(map[string]struct{}, len(view.SupportedLocales))
	for _, locale := range view.SupportedLocales {
		allowed[locale] = struct{}{}
	}
	filtered := make(map[string]LocalizedContent)
	for locale, item := range view.Localizations {
		if _, ok := allowed[locale]; ok {
			filtered[locale] = item
		}
	}
	view.Localizations = filtered
	return view
}

func (e *Engine) PublicSite() (site.Configuration, int64, error) {
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return site.Configuration{}, 0, ErrPublicUnavailable
	}
	return e.index.site, e.index.siteEpoch, nil
}

func (e *Engine) Search(ctx context.Context, query string) ([]PublicView, error) {
	folded := catalog.FoldSearch(query)
	if folded == "" {
		return e.Views()
	}
	candidates, err := e.repository.SearchPublications(ctx, folded, catalog.SearchProjectionVersion)
	if err != nil {
		return nil, err
	}
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return nil, ErrPublicUnavailable
	}
	views := make([]PublicView, 0, len(candidates))
	for _, candidate := range candidates {
		current, exists := e.index.byID[candidate.ProductID]
		if !exists || current.SourceRevision != candidate.SourceRevision || current.SiteEpoch != candidate.SiteEpoch || current.VisibilityGeneration != candidate.VisibilityGeneration {
			continue
		}
		haystack := catalog.FoldSearch(strings.Join([]string{
			current.View.PartNumber, current.View.Name, current.View.Manufacturer, current.View.Brand,
			current.View.Category, current.View.Lifecycle, applicationNames(current.View.Applications),
		}, " "))
		admittedView := e.admittedViewLocked(current.View)
		for _, localized := range admittedView.Localizations {
			haystack += " " + catalog.FoldSearch(localized.Name)
		}
		if strings.Contains(haystack, folded) {
			views = append(views, admittedView)
		}
	}
	return views, nil
}

func applicationNames(applications []Application) string {
	names := make([]string, 0, len(applications))
	for _, application := range applications {
		names = append(names, application.Name)
	}
	return strings.Join(names, " ")
}

func (e *Engine) Product(productID string) (PublicView, bool) {
	e.gate.RLock()
	defer e.gate.RUnlock()
	if e.index == nil || e.index.unhealthy != nil {
		return PublicView{}, false
	}
	activation, exists := e.index.byID[productID]
	return e.admittedViewLocked(activation.View), exists
}

func writeDurable(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func randomArtifactID() (string, error) {
	var raw [16]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", err
	}
	return "art_" + hex.EncodeToString(raw[:]), nil
}

func hashBytes(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

func etagMatches(header, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == current {
			return true
		}
	}
	return false
}
