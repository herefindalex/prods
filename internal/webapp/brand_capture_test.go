package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prods/internal/brandcapture"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

type fakeBrandCapturer struct {
	candidate brandcapture.Candidate
	err       error
	calls     int
}

func (capturer *fakeBrandCapturer) Capture(_ context.Context, sourceURL string) (brandcapture.Candidate, error) {
	capturer.calls++
	candidate := capturer.candidate
	candidate.SourceURL = sourceURL
	return candidate, capturer.err
}

func TestAdminBrandCaptureRequiresCSRFReturnsDiffAndDoesNotMutateWorkingCopy(t *testing.T) {
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	capturer := &fakeBrandCapturer{candidate: brandcapture.Candidate{
		Organization: "Captured Corp", Colors: []string{"#112233", "#445566"},
		Fonts:                   []string{"Inter, sans-serif"},
		Navigation:              []brandcapture.Navigation{{Label: "Products", URL: "https://source.example/products"}},
		NeedsRightsConfirmation: true, WorkingCopyOnly: true,
	}}
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		BrandCapturer: capturer,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	csrf := loginAdmin(t, client, server.URL)

	withoutCSRF := postAdminJSON(t, client, server.URL+"/admin/api/website/capture", "", `{"source_url":"https://source.example/about"}`)
	if body := responseBody(t, withoutCSRF); withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("capture without CSRF status=%d body=%s", withoutCSRF.StatusCode, body)
	}
	if capturer.calls != 0 {
		t.Fatal("capturer ran before CSRF authorization")
	}

	before, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	response := postAdminJSON(t, client, server.URL+"/admin/api/website/capture", csrf, `{"source_url":"https://source.example/about"}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("capture status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var result brandCaptureResponse
	decodeResponseJSON(t, response, &result)
	if result.WorkingRevision != before.WorkingRevision || result.Candidate.Organization != "Captured Corp" ||
		!result.Candidate.NeedsRightsConfirmation || !result.Candidate.WorkingCopyOnly {
		t.Fatalf("unexpected capture response: %+v", result)
	}
	if result.Proposed.Organization.DisplayName != "Captured Corp" || result.Proposed.Theme.PrimaryColor != "#112233" {
		t.Fatalf("proposal did not apply candidate: %+v", result.Proposed)
	}
	if len(result.Diff) == 0 {
		t.Fatal("capture response omitted field diff")
	}
	after, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after.WorkingRevision != before.WorkingRevision || after.Working.Organization.DisplayName != before.Working.Organization.DisplayName {
		t.Fatalf("capture mutated working copy: before=%+v after=%+v", before, after)
	}
	if capturer.calls != 1 {
		t.Fatalf("capturer calls=%d", capturer.calls)
	}
}

func TestAdminBrandCaptureClassifiesUnsafeURL(t *testing.T) {
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	capturer := &fakeBrandCapturer{err: errors.Join(brandcapture.ErrNonPublicDestination, errors.New("private address"))}
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		BrandCapturer: capturer,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := loginAdmin(t, client, server.URL)
	response := postAdminJSON(t, client, server.URL+"/admin/api/website/capture", csrf, `{"source_url":"http://127.0.0.1"}`)
	if body := responseBody(t, response); response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe capture status=%d body=%s", response.StatusCode, body)
	}
}

func TestAdminBrandCaptureReportsSourceAccessDenied(t *testing.T) {
	store, err := sqlite.CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	capturer := &fakeBrandCapturer{err: errors.Join(brandcapture.ErrSourceAccessDenied, errors.New("HTTP 403"))}
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", AdminToken: "test-admin-token", EnablePOCAdmin: true,
		BrandCapturer: capturer,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := loginAdmin(t, client, server.URL)
	response := postAdminJSON(t, client, server.URL+"/admin/api/website/capture", csrf, `{"source_url":"https://blocked.example"}`)
	var body apiErrorResponse
	decodeResponseJSON(t, response, &body)
	if response.StatusCode != http.StatusBadGateway || body.Code != apiCodeBrandCaptureDenied {
		t.Fatalf("blocked capture status=%d body=%+v", response.StatusCode, body)
	}
}

func TestBrandCaptureProposalLeavesSEOAssetsAndCustomCSSUntouched(t *testing.T) {
	current := site.DefaultConfiguration()
	current.Organization.PrimaryLogoAsset = "asset-1"
	current.Theme.CustomCSS = "p{color:#123456}"
	current.Theme.CustomCSSEnabled = true
	current.SEO.DefaultTitle = "Manual SEO"
	proposed, _, err := brandcapture.BuildProposal(current, brandcapture.Candidate{
		SourceURL: "https://source.example", Organization: "Source", Colors: []string{"#abcdef"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Organization.PrimaryLogoAsset != "asset-1" || proposed.Theme.CustomCSS != current.Theme.CustomCSS ||
		!proposed.Theme.CustomCSSEnabled || proposed.SEO.DefaultTitle != "Manual SEO" {
		t.Fatalf("proposal crossed manual boundaries: %+v", proposed)
	}
}
