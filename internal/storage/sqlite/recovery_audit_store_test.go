package sqlite

import "testing"

func TestRecoveryAuditImportIsIdempotentAndUsesSystemActor(t *testing.T) {
	store, _ := installedStore(t)
	eventID := "0123456789abcdef0123456789abcdef"
	operationID := "abcdef0123456789abcdef0123456789"
	for attempt := 0; attempt < 2; attempt++ {
		if err := store.ImportRecoveryAudit(t.Context(), eventID, operationID, "restore.completed", "backup-1", "success", "2026-09-14T12:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	var actor any
	var details string
	if err := store.db.QueryRow(`SELECT COUNT(*),actor_id,details_json FROM admin_log WHERE id=?`, "log_recovery_"+eventID).
		Scan(&count, &actor, &details); err != nil {
		t.Fatal(err)
	}
	if count != 1 || actor != nil || details == "" {
		t.Fatalf("recovery audit count=%d actor=%v details=%s", count, actor, details)
	}
}
