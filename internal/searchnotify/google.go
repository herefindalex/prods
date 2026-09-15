package searchnotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	TokenURL     string
	APIBaseURL   string
	Client       *http.Client
}

type GoogleSearchConsoleClient struct {
	config      GoogleOAuthConfig
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewGoogleSearchConsoleClient(config GoogleOAuthConfig) (*GoogleSearchConsoleClient, error) {
	config.ClientID = strings.TrimSpace(config.ClientID)
	config.ClientSecret = strings.TrimSpace(config.ClientSecret)
	config.RefreshToken = strings.TrimSpace(config.RefreshToken)
	if config.ClientID == "" || config.ClientSecret == "" || config.RefreshToken == "" {
		return nil, errors.New("Google Search Console OAuth client ID, client secret, and refresh token are required")
	}
	if config.TokenURL == "" {
		config.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if config.APIBaseURL == "" {
		config.APIBaseURL = "https://www.googleapis.com/webmasters/v3"
	}
	for _, endpoint := range []string{config.TokenURL, config.APIBaseURL} {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, errors.New("Google Search Console endpoints must be absolute HTTPS URLs")
		}
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: 20 * time.Second}
	}
	return &GoogleSearchConsoleClient{config: config}, nil
}

func (client *GoogleSearchConsoleClient) SubmitSitemap(ctx context.Context, siteURL, sitemapURL string) (int, string, bool, error) {
	if strings.TrimSpace(siteURL) == "" || strings.TrimSpace(sitemapURL) == "" {
		return 0, "Google Search Console site and Sitemap URLs are required", false, errors.New("invalid Google Search Console submission")
	}
	accessToken, err := client.token(ctx)
	if err != nil {
		return 0, "Google OAuth token refresh failed", true, err
	}
	endpoint := strings.TrimRight(client.config.APIBaseURL, "/") + "/sites/" + url.PathEscape(siteURL) + "/sitemaps/" + url.PathEscape(sitemapURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return 0, "create Google Search Console request", false, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := client.config.Client.Do(request)
	if err != nil {
		return 0, "Google Search Console request failed", true, err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	message := SafeResponseMessage(string(body))
	if message == "" {
		message = response.Status
	}
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusNoContent {
		return response.StatusCode, message, false, nil
	}
	retryable := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
	return response.StatusCode, message, retryable, fmt.Errorf("Google Search Console returned HTTP %d", response.StatusCode)
}

func (client *GoogleSearchConsoleClient) token(ctx context.Context) (string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.accessToken != "" && time.Now().UTC().Add(time.Minute).Before(client.expiresAt) {
		return client.accessToken, nil
	}
	form := url.Values{
		"client_id":     {client.config.ClientID},
		"client_secret": {client.config.ClientSecret},
		"refresh_token": {client.config.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.config.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.config.Client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Google OAuth token endpoint returned HTTP %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return "", errors.New("Google OAuth token response was invalid")
	}
	if strings.TrimSpace(token.AccessToken) == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return "", errors.New("Google OAuth token response did not contain a bearer access token")
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 300
	}
	client.accessToken = token.AccessToken
	client.expiresAt = time.Now().UTC().Add(time.Duration(token.ExpiresIn) * time.Second)
	return client.accessToken, nil
}
