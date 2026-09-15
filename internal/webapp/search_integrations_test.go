package webapp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"prods/internal/searchnotify"
)

func TestSearchIntegrationAdminValidationAndPublicIndexNowKey(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	endpoint := fixture.server.URL + "/admin/api/search-integrations"
	response, err := fixture.client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var initial searchIntegrationsResponse
	decodeResponseJSON(t, response, &initial)
	if response.StatusCode != http.StatusOK || initial.Revision != 1 || initial.IndexNowAvailable || initial.GoogleAvailable {
		t.Fatalf("initial search integrations status=%d state=%+v", response.StatusCode, initial)
	}

	withoutCSRF := putAdminJSON(t, fixture.client, endpoint, "", `{"revision":1,"indexnow_enabled":true}`)
	withoutCSRF.Body.Close()
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("search integration update without CSRF=%d", withoutCSRF.StatusCode)
	}
	unavailable := putAdminJSON(t, fixture.client, endpoint, fixture.csrf, `{"revision":1,"indexnow_enabled":true}`)
	unavailable.Body.Close()
	if unavailable.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("IndexNow on preview Base URL status=%d", unavailable.StatusCode)
	}

	settings, err := fixture.store.SearchIntegrationSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.IndexNowEnabled = true
	settings, err = fixture.store.UpdateSearchIntegrationSettings(t.Context(), fixture.owner.ID, settings.Revision, settings)
	if err != nil {
		t.Fatal(err)
	}
	keyResponse, err := http.Get(fixture.server.URL + "/" + settings.IndexNowKey + ".txt")
	if err != nil {
		t.Fatal(err)
	}
	keyBody := responseBody(t, keyResponse)
	if keyResponse.StatusCode != http.StatusOK || keyBody != settings.IndexNowKey ||
		keyResponse.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("IndexNow key status=%d headers=%v body=%q", keyResponse.StatusCode, keyResponse.Header, keyBody)
	}
	unknown, err := http.Get(fixture.server.URL + "/not-the-key.txt")
	if err != nil {
		t.Fatal(err)
	}
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown IndexNow key status=%d", unknown.StatusCode)
	}

	settings.IndexNowEnabled = false
	if _, err := fixture.store.UpdateSearchIntegrationSettings(t.Context(), fixture.owner.ID, settings.Revision, settings); err != nil {
		t.Fatal(err)
	}
	hidden, err := http.Get(fixture.server.URL + "/" + settings.IndexNowKey + ".txt")
	if err != nil {
		t.Fatal(err)
	}
	hidden.Body.Close()
	if hidden.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled IndexNow key status=%d", hidden.StatusCode)
	}
}

func TestSearchIntegrationJSONNeverExposesOAuthSecrets(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	response, err := fixture.client.Get(fixture.server.URL + "/admin/api/search-integrations")
	if err != nil {
		t.Fatal(err)
	}
	body := responseBody(t, response)
	if !json.Valid([]byte(body)) || strings.Contains(strings.ToLower(body), "client_secret") ||
		strings.Contains(strings.ToLower(body), "refresh_token") {
		t.Fatalf("unsafe search integration response=%s", body)
	}
	var settings searchnotify.Settings
	if err := json.Unmarshal([]byte(body), &settings); err != nil {
		t.Fatal(err)
	}
}
