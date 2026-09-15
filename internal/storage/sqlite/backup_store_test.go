package sqlite

import (
	"errors"
	"testing"
	"time"

	"prods/internal/catalog"
)

func TestBackupSettingsAndRunReceiptsAreDurableAndRevisionChecked(t *testing.T) {
	store, owner := installedStore(t)
	settings, err := store.BackupSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || settings.LocalTime != "03:00" || settings.TimeZone != "UTC" ||
		settings.RetentionDaily != 7 || settings.RetentionWeekly != 8 || settings.RetentionMonthly != 12 ||
		settings.RetentionPreUpgrade != 3 || settings.RetentionPreRestore != 3 || settings.Version != 1 {
		t.Fatalf("default backup settings = %+v", settings)
	}
	settings.LocalTime = "04:30"
	updated, err := store.UpdateBackupSettings(t.Context(), owner.ID, settings.Version, settings)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LocalTime != "04:30" || updated.Version != 2 {
		t.Fatalf("updated backup settings = %+v", updated)
	}
	if _, err := store.UpdateBackupSettings(t.Context(), owner.ID, settings.Version, settings); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale settings update = %v", err)
	}

	now := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	run, claimed, err := store.ClaimBackupRun(t.Context(), "scheduled", "2026-09-14T03:00:00Z", now)
	if err != nil || !claimed {
		t.Fatalf("claim scheduled backup = %+v %v %v", run, claimed, err)
	}
	if _, claimed, err := store.ClaimBackupRun(t.Context(), "scheduled", "2026-09-14T03:00:00Z", now); err != nil || claimed {
		t.Fatalf("duplicate scheduled claim = %v %v", claimed, err)
	}
	if err := store.CompleteBackupRun(t.Context(), run.ID, "backup-1", 1234, true, true, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	running, claimed, err := store.ClaimBackupRun(t.Context(), "manual", "", now.Add(2*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim manual backup = %+v %v %v", running, claimed, err)
	}
	if err := store.ReconcileBackupRuns(t.Context(), now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	runs, err := store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].Status != "failed" || runs[0].ErrorMessage == "" ||
		runs[1].Status != "succeeded" || runs[1].BackupID != "backup-1" || !runs[1].ContentVerified || !runs[1].ReadOnlyApplied {
		t.Fatalf("backup runs = %+v", runs)
	}
}
