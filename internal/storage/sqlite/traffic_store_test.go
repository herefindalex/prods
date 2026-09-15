package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func TestTrafficSettingsAreVersionedValidatedAndAudited(t *testing.T) {
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
		OwnerEmail: "owner@example.test", PasswordHash: password,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	settings, err := store.TrafficSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.RFQLimit != 10 || settings.RFQWindowSeconds != 600 || settings.Version != 1 {
		t.Fatalf("default traffic settings = %+v", settings)
	}
	settings.RFQLimit = 3
	settings.RFQWindowSeconds = 120
	updated, err := store.UpdateTrafficSettings(t.Context(), owner.ID, settings.Version, settings)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RFQLimit != 3 || updated.RFQWindowSeconds != 120 || updated.Version != 2 || updated.UpdatedAt == "" {
		t.Fatalf("updated traffic settings = %+v", updated)
	}
	if _, err := store.UpdateTrafficSettings(t.Context(), owner.ID, settings.Version, settings); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale traffic update error = %v", err)
	}
	updated.RFQLimit = 0
	if _, err := store.UpdateTrafficSettings(t.Context(), owner.ID, updated.Version, updated); !errors.Is(err, ErrInvalidTrafficSettings) {
		t.Fatalf("invalid traffic update error = %v", err)
	}

	audit, err := store.ListAudit(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range audit {
		if entry.Action == "traffic.settings_updated" && entry.ActorID == owner.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("traffic settings audit missing: %+v", audit)
	}
}
