package webapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"prods/internal/inquiries"
	"prods/internal/storage/sqlite"
)

func TestRFQRateLimiterUsesDistinctKeysAndResetsAtWindowBoundary(t *testing.T) {
	limiter := newRFQRateLimiter()
	now := time.Date(2026, 9, 14, 20, 0, 0, 0, time.UTC)
	if allowed, _ := limiter.allow("source", "key-a", now, 1, time.Minute); !allowed {
		t.Fatal("first RFQ key was rejected")
	}
	if allowed, _ := limiter.allow("source", "key-a", now.Add(time.Second), 1, time.Minute); !allowed {
		t.Fatal("same RFQ key did not remain admissible for idempotent retry")
	}
	if allowed, retry := limiter.allow("source", "key-b", now.Add(2*time.Second), 1, time.Minute); allowed || retry != 58*time.Second {
		t.Fatalf("second RFQ key allowed=%v retry=%s", allowed, retry)
	}
	if allowed, _ := limiter.allow("source", "key-b", now.Add(time.Minute), 1, time.Minute); !allowed {
		t.Fatal("RFQ window did not reset")
	}
	if allowed, _ := limiter.allow("other-source", "key-c", now, 1, time.Minute); !allowed {
		t.Fatal("one source consumed another source's RFQ limit")
	}
}

func TestTrafficSettingsAndRFQLimitPreserveReplayConflictAndHTMLInput(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	client := fixture.client
	baseURL := fixture.server.URL
	csrf := fixture.csrf

	response, err := client.Get(baseURL + "/admin/api/traffic-settings")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("traffic settings status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var settings sqlite.TrafficSettings
	decodeResponseJSON(t, response, &settings)
	settings.RFQLimit = 1
	settings.RFQWindowSeconds = 60
	updated := putTrafficSettings(t, client, baseURL, csrf, settings.Version, settings)
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("traffic settings update=%d body=%s", updated.StatusCode, responseBody(t, updated))
	}
	decodeResponseJSON(t, updated, &settings)
	stale := putTrafficSettings(t, client, baseURL, csrf, 1, settings)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale traffic settings=%d body=%s", stale.StatusCode, responseBody(t, stale))
	}

	firstKey, err := sqlite.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	firstSubmission := inquiries.Submission{
		Name: "Buyer One", Email: "one@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "REQUEST-ONE"}},
	}
	first := doPostRFQJSON(t, client, baseURL, csrf, firstKey, firstSubmission)
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first RFQ=%d body=%s", first.StatusCode, responseBody(t, first))
	}
	var original inquiries.Receipt
	decodeResponseJSON(t, first, &original)

	secondKey, err := sqlite.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	second := postRFQForm(t, client, baseURL, url.Values{
		"csrf_token":     {csrf},
		"submission_key": {secondKey},
		"kind":           {"requested"},
		"requested":      {"REQUEST-TWO"},
		"name":           {"Buyer Two"},
		"email":          {"two@example.test"},
	})
	secondBody := responseBody(t, second)
	if second.StatusCode != http.StatusTooManyRequests || second.Header.Get("Retry-After") == "" ||
		!strings.Contains(secondBody, "Buyer Two") || !strings.Contains(secondBody, "REQUEST-TWO") || !strings.Contains(secondBody, secondKey) {
		t.Fatalf("rate-limited HTML status=%d retry=%q body=%s", second.StatusCode, second.Header.Get("Retry-After"), secondBody)
	}
	if seconds, err := strconv.Atoi(second.Header.Get("Retry-After")); err != nil || seconds < 1 || seconds > 60 {
		t.Fatalf("Retry-After=%q err=%v", second.Header.Get("Retry-After"), err)
	}

	replay := doPostRFQJSON(t, client, baseURL, csrf, firstKey, firstSubmission)
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("rate-limited receipt replay=%d body=%s", replay.StatusCode, responseBody(t, replay))
	}
	var replayed inquiries.Receipt
	decodeResponseJSON(t, replay, &replayed)
	if !replayed.Replay || replayed.RFQID != original.RFQID {
		t.Fatalf("replayed receipt=%+v original=%+v", replayed, original)
	}

	changed := firstSubmission
	changed.Items = []inquiries.Item{{Kind: "requested", Requested: "CHANGED"}}
	conflict := doPostRFQJSON(t, client, baseURL, csrf, firstKey, changed)
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("rate-limited idempotency conflict=%d body=%s", conflict.StatusCode, responseBody(t, conflict))
	}
	publicRead, err := client.Get(baseURL + "/search")
	if err != nil {
		t.Fatal(err)
	}
	if publicRead.StatusCode != http.StatusOK {
		t.Fatalf("RFQ limit affected public read=%d body=%s", publicRead.StatusCode, responseBody(t, publicRead))
	}
}

func putTrafficSettings(t *testing.T, client *http.Client, baseURL, csrf string, expectedVersion int64, settings sqlite.TrafficSettings) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]any{"expected_version": expectedVersion, "settings": settings})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPut, baseURL+"/admin/api/traffic-settings", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func doPostRFQJSON(t *testing.T, client *http.Client, baseURL, csrf, key string, submission inquiries.Submission) *http.Response {
	t.Helper()
	body, err := json.Marshal(rfqJSONRequest{SubmissionKey: key, Submission: submission})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/rfqs", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func postRFQForm(t *testing.T, client *http.Client, baseURL string, values url.Values) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/rfq", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
