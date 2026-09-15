package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"prods/internal/identity"
	"prods/internal/site"
)

func TestSiteTimeZoneUpdateUsesRevisionAndAudit(t *testing.T) {
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail:       "owner@example.test",
		PasswordHash:     password,
		DefaultLocale:    "en-US",
		SupportedLocales: []string{"en-US", "zh-TW"},
		TimeZone:         "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	settings, err := store.SiteSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Revision != 1 || settings.TimeZone != "UTC" || len(settings.SupportedLocales) != 2 {
		t.Fatalf("initial settings = %+v", settings)
	}
	updated, err := store.UpdateSiteTimeZone(t.Context(), owner.ID, settings.Revision, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.TimeZone != "America/New_York" || updated.DefaultLocale != "en-US" {
		t.Fatalf("updated settings = %+v", updated)
	}
	if _, err := store.UpdateSiteTimeZone(t.Context(), owner.ID, settings.Revision, "Asia/Taipei"); !errors.Is(err, site.ErrSettingsConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	if _, err := store.UpdateSiteTimeZone(t.Context(), owner.ID, updated.Revision, "Not/A_Time_Zone"); !errors.Is(err, site.ErrInvalidSettings) {
		t.Fatalf("invalid update error = %v", err)
	}

	var auditCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='site.time_zone_updated' AND target_id='site'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("time-zone audit count = %d", auditCount)
	}
}
