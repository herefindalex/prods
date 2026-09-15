package searchnotify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"prods/internal/publishing"
)

const (
	ProviderIndexNow = "indexnow"
	ProviderGoogle   = "google_search_console"
	MaxIndexNowURLs  = 10_000
)

var (
	ErrInvalidSettings  = errors.New("invalid search integration settings")
	ErrSettingsConflict = errors.New("search integration settings revision conflict")
	ErrJobNotRetryable  = errors.New("search submission job is not retryable")
)

type Settings struct {
	Revision        int64  `json:"revision"`
	IndexNowEnabled bool   `json:"indexnow_enabled"`
	IndexNowKey     string `json:"indexnow_key,omitempty"`
	GoogleEnabled   bool   `json:"google_enabled"`
	GoogleSiteURL   string `json:"google_site_url,omitempty"`
	UpdatedAt       string `json:"updated_at"`
}

type Job struct {
	ID              string   `json:"id"`
	Provider        string   `json:"provider"`
	Subject         string   `json:"subject"`
	URLs            []string `json:"urls,omitempty"`
	Status          string   `json:"status"`
	Attempts        int      `json:"attempts"`
	NextAttemptAt   string   `json:"next_attempt_at,omitempty"`
	HTTPStatus      int      `json:"http_status,omitempty"`
	ResponseMessage string   `json:"response_message,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type Repository interface {
	SearchIntegrationSettings(context.Context) (Settings, error)
	ActivePublications(context.Context) ([]publishing.ActivePublication, error)
	ReconcileSearchSubmissions(context.Context, string, Settings, []publishing.ActivePublication) (int, error)
	ResetSearchSubmissionClaims(context.Context) error
	ClaimSearchSubmission(context.Context, time.Time, bool, bool) (Job, bool, error)
	CompleteSearchSubmission(context.Context, string, int, string) error
	FailSearchSubmission(context.Context, string, int, string, bool, time.Time) error
}

type IndexNowSubmitter interface {
	SubmitIndexNow(context.Context, IndexNowRequest) (int, string, bool, error)
}

type GoogleSubmitter interface {
	SubmitSitemap(context.Context, string, string) (int, string, bool, error)
}

type IndexNowRequest struct {
	Host        string   `json:"host"`
	Key         string   `json:"key"`
	KeyLocation string   `json:"keyLocation"`
	URLList     []string `json:"urlList"`
}

type Config struct {
	BaseURL      string
	IndexNow     IndexNowSubmitter
	Google       GoogleSubmitter
	PollInterval time.Duration
	Now          func() time.Time
}

type Manager struct {
	repository    Repository
	config        Config
	wake          chan struct{}
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	draining      atomic.Bool
	publicBaseURL bool
}

func NewManager(ctx context.Context, repository Repository, config Config) (*Manager, error) {
	if repository == nil {
		return nil, errors.New("search notification repository is required")
	}
	baseURL, baseURLError := EligibleBaseURL(config.BaseURL)
	if baseURLError == nil {
		config.BaseURL = strings.TrimRight(baseURL.String(), "/")
	} else {
		config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 10 * time.Second
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if err := repository.ResetSearchSubmissionClaims(ctx); err != nil {
		return nil, fmt.Errorf("reset search submission claims: %w", err)
	}
	manager := &Manager{repository: repository, config: config, wake: make(chan struct{}, 1), publicBaseURL: baseURLError == nil}
	workerContext, cancel := context.WithCancel(context.Background())
	manager.cancel = cancel
	manager.wg.Add(1)
	go manager.worker(workerContext)
	manager.Wake()
	return manager, nil
}

func (m *Manager) GoogleAvailable() bool {
	return m != nil && m.publicBaseURL && m.config.Google != nil
}

func (m *Manager) IndexNowAvailable() bool {
	return m != nil && m.publicBaseURL && m.config.IndexNow != nil
}

func (m *Manager) Wake() {
	if m == nil || m.draining.Load() {
		return
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) BeginDrain() {
	if m != nil {
		m.draining.Store(true)
	}
}

func (m *Manager) Close() {
	if m == nil || m.cancel == nil {
		return
	}
	m.BeginDrain()
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) worker(ctx context.Context) {
	defer m.wg.Done()
	ticker := time.NewTicker(m.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-ticker.C:
		}
		if m.draining.Load() {
			continue
		}
		_ = m.Process(ctx)
	}
}

func (m *Manager) Process(ctx context.Context) error {
	settings, err := m.repository.SearchIntegrationSettings(ctx)
	if err != nil {
		return err
	}
	effective := settings
	effective.IndexNowEnabled = settings.IndexNowEnabled && m.IndexNowAvailable()
	effective.GoogleEnabled = settings.GoogleEnabled && m.GoogleAvailable()
	publications, err := m.repository.ActivePublications(ctx)
	if err != nil {
		return err
	}
	if _, err := m.repository.ReconcileSearchSubmissions(ctx, m.config.BaseURL, effective, publications); err != nil {
		return err
	}
	for !m.draining.Load() {
		job, found, err := m.repository.ClaimSearchSubmission(ctx, m.config.Now().UTC(), effective.IndexNowEnabled, effective.GoogleEnabled)
		if err != nil || !found {
			return err
		}
		status, message, retryable, submitErr := m.submit(ctx, settings, job)
		if submitErr == nil {
			if err := m.repository.CompleteSearchSubmission(context.Background(), job.ID, status, message); err != nil {
				return err
			}
			continue
		}
		if message == "" {
			message = submitErr.Error()
		}
		if err := m.repository.FailSearchSubmission(context.Background(), job.ID, status, message, retryable, m.config.Now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) submit(ctx context.Context, settings Settings, job Job) (int, string, bool, error) {
	switch job.Provider {
	case ProviderIndexNow:
		if !settings.IndexNowEnabled || m.config.IndexNow == nil {
			return 0, "IndexNow integration is disabled or unavailable", false, errors.New("IndexNow unavailable")
		}
		base, _ := url.Parse(m.config.BaseURL)
		return m.config.IndexNow.SubmitIndexNow(ctx, IndexNowRequest{
			Host: base.Hostname(), Key: settings.IndexNowKey,
			KeyLocation: m.config.BaseURL + "/" + settings.IndexNowKey + ".txt", URLList: job.URLs,
		})
	case ProviderGoogle:
		if !settings.GoogleEnabled || m.config.Google == nil {
			return 0, "Google Search Console integration is disabled or unavailable", false, errors.New("integration with Google Search Console is unavailable")
		}
		return m.config.Google.SubmitSitemap(ctx, settings.GoogleSiteURL, job.Subject)
	default:
		return 0, "unknown search submission provider", false, errors.New("unknown search submission provider")
	}
}

func EligibleBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("search integrations require a public HTTPS base URL")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, errors.New("search integrations require a public HTTPS base URL")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
		return nil, errors.New("search integrations require a public HTTPS base URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed, nil
}

func SitemapFingerprint(settings Settings, publications []publishing.ActivePublication) string {
	parts := make([]string, 0, len(publications)+1)
	parts = append(parts, settings.GoogleSiteURL)
	for _, publication := range publications {
		parts = append(parts, fmt.Sprintf("%s\x00%d\x00%d\x00%s", publication.Route, publication.SourceRevision, publication.SiteEpoch, publication.ManifestHash))
	}
	sort.Strings(parts[1:])
	hash := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(hash[:])
}

func EncodeURLs(urls []string) (string, error) {
	body, err := json.Marshal(urls)
	return string(body), err
}

func DecodeURLs(body string) ([]string, error) {
	var urls []string
	if err := json.Unmarshal([]byte(body), &urls); err != nil {
		return nil, err
	}
	if len(urls) == 0 || len(urls) > MaxIndexNowURLs {
		return nil, errors.New("invalid IndexNow URL batch")
	}
	return urls, nil
}

func RetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second
	for current := 1; current < attempt && delay < time.Hour; current++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func SafeResponseMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r >= 0x20 {
			return r
		}
		return -1
	}, strings.TrimSpace(message))
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}

type HTTPIndexNowClient struct {
	Endpoint string
	Client   *http.Client
}

func (client HTTPIndexNowClient) SubmitIndexNow(ctx context.Context, request IndexNowRequest) (int, string, bool, error) {
	endpoint := strings.TrimSpace(client.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.indexnow.org/indexnow"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return 0, "invalid IndexNow endpoint", false, errors.New("invalid IndexNow endpoint")
	}
	if len(request.URLList) == 0 || len(request.URLList) > MaxIndexNowURLs || request.Key == "" {
		return 0, "invalid IndexNow request", false, errors.New("invalid IndexNow request")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return 0, "encode IndexNow request", false, err
	}
	httpClient := client.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return 0, "create IndexNow request", false, err
	}
	httpRequest.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := httpClient.Do(httpRequest)
	if err != nil {
		return 0, "IndexNow request failed", true, err
	}
	defer response.Body.Close()
	message := SafeResponseMessage(response.Status)
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted {
		return response.StatusCode, message, false, nil
	}
	retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
	return response.StatusCode, message, retryable, fmt.Errorf("IndexNow returned HTTP %d", response.StatusCode)
}
