package sqlite

import (
	"errors"
	"strings"
	"testing"

	"prods/internal/identity"
	"prods/internal/site"
)

func TestSiteMaintenanceIsDurableRevisionedAuthorizedAndAudited(t *testing.T) {
	store, owner := installedStore(t)
	initial, err := store.SiteMaintenance(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if initial.Active || initial.Revision != 1 || initial.Message != "" {
		t.Fatalf("initial maintenance=%+v", initial)
	}
	active, err := store.UpdateSiteMaintenance(t.Context(), owner.ID, 1, true, "  Planned work <script>  ")
	if err != nil {
		t.Fatal(err)
	}
	if !active.Active || active.Message != "Planned work <script>" || active.Revision != 2 || active.UpdatedBy != owner.ID {
		t.Fatalf("active maintenance=%+v", active)
	}
	if _, err := store.UpdateSiteMaintenance(t.Context(), owner.ID, 1, false, ""); !errors.Is(err, site.ErrMaintenanceConflict) {
		t.Fatalf("stale maintenance update=%v", err)
	}

	now := "2026-09-14T00:00:00Z"
	if _, err := store.db.Exec(`INSERT INTO roles(id,name,system_key,status,revision,created_at,updated_at)
		VALUES('role_catalog','Catalog',NULL,'active',1,?,?);
		INSERT INTO role_capabilities(role_id,capability) VALUES('role_catalog',?);
		INSERT INTO users(id,email,email_normalized,display_name,role,status,password_hash,auth_revision,created_at,updated_at)
		VALUES('usr_catalog','catalog@example.test','catalog@example.test','Catalog','role_catalog','active','x',1,?,?)`,
		now, now, identity.CapabilityAdminAccess, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateSiteMaintenance(t.Context(), "usr_catalog", 2, false, ""); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("maintenance without system capability=%v", err)
	}
	unchanged, err := store.SiteMaintenance(t.Context())
	if err != nil || !unchanged.Active || unchanged.Revision != 2 {
		t.Fatalf("unauthorized update changed state=%+v err=%v", unchanged, err)
	}
	var details string
	if err := store.db.QueryRow(`SELECT details_json FROM admin_log WHERE action='site.maintenance_updated'`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(details, "Planned work") || !strings.Contains(details, `"active":true`) {
		t.Fatalf("maintenance audit details=%s", details)
	}
}
