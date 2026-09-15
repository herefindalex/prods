package webapp

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"prods/internal/identity"
	"prods/internal/site"
	"prods/internal/storage/sqlite"
)

func TestAdminSiteTimeZoneSettings(t *testing.T) {
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail:       "owner@example.test",
		PasswordHash:     password,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US", "zh-TW"},
		TimeZone:         "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{BaseURL: "https://catalog.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	t.Cleanup(server.Close)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, "owner@example.test", "ownerpass1")

	response, err := client.Get(server.URL + "/admin/api/system/settings")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("get settings status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var initial site.Settings
	decodeResponseJSON(t, response, &initial)

	updatedResponse := putAdminJSON(t, client, server.URL+"/admin/api/system/settings", csrf,
		`{"expected_revision":1,"time_zone":"America/New_York"}`)
	if updatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("update settings status=%d body=%s", updatedResponse.StatusCode, responseBody(t, updatedResponse))
	}
	var updated site.Settings
	decodeResponseJSON(t, updatedResponse, &updated)
	if initial.Revision != 1 || updated.Revision != 2 || updated.TimeZone != "America/New_York" {
		t.Fatalf("initial=%+v updated=%+v", initial, updated)
	}

	stale := putAdminJSON(t, client, server.URL+"/admin/api/system/settings", csrf,
		`{"expected_revision":1,"time_zone":"Asia/Taipei"}`)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status=%d body=%s", stale.StatusCode, responseBody(t, stale))
	}
	_ = responseBody(t, stale)
	invalid := putAdminJSON(t, client, server.URL+"/admin/api/system/settings", csrf,
		`{"expected_revision":2,"time_zone":"Not/A_Time_Zone"}`)
	if invalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid update status=%d body=%s", invalid.StatusCode, responseBody(t, invalid))
	}
	_ = responseBody(t, invalid)
}
