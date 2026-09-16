package distribution

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdateStatus(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":"v1.3.0","html_url":%q,"assets":[{"name":"prods-linux-amd64","browser_download_url":%q}]}`,
			server.URL+"/herefindalex/prods/releases/tag/v1.3.0", server.URL+"/download")
	}))
	defer server.Close()
	client := testClient(server)
	available := client.UpdateStatus(context.Background(), "1.2.3", "linux", "amd64")
	if available.Status != "update_available" || available.DownloadURL != server.URL+"/download" {
		t.Fatalf("available = %+v", available)
	}
	current := client.UpdateStatus(context.Background(), "v1.4.0", "linux", "amd64")
	if current.Status != "current" {
		t.Fatalf("current = %+v", current)
	}
	development := client.UpdateStatus(context.Background(), "dev", "linux", "amd64")
	if development.Status != "development" {
		t.Fatalf("development = %+v", development)
	}
}

func TestUnavailableLatestReleaseIsCached(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := testClient(server)
	if _, err := client.LatestRelease(context.Background()); err == nil {
		t.Fatal("404 repository unexpectedly reported a release")
	}
	if _, err := client.LatestRelease(context.Background()); err == nil {
		t.Fatal("cached 404 unexpectedly reported a release")
	}
	if requests.Load() != 1 {
		t.Fatalf("404 requests = %d, want cached single request", requests.Load())
	}
}

func TestLatestReleaseRejectsUntrustedAsset(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "{\"tag_name\":\"v1.2.3\",\"html_url\":%q,\"assets\":[{\"name\":\"prods-linux-amd64\",\"browser_download_url\":\"https://evil.example/prods\"}]}",
			server.URL+"/herefindalex/prods/releases/tag/v1.2.3")
	}))
	defer server.Close()
	if _, err := testClient(server).LatestRelease(context.Background()); err == nil {
		t.Fatal("untrusted release asset was accepted")
	}
}
func TestUpdateStatusRejectsPrereleaseAndMissingPlatformAsset(t *testing.T) {
	for _, test := range []struct {
		name       string
		prerelease bool
		assetName  string
	}{
		{name: "prerelease", prerelease: true, assetName: "prods-linux-amd64"},
		{name: "missing platform asset", assetName: "prods-windows-amd64.exe"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fmt.Fprintf(w, `{"tag_name":"v2.0.0","html_url":%q,"prerelease":%t,"assets":[{"name":%q,"browser_download_url":%q}]}`,
					server.URL+"/herefindalex/prods/releases/tag/v2.0.0", test.prerelease, test.assetName, server.URL+"/download")
			}))
			defer server.Close()
			status := testClient(server).UpdateStatus(context.Background(), "v1.0.0", "linux", "amd64")
			if status.Status != "unavailable" || status.DownloadURL != "" {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func testClient(server *httptest.Server) *Client {
	return &Client{
		HTTPClient: server.Client(), APIBase: server.URL, RepositoryURL: server.URL,
		CacheTTL: time.Minute, AllowTestHTTP: true, cache: make(map[string]cachedRelease),
	}
}
