package webapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"prods/internal/brandcapture"
	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/localization"
	"prods/internal/maildelivery"
	"prods/internal/platform"
	"prods/internal/publishing"
	"prods/internal/recovery"
	"prods/internal/searchnotify"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

//go:embed templates/*.tmpl static/**
var content embed.FS

// SystemStaticFS exposes only the self-contained system UI bundle. Recovery
// mode uses it without opening the normal database or the rest of webapp's
// embedded Admin/Public assets.
func SystemStaticFS() (fs.FS, error) {
	return fs.Sub(content, "static/system")
}

type Config struct {
	IndexNowSubmitter          searchnotify.IndexNowSubmitter
	GoogleSearchSubmitter      searchnotify.GoogleSubmitter
	PublicDir                  string
	AssetDir                   string
	DatabasePath               string
	BackupDir                  string
	BaseURL                    string
	WorkDir                    string
	AdminToken                 string
	EnablePOCAdmin             bool
	SecureCookies              bool
	EnforceHost                bool
	SessionTTL                 time.Duration
	ResourceGate               *platform.ResourceGate
	EnableBackupScheduler      bool
	EnableAssetGC              bool
	AssetGCGrace               time.Duration
	AssetGCInterval            time.Duration
	TrustedProxyCIDRs          []string
	BackupRoots                map[string]string
	BackupFiles                map[string]string
	ApplicationVersion         string
	MailSender                 maildelivery.Sender
	BackupExternalRequirements []string
	RuntimeLogPath             string
	RuntimeLogFiles            int
	BrandCapturer              BrandCapturer
}

type BrandCapturer interface {
	Capture(context.Context, string) (brandcapture.Candidate, error)
}

type session struct {
	CSRF         string
	Admin        bool
	UserID       string
	Capabilities map[identity.Capability]bool
}

func (s session) can(capability identity.Capability) bool {
	return s.Admin && s.Capabilities[capability]
}

type loginAttempt struct {
	Failures     int
	BlockedUntil time.Time
}

type Server struct {
	searchManager  *searchnotify.Manager
	publisher      *publishing.Engine
	backupManager  *recovery.BackupManager
	assetGC        *recovery.AssetGCManager
	mailManager    *maildelivery.Manager
	brandCapturer  BrandCapturer
	store          *sqlite.Store
	config         Config
	templates      *template.Template
	mux            *http.ServeMux
	sessionsMu     sync.Mutex
	sessions       map[string]session
	loginMu        sync.Mutex
	loginAttempts  map[string]loginAttempt
	rfqLimiter     *rfqRateLimiter
	proxyTrust     proxyTrust
	expectedHost   string
	expectedOrigin string
	importMu       sync.Mutex
	importCancels  map[string]func()
	importWG       sync.WaitGroup
	closing        bool
	draining       atomic.Bool
	maintenanceMu  sync.RWMutex
	maintenance    atomic.Pointer[site.Maintenance]
	closeOnce      sync.Once
}

func New(store *sqlite.Store, config Config) (*Server, string, error) {
	generated := ""
	if config.SessionTTL <= 0 {
		config.SessionTTL = 12 * time.Hour
	}
	if config.WorkDir == "" {
		config.WorkDir = filepath.Join(os.TempDir(), "prods-work")
	}
	if config.AssetDir == "" {
		config.AssetDir = filepath.Join(filepath.Dir(config.WorkDir), "assets")
	}
	if config.DatabasePath == "" {
		config.DatabasePath = filepath.Join(filepath.Dir(config.WorkDir), "prods.db")
	}
	if config.BackupDir == "" {
		config.BackupDir = filepath.Join(filepath.Dir(config.WorkDir), "backups")
	}
	if config.ResourceGate == nil {
		config.ResourceGate = platform.NewResourceGate(nil)
	}
	if config.RuntimeLogFiles <= 0 {
		config.RuntimeLogFiles = platform.DefaultRuntimeLogFiles
	}
	if config.BrandCapturer == nil {
		config.BrandCapturer = brandcapture.NewFetcher()
	}
	if !config.EnablePOCAdmin && config.AdminToken != "" {
		return nil, "", errors.New("temporary Admin token requires explicit PoC mode")
	}
	if config.EnablePOCAdmin && config.AdminToken == "" {
		token, err := secureToken(24)
		if err != nil {
			return nil, "", err
		}
		config.AdminToken = token
		generated = token
	}
	expectedHost, expectedOrigin, err := requestAuthority(config.BaseURL)
	if err != nil {
		return nil, "", err
	}
	proxyPolicy, err := newProxyTrust(config.TrustedProxyCIDRs)
	if err != nil {
		return nil, "", err
	}
	tmpl, err := template.New("pages").Funcs(template.FuncMap{"urlquery": url.QueryEscape}).ParseFS(content, "templates/*.tmpl")
	if err != nil {
		return nil, "", fmt.Errorf("parse templates: %w", err)
	}
	static, err := fs.Sub(content, "static")
	if err != nil {
		return nil, "", err
	}
	if !config.EnablePOCAdmin && config.PublicDir == "" {
		config.PublicDir = filepath.Join(config.WorkDir, "public")
	}
	maintenance, err := store.SiteMaintenance(context.Background())
	if err != nil {
		return nil, "", fmt.Errorf("load site maintenance state: %w", err)
	}
	server := &Server{
		store: store, config: config, templates: tmpl, mux: http.NewServeMux(), brandCapturer: config.BrandCapturer,
		sessions: make(map[string]session), loginAttempts: make(map[string]loginAttempt),
		rfqLimiter: newRFQRateLimiter(), proxyTrust: proxyPolicy,
		expectedHost: expectedHost, expectedOrigin: expectedOrigin,
		importCancels: make(map[string]func()),
	}
	server.maintenance.Store(&maintenance)
	if err := store.ReconcileImportJobs(context.Background()); err != nil {
		return nil, "", fmt.Errorf("reconcile import jobs: %w", err)
	}
	if !config.EnablePOCAdmin {
		server.publisher, err = publishing.NewEngine(context.Background(), store, publishing.Config{
			Root: config.PublicDir, AssetRoot: config.AssetDir, BaseURL: config.BaseURL,
			ResourceGate: config.ResourceGate, ByteHeadroom: resourceByteHeadroom,
			InodeHeadroom: resourceInodeHeadroom, MinFreePercent: resourceMinFreePercent,
		})
		if err != nil {
			return nil, "", fmt.Errorf("initialize public architecture: %w", err)
		}
		server.searchManager, err = searchnotify.NewManager(context.Background(), store, searchnotify.Config{
			BaseURL: config.BaseURL, IndexNow: config.IndexNowSubmitter, Google: config.GoogleSearchSubmitter,
		})
		if err != nil {
			server.publisher.Close()
			return nil, "", fmt.Errorf("initialize search integrations: %w", err)
		}
	}
	if config.EnableBackupScheduler && !config.EnablePOCAdmin {
		server.backupManager, err = recovery.NewBackupManager(store, recovery.BackupManagerConfig{
			DatabasePath: config.DatabasePath, BackupDir: config.BackupDir, AssetDir: config.AssetDir,
			Roots: config.BackupRoots, Files: config.BackupFiles, ApplicationVersion: config.ApplicationVersion,
			ExternalRequirements: config.BackupExternalRequirements, ResourceGate: config.ResourceGate,
			ByteHeadroom: resourceByteHeadroom, InodeHeadroom: resourceInodeHeadroom, MinFreePercent: resourceMinFreePercent,
		})
		if err != nil {
			if server.searchManager != nil {
				server.searchManager.Close()
			}
			if server.publisher != nil {
				server.publisher.Close()
			}
			return nil, "", fmt.Errorf("initialize backup manager: %w", err)
		}
	}
	if config.EnableAssetGC && !config.EnablePOCAdmin {
		server.assetGC, err = recovery.NewAssetGCManager(context.Background(), store, recovery.AssetGCConfig{
			AssetDir: config.AssetDir,
			Grace:    config.AssetGCGrace,
			Interval: config.AssetGCInterval,
		})
		if err != nil {
			if server.searchManager != nil {
				server.searchManager.Close()
			}
			if server.backupManager != nil {
				server.backupManager.Close()
			}
			if server.publisher != nil {
				server.publisher.Close()
			}
			return nil, "", fmt.Errorf("initialize asset garbage collector: %w", err)
		}
		server.assetGC.Start(context.Background())
	}
	if server.backupManager != nil {
		server.backupManager.Start(context.Background())
	}
	if config.MailSender != nil {
		server.mailManager, err = maildelivery.NewManager(store, config.MailSender)
		if err != nil {
			if server.searchManager != nil {
				server.searchManager.Close()
			}
			if server.assetGC != nil {
				server.assetGC.Close()
			}
			if server.backupManager != nil {
				server.backupManager.Close()
			}
			if server.publisher != nil {
				server.publisher.Close()
			}
			return nil, "", fmt.Errorf("initialize SMTP delivery manager: %w", err)
		}
	}
	server.routes(static)
	return server, generated, nil
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	s.BeginDrain()
	s.closeOnce.Do(func() {
		s.importWG.Wait()
		if s.searchManager != nil {
			s.searchManager.Close()
		}
		if s.mailManager != nil {
			s.mailManager.Close()
		}
		if s.backupManager != nil {
			s.backupManager.Close()
		}
		if s.assetGC != nil {
			s.assetGC.Close()
		}
		if s.publisher != nil {
			s.publisher.Close()
		}
	})
}

// BeginDrain is the request-admission boundary for process shutdown. Requests
// already inside a handler may complete; new business requests are rejected.
func (s *Server) BeginDrain() {
	if s != nil {
		s.draining.Store(true)
		s.importMu.Lock()
		s.closing = true
		for _, cancel := range s.importCancels {
			cancel()
		}
		s.importMu.Unlock()
		if s.publisher != nil {
			s.publisher.BeginDrain()
		}
		if s.searchManager != nil {
			s.searchManager.BeginDrain()
		}
		if s.backupManager != nil {
			s.backupManager.BeginDrain()
		}
		if s.assetGC != nil {
			s.assetGC.BeginDrain()
		}
		if s.mailManager != nil {
			s.mailManager.BeginDrain()
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if s.draining.Load() && r.URL.Path != "/health/live" {
		http.Error(w, "service is draining", http.StatusServiceUnavailable)
		return
	}
	if s.config.EnforceHost && !strings.EqualFold(r.Host, s.expectedHost) {
		http.Error(w, "unrecognized Host", http.StatusMisdirectedRequest)
		return
	}
	if s.config.EnforceHost && isStateChanging(r.Method) {
		if origin := r.Header.Get("Origin"); origin != "" && !strings.EqualFold(origin, s.expectedOrigin) {
			http.Error(w, "unrecognized Origin", http.StatusForbidden)
			return
		}
	}
	maintenanceProtected := isMaintenanceProtectedPath(r.URL.Path)
	if maintenanceProtected && isStateChanging(r.Method) {
		s.maintenanceMu.RLock()
		if maintenance := s.maintenanceSnapshot(); maintenance.Active {
			s.maintenanceMu.RUnlock()
			s.writeMaintenanceResponse(w, r, maintenance)
			return
		}
		defer s.maintenanceMu.RUnlock()
	} else if maintenanceProtected {
		if maintenance := s.maintenanceSnapshot(); maintenance.Active {
			s.writeMaintenanceResponse(w, r, maintenance)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
	if s.publisher != nil && isStateChanging(r.Method) && strings.HasPrefix(r.URL.Path, "/admin/") && r.URL.Path != "/admin/api/website/capture" {
		s.publisher.Wake()
	}
	if s.searchManager != nil && isStateChanging(r.Method) && strings.HasPrefix(r.URL.Path, "/admin/") {
		s.searchManager.Wake()
	}
}

func requestAuthority(rawBaseURL string) (string, string, error) {
	location, err := url.Parse(rawBaseURL)
	if err != nil || location.Host == "" || (location.Scheme != "http" && location.Scheme != "https") {
		return "", "", fmt.Errorf("invalid base URL %q", rawBaseURL)
	}
	return location.Host, location.Scheme + "://" + location.Host, nil
}

func isStateChanging(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func (s *Server) routes(static fs.FS) {
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /maintenance/api/state", s.siteMaintenanceState)
	s.mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("live\n"))
	})
	s.mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err := s.store.Ready(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready\n"))
			return
		}
		reservation, err := s.admitResource(r.Context(), "readiness RFQ capacity", s.config.DatabasePath, 1<<20, 0)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready\n"))
			return
		}
		reservation.Release()
		_, _ = w.Write([]byte("ready\n"))
	})
	s.mux.HandleFunc("GET /{$}", s.search)
	s.mux.HandleFunc("GET /search", s.search)
	s.mux.HandleFunc("GET /api/locales", s.publicLocales)
	s.mux.HandleFunc("GET /products/{artifact}", s.product)
	s.mux.HandleFunc("GET /assets/{id}", s.publicAsset)
	s.mux.HandleFunc("GET /categories/{path...}", s.publicCategory)
	s.mux.HandleFunc("GET /manufacturers/{slug}", s.publicManufacturer)
	s.mux.HandleFunc("GET /brands/{slug}", s.publicBrand)
	s.mux.HandleFunc("GET /applications/{slug}", s.publicApplication)
	s.mux.HandleFunc("GET /sitemap.xml", s.publicMachine)
	s.mux.HandleFunc("GET /robots.txt", s.publicMachine)
	s.mux.HandleFunc("GET /llms.txt", s.publicMachine)
	s.mux.HandleFunc("GET /catalog/{artifact}", s.publicMachine)
	s.mux.HandleFunc("GET /site.css", s.publicSiteCustomCSS)
	s.mux.HandleFunc("GET /rfq", s.rfqForm)
	s.mux.HandleFunc("POST /rfq", s.rfqHTML)
	s.mux.HandleFunc("POST /api/rfqs", s.rfqJSON)
	s.mux.HandleFunc("GET /admin/login", s.adminLogin)
	s.mux.HandleFunc("POST /admin/login", s.adminLogin)
	s.mux.HandleFunc("GET /set-password", s.setPassword)
	s.mux.HandleFunc("POST /set-password", s.setPassword)
	s.mux.HandleFunc("POST /admin/logout", s.adminLogout)
	s.mux.HandleFunc("GET /admin", s.admin)
	s.mux.HandleFunc("POST /admin/api/previews/products", s.adminCreateProductPreview)
	s.mux.HandleFunc("GET /admin/previews/{token}", s.adminProductPreview)
	s.mux.HandleFunc("GET /admin/api/rfqs", s.adminRFQs)
	s.mux.HandleFunc("GET /admin/api/rfq-recipient-users", s.adminRFQRecipientUsers)
	s.mux.HandleFunc("GET /admin/api/rfq-recipient-settings", s.adminRFQRecipientSettings)
	s.mux.HandleFunc("PUT /admin/api/rfq-recipient-settings", s.adminUpdateRFQRecipientSettings)
	s.mux.HandleFunc("PUT /admin/api/rfqs/{id}/status", s.adminUpdateRFQStatus)
	s.mux.HandleFunc("PUT /admin/api/rfqs/{id}/recipients", s.adminReplaceRFQRecipients)
	s.mux.HandleFunc("POST /admin/api/rfqs/{id}/anonymize", s.adminAnonymizeRFQ)
	s.mux.HandleFunc("GET /admin/api/rfqs/{id}/deliveries", s.adminRFQDeliveries)
	s.mux.HandleFunc("POST /admin/api/rfqs/{id}/deliveries", s.adminCreateRFQDelivery)
	s.mux.HandleFunc("POST /admin/api/exports/products.xlsx", s.adminExportProducts)
	s.mux.HandleFunc("POST /admin/api/exports/rfqs.csv", s.adminExportRFQs)
	s.mux.HandleFunc("GET /admin/api/system/health", s.adminSystemHealth)
	s.mux.HandleFunc("GET /admin/api/system/runtime-log", s.adminRuntimeLog)
	s.mux.HandleFunc("GET /admin/api/system/maintenance", s.adminSiteMaintenance)
	s.mux.HandleFunc("PUT /admin/api/system/maintenance", s.adminUpdateSiteMaintenance)
	s.mux.HandleFunc("GET /admin/api/jobs", s.adminJobs)
	s.mux.HandleFunc("POST /admin/api/jobs/publication/{id}/retry", s.adminRetryPublicationJob)
	s.mux.HandleFunc("POST /admin/api/jobs/search/{id}/retry", s.adminRetrySearchSubmission)
	s.mux.HandleFunc("GET /admin/api/search-integrations", s.adminSearchIntegrations)
	s.mux.HandleFunc("PUT /admin/api/search-integrations", s.adminUpdateSearchIntegrations)
	s.mux.HandleFunc("GET /admin/api/backups", s.adminBackupStatus)
	s.mux.HandleFunc("PUT /admin/api/backups/settings", s.adminUpdateBackupSettings)
	s.mux.HandleFunc("POST /admin/api/backups/run", s.adminRunBackup)
	s.mux.HandleFunc("GET /admin/api/traffic-settings", s.adminTrafficSettings)
	s.mux.HandleFunc("PUT /admin/api/traffic-settings", s.adminUpdateTrafficSettings)
	s.mux.HandleFunc("GET /admin/api/audit", s.adminAudit)
	s.mux.HandleFunc("GET /admin/api/products", s.adminProducts)
	s.mux.HandleFunc("POST /admin/api/products", s.adminCreateProduct)
	s.mux.HandleFunc("GET /admin/api/products/{id}", s.adminProduct)
	s.mux.HandleFunc("PUT /admin/api/products/{id}", s.adminUpdateProduct)
	s.mux.HandleFunc("POST /admin/api/products/{id}/url", s.adminUpdateProductURL)
	s.mux.HandleFunc("POST /admin/api/products/{id}/hide", s.adminHideProduct)
	s.mux.HandleFunc("POST /admin/api/products/{id}/archive", s.adminArchiveProduct)
	s.mux.HandleFunc("POST /admin/api/products/{id}/clone", s.adminCloneProduct)
	s.mux.HandleFunc("POST /admin/api/categories", s.adminCreateCategory)
	s.mux.HandleFunc("GET /admin/api/categories", s.adminCategories)
	s.mux.HandleFunc("POST /admin/api/categories/{id}/move", s.adminMoveCategory)
	s.mux.HandleFunc("POST /admin/api/categories/{id}/update-preview", s.adminPreviewCategoryUpdate)
	s.mux.HandleFunc("POST /admin/api/categories/{id}/update", s.adminUpdateCategory)
	s.mux.HandleFunc("POST /admin/api/categories/{id}/disable", s.adminDisableCategory)
	s.mux.HandleFunc("POST /admin/api/dictionaries", s.adminCreateDictionary)
	s.mux.HandleFunc("GET /admin/api/dictionaries", s.adminDictionaries)
	s.mux.HandleFunc("POST /admin/api/dictionaries/{id}/disable", s.adminDisableDictionary)
	s.mux.HandleFunc("POST /admin/api/dictionaries/{id}/update-preview", s.adminPreviewDictionaryUpdate)
	s.mux.HandleFunc("POST /admin/api/dictionaries/{id}/update", s.adminUpdateDictionary)
	s.mux.HandleFunc("POST /admin/api/specs", s.adminCreateSpec)
	s.mux.HandleFunc("GET /admin/api/specs", s.adminSpecs)
	s.mux.HandleFunc("GET /admin/api/spec-sets", s.adminSpecSets)
	s.mux.HandleFunc("GET /admin/api/roles", s.adminRoles)
	s.mux.HandleFunc("POST /admin/api/roles", s.adminCreateRole)
	s.mux.HandleFunc("GET /admin/api/users", s.adminUsers)
	s.mux.HandleFunc("POST /admin/api/users", s.adminCreateUser)
	s.mux.HandleFunc("POST /admin/api/users/{id}/disable", s.adminDisableUser)
	s.mux.HandleFunc("POST /admin/api/spec-sets", s.adminCreateSpecSet)
	s.mux.HandleFunc("GET /admin/api/categories/{id}/spec-set", s.adminCategorySpecSet)
	s.mux.HandleFunc("POST /admin/api/categories/{id}/spec-set", s.adminSetCategorySpecSet)
	s.mux.HandleFunc("GET /admin/api/products/{id}/spec-values", s.adminProductSpecValues)
	s.mux.HandleFunc("POST /admin/api/products/{id}/spec-values", s.adminSaveSpecValue)
	s.mux.HandleFunc("POST /admin/api/spec-values/{id}/normalized", s.adminSaveNormalizedValue)
	s.mux.HandleFunc("GET /admin/api/products/{id}/documents", s.adminProductDocuments)
	s.mux.HandleFunc("POST /admin/api/products/{id}/documents", s.adminAddProductDocument)
	s.mux.HandleFunc("POST /admin/api/products/{id}/assets", s.adminUploadProductAsset)
	s.mux.HandleFunc("GET /admin/api/products/{id}/images", s.adminProductImages)
	s.mux.HandleFunc("POST /admin/api/products/{id}/images", s.adminAddProductImage)
	s.mux.HandleFunc("PUT /admin/api/products/{id}/images/{image_id}", s.adminUpdateProductImage)
	s.mux.HandleFunc("POST /admin/api/products/{id}/images/{image_id}/delete", s.adminDeleteProductImage)
	s.mux.HandleFunc("GET /admin/api/website/routes", s.adminSiteRoutes)
	s.mux.HandleFunc("POST /admin/api/website/routes/preview", s.adminPreviewSiteRoutes)
	s.mux.HandleFunc("POST /admin/api/website/routes/publish", s.adminPublishSiteRoutes)
	s.mux.HandleFunc("GET /admin/api/website/configuration", s.adminWebsiteConfiguration)
	s.mux.HandleFunc("POST /admin/api/website/capture", s.adminCaptureWebsiteBrand)
	s.mux.HandleFunc("PUT /admin/api/website/configuration", s.adminSaveWebsiteConfiguration)
	s.mux.HandleFunc("GET /admin/api/website/versions", s.adminWebsiteVersions)
	s.mux.HandleFunc("POST /admin/api/website/versions/restore", s.adminRestoreWebsiteVersion)
	s.mux.HandleFunc("POST /admin/api/website/preview", s.adminCreateWebsitePreview)
	s.mux.HandleFunc("POST /admin/api/website/assets", s.adminUploadWebsiteAsset)
	s.mux.HandleFunc("POST /admin/api/website/custom-css", s.adminSetWebsiteCustomCSSState)
	s.mux.HandleFunc("GET /admin/previews/website-assets/{id}", s.adminWebsiteAsset)
	s.mux.HandleFunc("GET /admin/previews/website/{token}", s.adminWebsitePreview)
	s.mux.HandleFunc("POST /admin/api/imports", s.adminCreateImportJob)
	s.mux.HandleFunc("GET /admin/api/imports/{id}", s.adminImportJob)
	s.mux.HandleFunc("GET /admin/api/imports/{id}/report", s.adminImportReport)
	s.mux.HandleFunc("POST /admin/api/imports/{id}/commit", s.adminCommitImport)
	s.mux.HandleFunc("POST /admin/api/imports/{id}/cancel", s.adminCancelImport)
	s.mux.HandleFunc("POST /admin/api/import-templates", s.adminSaveImportTemplate)
	s.mux.HandleFunc("GET /admin/api/import-templates/{id}", s.adminImportTemplate)
	s.mux.HandleFunc("GET /{path...}", s.publicGenerated)
}

type publicPage struct {
	Site            site.Configuration
	SiteEpoch       int64
	Language        string
	Navigation      []site.NavigationItem
	Stylesheet      template.CSS
	Text            localization.Messages
	CatalogURL      string
	RFQURL          string
	JSONURL         string
	MarkdownURL     string
	LanguageOptions []publicLanguageOption
}

type publicLanguageOption struct {
	Locale string
	URL    string
	Active bool
}

func (s *Server) currentPublicPage(r *http.Request, views []publishing.PublicView) (publicPage, error) {
	ctx := r.Context()
	var configuration site.Configuration
	var epoch int64
	var defaultLocale string
	var supportedLocales []string
	if len(views) > 0 {
		configuration = views[0].Site
		epoch = views[0].SiteEpoch
		defaultLocale = views[0].Language
		supportedLocales = append([]string(nil), views[0].SupportedLocales...)
	} else {
		var err error
		if s.publisher != nil {
			configuration, epoch, err = s.publisher.PublicSite()
		} else {
			configuration, _, epoch, err = s.store.ActiveWebsiteConfiguration(ctx)
		}
		if err != nil {
			if !s.config.EnablePOCAdmin {
				return publicPage{}, err
			}
			configuration = site.DefaultConfiguration()
			defaultLocale = "en-US"
			supportedLocales = []string{"en-US"}
			epoch = 1
		} else {
			defaultLocale, supportedLocales, err = s.store.SiteLocales(ctx)
			if err != nil {
				if !s.config.EnablePOCAdmin {
					return publicPage{}, err
				}
				defaultLocale = "en-US"
				supportedLocales = []string{"en-US"}
			}
		}
	}
	if len(supportedLocales) == 0 {
		supportedLocales = []string{defaultLocale}
	}
	language, err := localization.Resolve(defaultLocale, supportedLocales, r.URL.Query().Get("lang"), "")
	if err != nil {
		return publicPage{}, err
	}
	if err := configuration.Prepare(); err != nil {
		return publicPage{}, err
	}
	page := publicPage{
		Site: configuration, SiteEpoch: epoch, Language: language,
		Navigation: configuration.VisibleNavigation(), Stylesheet: template.CSS(configuration.Stylesheet()),
		Text: localization.For(language), CatalogURL: withLanguage("/search", language), RFQURL: withLanguage("/rfq", language),
	}
	for _, locale := range supportedLocales {
		page.LanguageOptions = append(page.LanguageOptions, publicLanguageOption{Locale: locale, URL: withLanguage(r.URL.RequestURI(), locale), Active: locale == language})
	}
	return page, nil
}

func withLanguage(rawURL, locale string) string {
	location, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := location.Query()
	query.Set("lang", locale)
	location.RawQuery = query.Encode()
	return location.String()
}

func (s *Server) publicLocales(w http.ResponseWriter, r *http.Request) {
	defaultLocale, supported, err := s.store.SiteLocales(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	current, err := localization.Resolve(defaultLocale, supported, r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"default": defaultLocale, "supported": supported, "current": current})
}

type searchPage struct {
	publicPage
	Title               string
	Query               string
	Products            []publishing.PublicView
	NoResults           bool
	PreviousURL         string
	NextURL             string
	RequestURL          string
	ManufacturerID      string
	BrandID             string
	CategoryID          string
	ManufacturerOptions []searchFilterOption
	BrandOptions        []searchFilterOption
	CategoryOptions     []searchFilterOption
}

type publicSearchFilters struct {
	ManufacturerID string
	BrandID        string
	CategoryID     string
}

type searchFilterOption struct {
	Value    string
	Label    string
	Selected bool
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	filters := publicSearchFilters{
		ManufacturerID: strings.TrimSpace(r.URL.Query().Get("manufacturer_id")),
		BrandID:        strings.TrimSpace(r.URL.Query().Get("brand_id")),
		CategoryID:     strings.TrimSpace(r.URL.Query().Get("category_id")),
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 3
	allViews, err := s.publicViews(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	views := allViews
	if query != "" && s.publisher != nil {
		views, err = s.publisher.Search(r.Context(), query)
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	if query != "" && s.publisher == nil {
		folded := catalog.FoldSearch(query)
		filtered := make([]publishing.PublicView, 0, len(views))
		for _, view := range views {
			haystack := catalog.FoldSearch(strings.Join([]string{
				view.PartNumber, view.Name, view.Manufacturer, view.Brand, view.Category, view.Lifecycle, publicApplicationNames(view.Applications),
			}, " "))
			if strings.Contains(haystack, folded) {
				filtered = append(filtered, view)
			}
		}
		views = filtered
	}
	views = filterPublicViews(views, filters)
	manufacturerOptions, brandOptions, categoryOptions := publicSearchFilterOptions(allViews, filters)
	total := len(views)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	presentation, err := s.currentPublicPage(r, views)
	if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	w.Header().Set("Content-Language", presentation.Language)
	displayProducts := append([]publishing.PublicView(nil), views[start:end]...)
	for index := range displayProducts {
		displayProducts[index].CanonicalURL = withLanguage(displayProducts[index].CanonicalURL, presentation.Language)
	}
	data := searchPage{
		publicPage: presentation, Title: presentation.Text.Catalog, Query: query, Products: displayProducts,
		NoResults: query != "" && total == 0, RequestURL: withLanguage("/rfq?query="+url.QueryEscape(query), presentation.Language),
		ManufacturerID: filters.ManufacturerID, BrandID: filters.BrandID, CategoryID: filters.CategoryID,
		ManufacturerOptions: manufacturerOptions, BrandOptions: brandOptions, CategoryOptions: categoryOptions,
	}
	if page > 1 {
		data.PreviousURL = searchURL(query, page-1, presentation.Language, filters)
	}
	if page*pageSize < total {
		data.NextURL = searchURL(query, page+1, presentation.Language, filters)
	}
	s.render(w, "search", data)
}

func filterPublicViews(views []publishing.PublicView, filters publicSearchFilters) []publishing.PublicView {
	if filters.ManufacturerID == "" && filters.BrandID == "" && filters.CategoryID == "" {
		return views
	}
	filtered := make([]publishing.PublicView, 0, len(views))
	for _, view := range views {
		if filters.ManufacturerID != "" && view.ManufacturerID != filters.ManufacturerID {
			continue
		}
		if filters.BrandID != "" && view.BrandID != filters.BrandID {
			continue
		}
		if filters.CategoryID != "" && !publicViewHasCategory(view, filters.CategoryID) {
			continue
		}
		filtered = append(filtered, view)
	}
	return filtered
}

func publicViewHasCategory(view publishing.PublicView, categoryID string) bool {
	for _, category := range view.CategoryTrail {
		if category.ID == categoryID {
			return true
		}
	}
	return len(view.CategoryTrail) == 0 && view.CategoryID == categoryID
}

func publicSearchFilterOptions(views []publishing.PublicView, selected publicSearchFilters) ([]searchFilterOption, []searchFilterOption, []searchFilterOption) {
	manufacturers := make(map[string]string)
	brands := make(map[string]string)
	categories := make(map[string]string)
	for _, view := range views {
		if view.ManufacturerID != "" && view.Manufacturer != "" {
			manufacturers[view.ManufacturerID] = view.Manufacturer
		}
		if view.BrandID != "" && view.Brand != "" {
			brands[view.BrandID] = view.Brand
		}
		if len(view.CategoryTrail) == 0 && view.CategoryID != "" && view.Category != "" {
			categories[view.CategoryID] = view.Category
		}
		for _, category := range view.CategoryTrail {
			if category.ID != "" && category.Name != "" {
				categories[category.ID] = category.Name
			}
		}
	}
	return sortedSearchFilterOptions(manufacturers, selected.ManufacturerID),
		sortedSearchFilterOptions(brands, selected.BrandID),
		sortedSearchFilterOptions(categories, selected.CategoryID)
}

func sortedSearchFilterOptions(values map[string]string, selected string) []searchFilterOption {
	options := make([]searchFilterOption, 0, len(values))
	for value, label := range values {
		options = append(options, searchFilterOption{Value: value, Label: label, Selected: value == selected})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Label == options[j].Label {
			return options[i].Value < options[j].Value
		}
		return options[i].Label < options[j].Label
	})
	return options
}

func (s *Server) publicCategory(w http.ResponseWriter, r *http.Request) {
	s.publicAggregate(w, r, "category")
}

func (s *Server) publicManufacturer(w http.ResponseWriter, r *http.Request) {
	s.publicAggregate(w, r, "manufacturer")
}

func (s *Server) publicBrand(w http.ResponseWriter, r *http.Request) {
	s.publicAggregate(w, r, "brand")
}

func (s *Server) publicApplication(w http.ResponseWriter, r *http.Request) {
	s.publicAggregate(w, r, "application")
}

func (s *Server) publicAggregate(w http.ResponseWriter, r *http.Request, kind string) {
	views, err := s.publicViews(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	targetURL := strings.TrimRight(s.config.BaseURL, "/") + r.URL.Path
	filtered := make([]publishing.PublicView, 0)
	title := ""
	for _, view := range views {
		var aggregateURL, aggregateName string
		switch kind {
		case "category":
			for _, category := range view.CategoryTrail {
				if category.URL == targetURL {
					aggregateURL, aggregateName = category.URL, category.Name
					break
				}
			}
			if aggregateURL == "" && view.CategoryURL == targetURL {
				aggregateURL, aggregateName = view.CategoryURL, view.Category
			}
		case "manufacturer":
			aggregateURL, aggregateName = view.ManufacturerURL, view.Manufacturer
		case "brand":
			aggregateURL, aggregateName = view.BrandURL, view.Brand
		case "application":
			for _, application := range view.Applications {
				if application.URL == targetURL {
					aggregateURL, aggregateName = application.URL, application.Name
					break
				}
			}
		default:
			s.internalError(w, errors.New("unknown public aggregate kind"))
			return
		}
		if aggregateURL == targetURL {
			filtered = append(filtered, view)
			if title == "" {
				title = aggregateName
			}
		}
	}
	if len(filtered) == 0 {
		http.NotFound(w, r)
		return
	}
	hasher := sha256.New()
	_, _ = io.WriteString(hasher, targetURL)
	for _, view := range filtered {
		_, _ = fmt.Fprintf(hasher, "\x00%s\x00%d\x00%d", view.ID, view.Revision, view.SiteEpoch)
	}
	etag := `"g-` + hex.EncodeToString(hasher.Sum(nil))[:32] + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	presentation, err := s.currentPublicPage(r, filtered)
	if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	w.Header().Set("Content-Language", presentation.Language)
	displayProducts := append([]publishing.PublicView(nil), filtered...)
	for index := range displayProducts {
		displayProducts[index].CanonicalURL = withLanguage(displayProducts[index].CanonicalURL, presentation.Language)
	}
	s.render(w, "search", searchPage{publicPage: presentation, Title: title, Products: displayProducts})
}

func publicApplicationNames(applications []publishing.Application) string {
	names := make([]string, 0, len(applications))
	for _, application := range applications {
		names = append(names, application.Name)
	}
	return strings.Join(names, " ")
}

func (s *Server) publicViews(ctx context.Context) ([]publishing.PublicView, error) {
	if s.publisher != nil {
		return s.publisher.Views()
	}
	products, _, err := s.store.ListPublished(ctx, "", 1, 1000)
	if err != nil {
		return nil, err
	}
	views := make([]publishing.PublicView, 0, len(products))
	for _, product := range products {
		view, err := publishing.FromProduct(product, s.config.BaseURL)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func searchURL(query string, page int, locale string, filters publicSearchFilters) string {
	values := url.Values{"page": {strconv.Itoa(page)}}
	if query != "" {
		values.Set("q", query)
	}
	values.Set("lang", locale)
	if filters.ManufacturerID != "" {
		values.Set("manufacturer_id", filters.ManufacturerID)
	}
	if filters.BrandID != "" {
		values.Set("brand_id", filters.BrandID)
	}
	if filters.CategoryID != "" {
		values.Set("category_id", filters.CategoryID)
	}
	return "/search?" + values.Encode()
}

type productPage struct {
	publicPage
	Title  string
	View   publishing.PublicView
	JSONLD template.JS
}

func (s *Server) product(w http.ResponseWriter, r *http.Request) {
	if s.publisher != nil {
		if !s.publisher.ServePath(w, r) {
			http.NotFound(w, r)
		}
		return
	}
	artifact := r.PathValue("artifact")
	format := "html"
	id := artifact
	for _, suffix := range []string{".json", ".md"} {
		if strings.HasSuffix(artifact, suffix) {
			format = suffix[1:]
			id = strings.TrimSuffix(artifact, suffix)
			break
		}
	}
	product, err := s.store.PublishedProduct(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	view, err := publishing.FromProduct(product, s.config.BaseURL)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	etag := fmt.Sprintf(`"product-%s-r%d-%s"`, view.ID, view.Revision, format)
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	switch format {
	case "json":
		body, err := publishing.JSON(view)
		if err != nil {
			s.internalError(w, err)
			return
		}
		writeBody(w, "application/json; charset=utf-8", body)
	case "md":
		writeBody(w, "text/markdown; charset=utf-8", publishing.Markdown(view))
	default:
		jsonLD, err := publishing.JSONLD(view)
		if err != nil {
			s.internalError(w, err)
			return
		}
		presentation, err := s.currentPublicPage(r, []publishing.PublicView{view})
		if err != nil {
			s.writePublicPageError(w, err)
			return
		}
		presentation.JSONURL = view.CanonicalURL + ".json"
		presentation.MarkdownURL = view.CanonicalURL + ".md"
		w.Header().Set("Content-Language", presentation.Language)
		s.render(w, "product", productPage{publicPage: presentation, Title: view.PartNumber, View: view, JSONLD: template.JS(jsonLD)})
	}
}

func (s *Server) publicGenerated(w http.ResponseWriter, r *http.Request) {
	if s.serveIndexNowKey(w, r) {
		return
	}
	if s.publisher == nil || !s.publisher.ServePath(w, r) {
		http.NotFound(w, r)
	}
}

func (s *Server) publicMachine(w http.ResponseWriter, r *http.Request) {
	if s.publisher != nil {
		if !s.publisher.ServeMachinePath(w, r) {
			http.NotFound(w, r)
		}
		return
	}
	views, err := s.publicViews(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	configuration, _, siteEpoch, err := s.store.ActiveWebsiteConfiguration(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	outputs, err := publishing.MachineRepresentations(views, configuration, s.config.BaseURL, siteEpoch)
	if err != nil {
		s.internalError(w, err)
		return
	}
	representation, exists := outputs[r.URL.Path]
	if !exists {
		http.NotFound(w, r)
		return
	}
	representation.Serve(w, r, r.URL.Path)
}

type rfqPage struct {
	publicPage
	Title         string
	CSRF          string
	SubmissionKey string
	Product       *publishing.PublicView
	RawQuery      string
	Requested     string
	Name          string
	Email         string
	Quantity      string
	Notes         string
	Error         string
}

func (s *Server) rfqForm(w http.ResponseWriter, r *http.Request) {
	page, err := s.newRFQPage(w, r)
	if err != nil {
		s.writePublicPageError(w, err)
		return
	}
	w.Header().Set("Content-Language", page.Language)
	s.render(w, "rfq", page)
}

func (s *Server) writePublicPageError(w http.ResponseWriter, err error) {
	if errors.Is(err, localization.ErrUnsupportedLocale) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.internalError(w, err)
}

func (s *Server) publicProductView(ctx context.Context, id string) (publishing.PublicView, error) {
	if s.publisher != nil {
		view, found := s.publisher.Product(id)
		if !found {
			return publishing.PublicView{}, sql.ErrNoRows
		}
		return view, nil
	}
	product, err := s.store.PublishedProduct(ctx, id)
	if err != nil {
		return publishing.PublicView{}, err
	}
	return publishing.FromProduct(product, s.config.BaseURL)
}

func (s *Server) newRFQPage(w http.ResponseWriter, r *http.Request) (rfqPage, error) {
	_, current, err := s.ensureSession(w, r, false)
	if err != nil {
		return rfqPage{}, err
	}
	key, err := sqlite.NewKey()
	if err != nil {
		return rfqPage{}, err
	}
	page := rfqPage{Title: "Request for quotation", CSRF: current.CSRF, SubmissionKey: key, RawQuery: r.URL.Query().Get("query"), Requested: r.URL.Query().Get("query")}
	if id := r.URL.Query().Get("product_id"); id != "" {
		view, err := s.publicProductView(r.Context(), id)
		if err != nil {
			return rfqPage{}, err
		}
		page.Product = &view
	}
	var views []publishing.PublicView
	if page.Product != nil {
		views = []publishing.PublicView{*page.Product}
	}
	presentation, err := s.currentPublicPage(r, views)
	if err != nil {
		return rfqPage{}, err
	}
	page.publicPage = presentation
	page.Title = presentation.Text.RFQTitle
	return page, nil
}

func (s *Server) rfqHTML(w http.ResponseWriter, r *http.Request) {
	id, current, ok := s.readSession(r)
	if !ok || !s.validCSRF(r.FormValue("csrf_token"), current.CSRF) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	_ = id
	item := inquiries.Item{Kind: r.FormValue("kind"), ProductID: r.FormValue("product_id"), Requested: r.FormValue("requested"), RawQuery: r.FormValue("raw_query"), Quantity: r.FormValue("quantity"), Notes: r.FormValue("notes")}
	submission := inquiries.Submission{Name: r.FormValue("name"), Email: r.FormValue("email"), Items: []inquiries.Item{item}}
	receipt, err := s.submitPublicRFQ(r, r.FormValue("submission_key"), submission)
	if err != nil {
		status := http.StatusBadRequest
		message := "Could not save the RFQ. Your entered content is preserved."
		if errors.Is(err, platform.ErrResourceCritical) || errors.Is(err, platform.ErrResourceUnavailable) {
			status = http.StatusInsufficientStorage
			message = "Storage is temporarily unavailable. Your entered content is preserved; retry after an administrator resolves system capacity."
		}
		if errors.Is(err, inquiries.ErrIdempotencyConflict) {
			status = http.StatusConflict
			message = "This submission key already completed with different content. Your edits were not overwritten; reload only when you intend to create a new submission."
		}
		var rateLimit rfqRateLimitError
		if errors.As(err, &rateLimit) {
			status = http.StatusTooManyRequests
			message = "Too many RFQ submissions from this connection. Your entered content is preserved; retry after the indicated wait."
			writeRFQRateLimit(w, rateLimit)
		}
		var product *publishing.PublicView
		var views []publishing.PublicView
		if item.Kind == "catalog" && item.ProductID != "" {
			view, productErr := s.publicProductView(r.Context(), item.ProductID)
			if productErr == nil {
				product = &view
				views = []publishing.PublicView{view}
			} else if !errors.Is(productErr, sql.ErrNoRows) {
				s.internalError(w, productErr)
				return
			}
		}
		presentation, presentationErr := s.currentPublicPage(r, views)
		if presentationErr != nil {
			s.writePublicPageError(w, presentationErr)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		s.render(w, "rfq", rfqPage{publicPage: presentation, Title: presentation.Text.RFQTitle, CSRF: current.CSRF, SubmissionKey: r.FormValue("submission_key"), Product: product, RawQuery: item.RawQuery, Requested: item.Requested, Name: submission.Name, Email: submission.Email, Quantity: item.Quantity, Notes: item.Notes, Error: message})
		return
	}
	presentation, err := s.currentPublicPage(r, nil)
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.render(w, "receipt", receiptPage{publicPage: presentation, Title: presentation.Text.RFQReceived, Receipt: receipt})
}

type receiptPage struct {
	publicPage
	Title   string
	Receipt inquiries.Receipt
}

type rfqJSONRequest struct {
	SubmissionKey string               `json:"submission_key"`
	Submission    inquiries.Submission `json:"submission"`
}

func (s *Server) rfqJSON(w http.ResponseWriter, r *http.Request) {
	_, current, ok := s.readSession(r)
	if !ok || !s.validCSRF(r.Header.Get("X-CSRF-Token"), current.CSRF) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	var request rfqJSONRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	receipt, err := s.submitPublicRFQ(r, request.SubmissionKey, request.Submission)
	if errors.Is(err, inquiries.ErrIdempotencyConflict) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	var rateLimit rfqRateLimitError
	if errors.As(err, &rateLimit) {
		writeRFQRateLimit(w, rateLimit)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": rateLimit.Error()})
		return
	}
	if errors.Is(err, platform.ErrResourceCritical) || errors.Is(err, platform.ErrResourceUnavailable) {
		s.writeResourceError(w, err)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	page, err := s.newLoginPage(r)
	if err != nil {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {
		s.render(w, "login", page)
		return
	}
	if s.config.EnablePOCAdmin {
		provided := []byte(r.FormValue("token"))
		expected := []byte(s.config.AdminToken)
		if len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			page.Error = page.Text.InvalidTemporaryToken
			s.render(w, "login", page)
			return
		}
		if _, _, err := s.ensureSession(w, r, true); err != nil {
			s.internalError(w, err)
			return
		}
	} else if retryAfter, allowed := s.loginAllowed(r.FormValue("email"), time.Now().UTC()); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
		w.WriteHeader(http.StatusTooManyRequests)
		page.Email = r.FormValue("email")
		page.Error = page.Text.TooManyAttempts
		s.render(w, "login", page)
		return
	} else if err := s.loginOwner(w, r); err != nil {
		s.recordLoginFailure(r.FormValue("email"), time.Now().UTC())
		w.WriteHeader(http.StatusUnauthorized)
		page.Email = r.FormValue("email")
		page.Error = page.Text.InvalidCredentials
		s.render(w, "login", page)
		return
	} else {
		s.clearLoginFailures(r.FormValue("email"))
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

type loginPage struct {
	Title     string
	Language  string
	ActionURL string
	Text      localization.AuthText
	Email     string
	Error     string
	POC       bool
}

func (s *Server) authText(r *http.Request) (string, localization.AuthText, error) {
	defaultLocale, supported, err := s.store.SiteLocales(r.Context())
	if err != nil {
		return "", localization.AuthText{}, err
	}
	locale, err := localization.Resolve(defaultLocale, supported, r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"))
	if err != nil {
		return "", localization.AuthText{}, err
	}
	return locale, localization.AuthFor(locale), nil
}

func (s *Server) newLoginPage(r *http.Request) (loginPage, error) {
	locale, text, err := s.authText(r)
	if err != nil {
		return loginPage{}, err
	}
	return loginPage{
		Title: text.AdminTitle, Language: locale, ActionURL: withLanguage("/admin/login", locale),
		Text: text, POC: s.config.EnablePOCAdmin,
	}, nil
}

func (s *Server) loginOwner(w http.ResponseWriter, r *http.Request) error {
	user, err := s.store.ActiveUserByEmail(r.Context(), r.FormValue("email"))
	if err != nil {
		// Run the same bounded password KDF for unknown users. The encoded value
		// is syntactically valid but intentionally cannot match a useful password.
		_, _ = identity.VerifyPassword("$pbkdf2-sha256$v=1$i=600000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", r.FormValue("password"))
		return sqlite.ErrInvalidCredentials
	}
	ok, err := identity.VerifyPassword(user.PasswordHash, r.FormValue("password"))
	if err != nil || !ok {
		return sqlite.ErrInvalidCredentials
	}
	rawToken, err := identity.NewToken(32)
	if err != nil {
		return err
	}
	csrfToken, err := identity.NewToken(32)
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(s.config.SessionTTL)
	if err := s.store.CreateAdminSession(r.Context(), rawToken, csrfToken, user, expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: "prods_admin", Value: rawToken, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: s.config.SecureCookies, Expires: expiresAt,
	})
	return nil
}

func (s *Server) loginAllowed(email string, now time.Time) (time.Duration, bool) {
	key := loginKey(email)
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	attempt := s.loginAttempts[key]
	if now.Before(attempt.BlockedUntil) {
		return attempt.BlockedUntil.Sub(now), false
	}
	return 0, true
}

func (s *Server) recordLoginFailure(email string, now time.Time) {
	key := loginKey(email)
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	attempt := s.loginAttempts[key]
	attempt.Failures++
	if attempt.Failures >= 5 {
		exponent := min(attempt.Failures-5, 5)
		attempt.BlockedUntil = now.Add(time.Second * time.Duration(1<<exponent))
	}
	s.loginAttempts[key] = attempt
}

func (s *Server) clearLoginFailures(email string) {
	s.loginMu.Lock()
	delete(s.loginAttempts, loginKey(email))
	s.loginMu.Unlock()
}

func loginKey(email string) string {
	return identity.TokenDigest(identity.NormalizeEmail(email))
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	id, current, ok := s.readSession(r)
	if !ok || !current.Admin {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.validCSRF(r.FormValue("csrf_token"), current.CSRF) && !s.validCSRF(r.Header.Get("X-CSRF-Token"), current.CSRF) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if s.config.EnablePOCAdmin {
		s.sessionsMu.Lock()
		delete(s.sessions, id)
		s.sessionsMu.Unlock()
	} else if err := s.store.RevokeAdminSession(r.Context(), id); err != nil {
		s.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "prods_admin", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.config.SecureCookies, MaxAge: -1})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	_, current, ok := s.readSession(r)
	if !ok || !current.can(identity.CapabilityAdminAccess) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	language, _, err := s.authText(r)
	if err != nil {
		http.Error(w, "unsupported language", http.StatusBadRequest)
		return
	}
	s.render(w, "admin", struct {
		CSRF     string
		Language string
	}{current.CSRF, language})
}

func (s *Server) adminRFQs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityRFQView, false); !ok {
		return
	}
	items, err := s.store.ListRFQs(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityAuditView, false); !ok {
		return
	}
	entries, err := s.store.ListAudit(r.Context(), 100)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) adminCreateProduct(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var product catalog.Product
	if err := decodeJSON(r.Body, &product); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	var err error
	if s.config.EnablePOCAdmin {
		if product.ID == "" {
			product.ID = "product-" + mustToken(12)
		}
		err = s.store.InsertProduct(r.Context(), product)
	} else {
		product, err = s.store.CreateProduct(r.Context(), current.UserID, product)
	}
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (s *Server) adminProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilityCatalogView, false); !ok {
		return
	}
	product, err := s.store.Product(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

type productUpdateRequest struct {
	ExpectedRevision int64           `json:"expected_revision"`
	Product          catalog.Product `json:"product"`
}

func (s *Server) adminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request productUpdateRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if request.Product.ID != "" && request.Product.ID != r.PathValue("id") {
		http.Error(w, "product id does not match route", http.StatusUnprocessableEntity)
		return
	}
	request.Product.ID = r.PathValue("id")
	if s.publisher != nil {
		stored, err := s.store.Product(r.Context(), request.Product.ID)
		if err != nil {
			s.writeCatalogError(w, err)
			return
		}
		if stored.Status == catalog.Published && request.Product.Status == catalog.Hidden {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "use the Hide action for synchronous public revocation"})
			return
		}
	}
	product, err := s.store.UpdateProduct(r.Context(), current.UserID, request.ExpectedRevision, request.Product)
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) adminUpdateProductURL(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64  `json:"expected_revision"`
		Slug             string `json:"slug"`
		CustomPath       string `json:"custom_path"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	product, err := s.store.UpdateProductURL(r.Context(), current.UserID, r.PathValue("id"), request.ExpectedRevision, request.Slug, request.CustomPath)
	if err != nil {
		s.writeCatalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (s *Server) adminHideProduct(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogPublish, true)
	if !ok {
		return
	}
	if s.config.EnablePOCAdmin {
		if err := s.store.HideProduct(r.Context(), r.PathValue("id")); err != nil {
			http.Error(w, "product not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if s.publisher == nil {
		http.Error(w, "public architecture unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.publisher.Revoke(r.Context(), publishing.RevokeRequest{
		ActorID: current.UserID, ProductID: r.PathValue("id"), ExpectedRevision: request.ExpectedRevision,
	}); err != nil {
		s.writeCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminArchiveProduct(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogPublish, true)
	if !ok {
		return
	}
	var request struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if err := decodeJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if s.publisher == nil {
		http.Error(w, "public architecture unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.publisher.Revoke(r.Context(), publishing.RevokeRequest{
		ActorID: current.UserID, ProductID: r.PathValue("id"), ExpectedRevision: request.ExpectedRevision, Archive: true,
	}); err != nil {
		s.writeCatalogError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireCapability(w http.ResponseWriter, r *http.Request, capability identity.Capability, csrf bool) (session, bool) {
	_, current, ok := s.readSession(r)
	if !ok || !current.Admin {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return session{}, false
	}
	if !current.can(capability) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return session{}, false
	}
	if csrf && !s.validCSRF(r.Header.Get("X-CSRF-Token"), current.CSRF) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return session{}, false
	}
	return current, true
}

func (s *Server) writeCatalogError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "product not found", http.StatusNotFound)
	case errors.Is(err, catalog.ErrRevisionConflict), errors.Is(err, catalog.ErrIdentityConflict),
		errors.Is(err, catalog.ErrRouteConflict), errors.Is(err, catalog.ErrCategoryCycle), errors.Is(err, catalog.ErrManualNormalization), sqlite.IsUniqueViolation(err):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, sqlite.ErrPermissionDenied):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, catalog.ErrArchivedProduct), errors.Is(err, catalog.ErrInvalidProduct),
		errors.Is(err, catalog.ErrInvalidCategory), errors.Is(err, catalog.ErrSystemCategory),
		errors.Is(err, catalog.ErrInvalidDictionary), errors.Is(err, catalog.ErrDisabledReference),
		errors.Is(err, catalog.ErrInvalidSpec), errors.Is(err, catalog.ErrInvalidDocument), errors.Is(err, catalog.ErrInvalidAsset),
		errors.Is(err, publishing.ErrSiteRouteInvalid):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	default:
		s.internalError(w, err)
	}
}

func (s *Server) ensureSession(w http.ResponseWriter, r *http.Request, admin bool) (string, session, error) {
	if id, current, ok := s.readSession(r); ok && (!admin || current.Admin) {
		return id, current, nil
	}
	id, err := secureToken(24)
	if err != nil {
		return "", session{}, err
	}
	csrf, err := secureToken(24)
	if err != nil {
		return "", session{}, err
	}
	current := session{CSRF: csrf, Admin: admin, Capabilities: make(map[identity.Capability]bool)}
	if admin && s.config.EnablePOCAdmin {
		for _, capability := range identity.OwnerCapabilities {
			current.Capabilities[capability] = true
		}
	}
	s.sessionsMu.Lock()
	s.sessions[id] = current
	s.sessionsMu.Unlock()
	name := "prods_anon"
	if admin {
		name = "prods_admin"
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.config.SecureCookies})
	return id, current, nil
}

func (s *Server) readSession(r *http.Request) (string, session, bool) {
	for _, name := range []string{"prods_admin", "prods_anon"} {
		cookie, err := r.Cookie(name)
		if err != nil {
			continue
		}
		if name == "prods_admin" && !s.config.EnablePOCAdmin {
			persisted, err := s.store.AdminSession(r.Context(), cookie.Value, time.Now().UTC())
			if err == nil {
				return cookie.Value, session{
					CSRF: persisted.CSRFToken, Admin: true, UserID: persisted.User.ID,
					Capabilities: persisted.User.Capabilities,
				}, true
			}
			continue
		}
		s.sessionsMu.Lock()
		current, ok := s.sessions[cookie.Value]
		s.sessionsMu.Unlock()
		if ok {
			return cookie.Value, current, true
		}
	}
	return "", session{}, false
}

func (s *Server) validCSRF(provided, expected string) bool {
	return len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render failed", "template", name, "error", err)
	}
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func decodeJSON(body io.ReadCloser, target any) error {
	defer body.Close()
	decoder := json.NewDecoder(io.LimitReader(body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && reflected.Kind() == reflect.Slice && reflected.IsNil() {
		value = reflect.MakeSlice(reflected.Type(), 0, 0).Interface()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeBody(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(body)
}

func secureToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func mustToken(bytes int) string {
	token, err := secureToken(bytes)
	if err != nil {
		panic(err)
	}
	return token
}

func HashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
