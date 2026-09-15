package searchnotify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"prods/internal/publishing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestIndexNowTreatsOnly200And202AsAcceptedAndBoundsBatch(t *testing.T) {
	statuses := []int{http.StatusOK, http.StatusAccepted, http.StatusTooManyRequests, http.StatusBadRequest}
	for _, status := range statuses {
		client := HTTPIndexNowClient{Client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if request.URL.String() != "https://api.indexnow.org/indexnow" || request.Header.Get("Content-Type") != "application/json; charset=utf-8" {
				t.Fatalf("IndexNow request URL=%s headers=%v", request.URL, request.Header)
			}
			return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(""))}, nil
		})}}
		gotStatus, _, retryable, err := client.SubmitIndexNow(t.Context(), IndexNowRequest{
			Host: "catalog.example.test", Key: strings.Repeat("a", 32),
			KeyLocation: "https://catalog.example.test/key.txt", URLList: []string{"https://catalog.example.test/products/one"},
		})
		accepted := status == http.StatusOK || status == http.StatusAccepted
		if gotStatus != status || (err == nil) != accepted || retryable != (status == http.StatusTooManyRequests) {
			t.Fatalf("IndexNow status=%d got=%d retryable=%v err=%v", status, gotStatus, retryable, err)
		}
	}
	tooMany := make([]string, MaxIndexNowURLs+1)
	if _, _, retryable, err := (HTTPIndexNowClient{}).SubmitIndexNow(t.Context(), IndexNowRequest{Key: "key", URLList: tooMany}); err == nil || retryable {
		t.Fatalf("oversized IndexNow batch retryable=%v err=%v", retryable, err)
	}
}

func TestGoogleSearchConsoleRefreshesOAuthAndSubmitsSitemapWithoutLeakingSecrets(t *testing.T) {
	var tokenCalls, submitCalls int
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "oauth.example.test":
			tokenCalls++
			body, _ := io.ReadAll(request.Body)
			if !strings.Contains(string(body), "client_secret=client-secret") || !strings.Contains(string(body), "refresh_token=refresh-secret") {
				t.Fatalf("OAuth request body=%s", body)
			}
			return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"access_token":"access-secret","expires_in":3600,"token_type":"Bearer"}`))}, nil
		case "search.example.test":
			submitCalls++
			if request.Method != http.MethodPut || request.Header.Get("Authorization") != "Bearer access-secret" ||
				!strings.Contains(request.URL.EscapedPath(), "https:%2F%2Fcatalog.example.test%2F") ||
				!strings.Contains(request.URL.EscapedPath(), "https:%2F%2Fcatalog.example.test%2Fsitemap.xml") {
				t.Fatalf("Search Console request=%s %s headers=%v", request.Method, request.URL, request.Header)
			}
			return &http.Response{StatusCode: http.StatusNoContent, Status: "204 No Content", Body: io.NopCloser(strings.NewReader(""))}, nil
		default:
			return nil, errors.New("unexpected host")
		}
	})}
	client, err := NewGoogleSearchConsoleClient(GoogleOAuthConfig{
		ClientID: "client-id", ClientSecret: "client-secret", RefreshToken: "refresh-secret",
		TokenURL: "https://oauth.example.test/token", APIBaseURL: "https://search.example.test/webmasters/v3", Client: httpClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		status, message, retryable, err := client.SubmitSitemap(t.Context(), "https://catalog.example.test/", "https://catalog.example.test/sitemap.xml")
		if err != nil || status != http.StatusNoContent || retryable || !strings.Contains(message, "204") {
			t.Fatalf("Google submission status=%d message=%q retryable=%v err=%v", status, message, retryable, err)
		}
	}
	if tokenCalls != 1 || submitCalls != 2 {
		t.Fatalf("Google calls token=%d submit=%d", tokenCalls, submitCalls)
	}
}

func TestEligibleBaseURLRejectsPreviewAndPrivateAddresses(t *testing.T) {
	for _, raw := range []string{"http://catalog.example.test", "https://localhost", "https://127.0.0.1", "https://10.0.0.1"} {
		if _, err := EligibleBaseURL(raw); err == nil {
			t.Fatalf("eligible base URL accepted %q", raw)
		}
	}
	if parsed, err := EligibleBaseURL("https://catalog.example.test"); err != nil || parsed.Hostname() != "catalog.example.test" {
		t.Fatalf("eligible base URL=%v err=%v", parsed, err)
	}
}

type failingRepository struct {
	claimed bool
	failed  bool
}

func (*failingRepository) SearchIntegrationSettings(context.Context) (Settings, error) {
	return Settings{IndexNowEnabled: true, IndexNowKey: strings.Repeat("a", 32)}, nil
}
func (*failingRepository) ActivePublications(context.Context) ([]publishing.ActivePublication, error) {
	return nil, nil
}
func (*failingRepository) ReconcileSearchSubmissions(context.Context, string, Settings, []publishing.ActivePublication) (int, error) {
	return 0, nil
}
func (*failingRepository) ResetSearchSubmissionClaims(context.Context) error { return nil }
func (repository *failingRepository) ClaimSearchSubmission(_ context.Context, _ time.Time, indexNow, _ bool) (Job, bool, error) {
	if repository.claimed || !indexNow {
		return Job{}, false, nil
	}
	repository.claimed = true
	return Job{ID: "job-1", Provider: ProviderIndexNow, URLs: []string{"https://catalog.example.test/products/one"}, Attempts: 1}, true, nil
}
func (*failingRepository) CompleteSearchSubmission(context.Context, string, int, string) error {
	return errors.New("unexpected completion")
}
func (repository *failingRepository) FailSearchSubmission(context.Context, string, int, string, bool, time.Time) error {
	repository.failed = true
	return nil
}

type failingIndexNow struct{}

func (failingIndexNow) SubmitIndexNow(context.Context, IndexNowRequest) (int, string, bool, error) {
	return 503, "external outage", true, errors.New("external outage")
}

func TestExternalSearchFailureIsRecordedWithoutFailingCoreProcess(t *testing.T) {
	repository := &failingRepository{}
	manager := &Manager{
		repository:    repository,
		config:        Config{BaseURL: "https://catalog.example.test", IndexNow: failingIndexNow{}, Now: time.Now},
		publicBaseURL: true,
	}
	if err := manager.Process(t.Context()); err != nil {
		t.Fatalf("external search failure escaped optional manager: %v", err)
	}
	if !repository.failed {
		t.Fatal("external search failure was not recorded")
	}
}
