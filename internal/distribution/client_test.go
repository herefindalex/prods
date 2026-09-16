package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadSampleDataUsesMatchingReleaseAndPersistsVerifiedFile(t *testing.T) {
	body := []byte(`{"schema_version":1,"release_version":"v1.2.3","dataset_version":"test-r1","seed":1,"source_statement":"synthetic","categories":[],"products":[{"id":"sample-1","part_number":"SAMPLE-1","name":"Sample","status":"hidden"}]}`)
	digest := sha256.Sum256(body)
	var releaseRequests atomic.Int64
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/herefindalex/prods/releases/tags/v1.2.3":
			releaseRequests.Add(1)
			fmt.Fprintf(w, `{"tag_name":"v1.2.3","html_url":%q,"assets":[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]}`,
				server.URL+"/herefindalex/prods/releases/tag/v1.2.3", SampleAssetName, server.URL+"/assets/sample", SampleChecksumName, server.URL+"/assets/checksum")
		case "/assets/sample":
			_, _ = w.Write(body)
		case "/assets/checksum":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(digest[:]), SampleAssetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := testClient(server)
	if !client.SampleAvailable(context.Background(), "v1.2.3") {
		t.Fatal("matching release should report sample data available")
	}
	root := t.TempDir()
	downloaded, err := client.DownloadSampleData(context.Background(), root, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if releaseRequests.Load() != 1 {
		t.Fatalf("release metadata requests = %d, want cached single request", releaseRequests.Load())
	}
	if downloaded.Data.ReleaseVersion != "v1.2.3" || downloaded.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("downloaded metadata = %+v", downloaded)
	}
	stored, err := os.ReadFile(filepath.Join(root, "sample-data", SampleAssetName))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(body) {
		t.Fatal("stored sample data changed")
	}
	info, err := os.Stat(downloaded.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("sample file mode = %o", info.Mode().Perm())
	}
}

func TestDownloadSampleDataRejectsChecksumAndReleaseVersionMismatch(t *testing.T) {
	for _, test := range []struct {
		name     string
		body     string
		checksum string
	}{
		{name: "checksum", body: `{"schema_version":1,"release_version":"v1.2.3","dataset_version":"test-r1","seed":1,"source_statement":"synthetic","categories":[],"products":[{"id":"a","part_number":"A"}]}`, checksum: strings.Repeat("0", 64)},
		{name: "release version", body: `{"schema_version":1,"release_version":"v9.9.9","dataset_version":"test-r1","seed":1,"source_statement":"synthetic","categories":[],"products":[{"id":"a","part_number":"A"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(test.body)
			digest := sha256.Sum256(body)
			checksum := test.checksum
			if checksum == "" {
				checksum = hex.EncodeToString(digest[:])
			}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/releases/tags/"):
					fmt.Fprintf(w, `{"tag_name":"v1.2.3","html_url":%q,"assets":[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]}`,
						server.URL+"/herefindalex/prods/releases/tag/v1.2.3", SampleAssetName, server.URL+"/sample", SampleChecksumName, server.URL+"/checksum")
				case r.URL.Path == "/sample":
					_, _ = w.Write(body)
				case r.URL.Path == "/checksum":
					fmt.Fprintln(w, checksum)
				}
			}))
			defer server.Close()
			_, err := testClient(server).DownloadSampleData(context.Background(), t.TempDir(), "v1.2.3")
			if err == nil {
				t.Fatal("invalid sample data was accepted")
			}
		})
	}
}

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

func TestUnpublishedRepositoryIsUnavailableAndCached(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := testClient(server)
	if client.SampleAvailable(context.Background(), "v1.0.0") || client.SampleAvailable(context.Background(), "v1.0.0") {
		t.Fatal("404 repository unexpectedly reported sample data")
	}
	if requests.Load() != 1 {
		t.Fatalf("404 requests = %d, want cached single request", requests.Load())
	}
}

func TestReleaseForVersionRejectsMismatchedTagAndUntrustedAsset(t *testing.T) {
	for _, test := range []struct {
		name    string
		release func(string) string
	}{
		{
			name: "mismatched tag",
			release: func(base string) string {
				return fmt.Sprintf(`{"tag_name":"v9.9.9","html_url":%q,"assets":[]}`, base+"/herefindalex/prods/releases/tag/v9.9.9")
			},
		},
		{
			name: "untrusted asset",
			release: func(base string) string {
				return fmt.Sprintf(`{"tag_name":"v1.2.3","html_url":%q,"assets":[{"name":%q,"browser_download_url":"https://evil.example/sample"}]}`,
					base+"/herefindalex/prods/releases/tag/v1.2.3", SampleAssetName)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.release(server.URL)))
			}))
			defer server.Close()
			if _, err := testClient(server).ReleaseForVersion(context.Background(), "v1.2.3"); err == nil {
				t.Fatal("invalid release metadata was accepted")
			}
		})
	}
}

func TestDownloadSampleDataRejectsInvalidReleaseAndPayloadShapes(t *testing.T) {
	valid := `{"schema_version":1,"release_version":"v1.2.3","dataset_version":"test-r1","seed":1,"source_statement":"synthetic","categories":[],"products":[{"id":"a","part_number":"A"}]}`
	for _, test := range []struct {
		name          string
		body          string
		prerelease    bool
		dataAsset     bool
		checksumAsset bool
	}{
		{name: "missing data asset", body: valid, checksumAsset: true},
		{name: "missing checksum asset", body: valid, dataAsset: true},
		{name: "prerelease", body: valid, prerelease: true, dataAsset: true, checksumAsset: true},
		{name: "malformed JSON", body: `{`, dataAsset: true, checksumAsset: true},
		{name: "unknown field", body: strings.TrimSuffix(valid, `}`) + `,"unexpected":true}`, dataAsset: true, checksumAsset: true},
		{name: "invalid status", body: strings.Replace(valid, `"part_number":"A"`, `"part_number":"A","status":"draft"`, 1), dataAsset: true, checksumAsset: true},
		{name: "oversized name", body: strings.Replace(valid, `"part_number":"A"`, `"part_number":"A","name":"`+strings.Repeat("x", 501)+`"`, 1), dataAsset: true, checksumAsset: true},
		{name: "relative document URL", body: strings.Replace(valid, `"part_number":"A"`, `"part_number":"A","document_url":"manual.pdf"`, 1), dataAsset: true, checksumAsset: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(test.body)
			digest := sha256.Sum256(body)
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/releases/tags/"):
					assets := make([]string, 0, 2)
					if test.dataAsset {
						assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, SampleAssetName, server.URL+"/sample"))
					}
					if test.checksumAsset {
						assets = append(assets, fmt.Sprintf(`{"name":%q,"browser_download_url":%q}`, SampleChecksumName, server.URL+"/checksum"))
					}
					fmt.Fprintf(w, `{"tag_name":"v1.2.3","html_url":%q,"prerelease":%t,"assets":[%s]}`,
						server.URL+"/herefindalex/prods/releases/tag/v1.2.3", test.prerelease, strings.Join(assets, ","))
				case r.URL.Path == "/sample":
					_, _ = w.Write(body)
				case r.URL.Path == "/checksum":
					fmt.Fprintf(w, "%x  %s\n", digest, SampleAssetName)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			if _, err := testClient(server).DownloadSampleData(context.Background(), t.TempDir(), "v1.2.3"); err == nil {
				t.Fatal("invalid sample release or payload was accepted")
			}
		})
	}
}

func TestDownloadRejectsOversizedAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 33)))
	}))
	defer server.Close()
	client := testClient(server)
	if _, err := client.download(context.Background(), server.URL+"/asset", 32); err == nil {
		t.Fatal("oversized asset was accepted")
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
