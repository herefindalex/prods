package recovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storesqlite "prods/internal/storage/sqlite"
)

func TestBackupRetentionSharesBucketsPreservesManualAndNeverDeletesLastGood(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	writeRetentionManifest(t, root, "scheduled-new", "scheduled", now.Add(-time.Hour), true)
	writeRetentionManifest(t, root, "scheduled-old-same-day", "scheduled", now.Add(-2*time.Hour), true)
	writeRetentionManifest(t, root, "scheduled-expired", "scheduled", now.AddDate(-2, 0, 0), false)
	writeRetentionManifest(t, root, "manual-old", "manual", now.AddDate(-5, 0, 0), false)
	writeRetentionManifest(t, root, "upgrade-new", "pre-upgrade", now.Add(-3*time.Hour), false)
	writeRetentionManifest(t, root, "upgrade-old", "pre-upgrade", now.AddDate(0, -2, 0), false)

	settings := storesqlite.BackupSettings{
		TimeZone: "UTC", RetentionDaily: 1, RetentionWeekly: 1, RetentionMonthly: 1,
		RetentionPreUpgrade: 1, RetentionPreRestore: 1,
	}
	result, err := ApplyBackupRetention(t.Context(), root, settings, now)
	if err != nil {
		t.Fatal(err)
	}
	reasons := result.Reasons["scheduled-new"]
	if len(reasons) != 3 || !containsRetentionReason(reasons, "daily:2026-09-14") ||
		!containsRetentionReason(reasons, "weekly:2026-W38") || !containsRetentionReason(reasons, "monthly:2026-09") {
		t.Fatalf("shared retention reasons = %v", reasons)
	}
	for _, id := range []string{"scheduled-old-same-day", "scheduled-expired", "upgrade-old"} {
		if _, err := os.Stat(filepath.Join(root, id)); !os.IsNotExist(err) {
			t.Fatalf("expired backup %s still exists: %v", id, err)
		}
	}
	for _, id := range []string{"scheduled-new", "manual-old", "upgrade-new"} {
		if _, err := os.Stat(filepath.Join(root, id)); err != nil {
			t.Fatalf("retained backup %s: %v", id, err)
		}
	}
	if len(result.Deleted) != 3 {
		t.Fatalf("deleted backups = %v", result.Deleted)
	}

	lastRoot := t.TempDir()
	writeRetentionManifest(t, lastRoot, "only-expired", "scheduled", now.AddDate(-3, 0, 0), true)
	last, err := ApplyBackupRetention(t.Context(), lastRoot, settings, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Deleted) != 0 || !containsRetentionReason(last.Reasons["only-expired"], "last-good") {
		t.Fatalf("last-good retention = %+v", last)
	}
}

func writeRetentionManifest(t *testing.T, root, id, kind string, created time.Time, readOnly bool) {
	t.Helper()
	directory := filepath.Join(root, id)
	if err := os.MkdirAll(filepath.Join(directory, "database"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		ManifestVersion: ManifestVersion, ID: id, Kind: kind, CreatedUTC: created.Format(time.RFC3339Nano),
		SQLiteVersion: "test", ContainsSensitiveData: true,
		SchemaVersion: 1, Database: ManifestFile{Path: "database/prods.db", SHA256: strings.Repeat("0", 64), Size: 1},
		Roots: map[string][]ManifestFile{}, ContentVerified: true, ReadOnlyApplied: readOnly,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "database", "prods.db"), []byte{0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if readOnly {
		_ = os.Chmod(filepath.Join(directory, "database", "prods.db"), 0o400)
		_ = os.Chmod(filepath.Join(directory, "database"), 0o500)
		_ = os.Chmod(filepath.Join(directory, "manifest.json"), 0o400)
		_ = os.Chmod(directory, 0o500)
		t.Cleanup(func() { makeBackupTreeWritable(root) })
	}
}

func containsRetentionReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
