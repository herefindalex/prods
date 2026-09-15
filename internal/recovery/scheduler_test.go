package recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"prods/internal/platform"
	storesqlite "prods/internal/storage/sqlite"
)

func TestBackupManagerRunsOneCurrentScheduleAndPersistsReceipt(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	assetRoot := filepath.Join(root, "data", "assets")
	backupRoot := filepath.Join(root, "backups")
	hostConfigPath := filepath.Join(root, "runtime", "prods.ini")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hostConfigPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostConfigPath, []byte("listen=:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { makeBackupTreeWritable(backupRoot) })
	now := time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
	manager, err := NewBackupManager(store, BackupManagerConfig{
		DatabasePath: databasePath, AssetDir: assetRoot, BackupDir: backupRoot, ApplicationVersion: "test",
		Files: map[string]string{"host-config": hostConfigPath},
		ResourceGate: platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
			return platform.ResourceStats{
				VolumeID: "backup", TotalBytes: 1 << 40, FreeBytes: 1 << 39,
				FreeInodes: 1_000_000, SupportsInodes: true,
			}, nil
		}),
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	ran, err := manager.RunDue(t.Context(), now)
	if err != nil || !ran {
		t.Fatalf("scheduled backup = %v, %v", ran, err)
	}
	if ran, err := manager.RunDue(t.Context(), now.Add(time.Hour)); err != nil || ran {
		t.Fatalf("duplicate current-day backup = %v, %v", ran, err)
	}
	runs, err := store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Kind != "scheduled" || runs[0].Status != "succeeded" || runs[0].BackupID == "" ||
		!runs[0].ContentVerified || !runs[0].ReadOnlyApplied || runs[0].SizeBytes <= 0 {
		t.Fatalf("scheduled run receipt = %+v", runs)
	}
	manifest, err := LoadManifest(filepath.Join(backupRoot, runs[0].BackupID))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.RunID != runs[0].ID || manifest.Kind != "scheduled" || !manifest.ContentVerified || !manifest.ReadOnlyApplied {
		t.Fatalf("scheduled manifest = %+v", manifest)
	}
	entry, ok := manifest.Files["host-config"]
	if !ok || entry.Path != "files/host-config" {
		t.Fatalf("scheduled host config entry = %+v", entry)
	}
	if body, err := os.ReadFile(filepath.Join(backupRoot, runs[0].BackupID, filepath.FromSlash(entry.Path))); err != nil || string(body) != "listen=:8080\n" {
		t.Fatalf("scheduled host config = %q, %v", body, err)
	}
}

func TestBackupManagerReconcilesPublishedManifestBeforeRunReceipt(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	assetRoot := filepath.Join(root, "data", "assets")
	backupRoot := filepath.Join(root, "backups")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	run, claimed, err := store.ClaimBackupRun(t.Context(), "manual", "", time.Now())
	if err != nil || !claimed {
		t.Fatalf("claim run = %+v %v %v", run, claimed, err)
	}
	manifest, err := CreateBackup(t.Context(), store, BackupConfig{
		BackupDir: backupRoot, AssetDir: assetRoot, ApplicationVersion: "test", Kind: "manual", RunID: run.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewBackupManager(store, BackupManagerConfig{
		DatabasePath: databasePath, AssetDir: assetRoot, BackupDir: backupRoot,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	runs, err := store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "succeeded" || runs[0].BackupID != manifest.ID || !runs[0].ContentVerified {
		t.Fatalf("reconciled run = %+v", runs)
	}
}

func TestBackupManagerDoesNotStartDueWorkWhenReconciliationIsUnresolved(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	backupRoot := filepath.Join(root, "backup-root-is-a-file")
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := os.WriteFile(backupRoot, []byte("unavailable directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := store.ClaimBackupRun(t.Context(), "manual", "", time.Now()); err != nil || !claimed {
		t.Fatalf("claim interrupted run: claimed=%v err=%v", claimed, err)
	}
	manager, err := NewBackupManager(store, BackupManagerConfig{
		DatabasePath: databasePath,
		AssetDir:     filepath.Join(root, "assets"),
		BackupDir:    backupRoot,
		Now: func() time.Time {
			return time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.runCycle(t.Context())
	health := manager.RuntimeHealth()
	if health.ConsecutiveFailures != 1 || health.FailureStage != "reconcile" || health.LastFailureUTC.IsZero() {
		t.Fatalf("unresolved reconciliation runtime health=%+v", health)
	}
	runs, err := store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Kind != "manual" || runs[0].Status != "running" {
		t.Fatalf("unresolved reconciliation admitted new work: %+v", runs)
	}
}

func TestBackupScheduleStateShowsOverdueAndNextLocalDay(t *testing.T) {
	settings := storesqlite.BackupSettings{Enabled: true, LocalTime: "03:00", TimeZone: "America/New_York"}
	now := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	slot, due, err := BackupScheduleState(now, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slot != "2026-09-14T07:00:00Z" || !due {
		t.Fatalf("overdue state = %q due=%v", slot, due)
	}
	runs := []storesqlite.BackupRun{{Kind: "scheduled", ScheduledFor: slot, Status: "failed"}}
	next, due, err := BackupScheduleState(now, settings, runs)
	if err != nil {
		t.Fatal(err)
	}
	if next != "2026-09-15T07:00:00Z" || due {
		t.Fatalf("next state = %q due=%v", next, due)
	}
}

func TestBackupManagerCloseCancelsScheduledRunAndPersistsFailure(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	assetRoot := filepath.Join(root, "data", "assets")
	backupRoot := filepath.Join(root, "backups")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)
	manager, err := NewBackupManager(store, BackupManagerConfig{
		DatabasePath: databasePath,
		AssetDir:     assetRoot,
		BackupDir:    backupRoot,
		Now:          func() time.Time { return now },
		PollInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var once sync.Once
	manager.createBackup = func(ctx context.Context, _ Snapshotter, _ BackupConfig) (Manifest, error) {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return Manifest{}, ctx.Err()
	}
	manager.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled backup did not start")
	}
	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("backup manager did not stop after cancellation")
	}

	runs, err := store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "failed" || !strings.Contains(runs[0].ErrorMessage, "canceled") {
		t.Fatalf("shutdown backup receipt=%+v", runs)
	}
	if _, err := manager.RunManual(t.Context()); !errors.Is(err, ErrBackupManagerClosed) {
		t.Fatalf("manual backup after close error=%v", err)
	}
}

func TestBackupManagerCloseWaitsForAdmittedManualRun(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	manager, err := NewBackupManager(store, BackupManagerConfig{
		DatabasePath: databasePath,
		AssetDir:     filepath.Join(root, "assets"),
		BackupDir:    filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	manager.createBackup = func(context.Context, Snapshotter, BackupConfig) (Manifest, error) {
		close(started)
		<-release
		return Manifest{}, errors.New("forced manual backup failure")
	}
	runDone := make(chan error, 1)
	go func() {
		_, err := manager.RunManual(context.Background())
		runDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("manual backup did not start")
	}
	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("manager closed before admitted manual backup finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-runDone:
		if err == nil || !strings.Contains(err.Error(), "forced manual backup failure") {
			t.Fatalf("manual backup error=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual backup did not finish")
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not close after manual backup finished")
	}
}

func TestBackupManagerPersistsRetentionFailureAsWarning(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "data", "prods.db")
	assetRoot := filepath.Join(root, "data", "assets")
	backupRoot := filepath.Join(root, "backups")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storesqlite.CreatePOC(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	t.Cleanup(func() { makeBackupTreeWritable(backupRoot) })
	manager, err := NewBackupManager(store, BackupManagerConfig{
		DatabasePath:       databasePath,
		AssetDir:           assetRoot,
		BackupDir:          backupRoot,
		ApplicationVersion: "test",
		ResourceGate: platform.NewResourceGate(func(string) (platform.ResourceStats, error) {
			return platform.ResourceStats{
				VolumeID:       "test-volume",
				TotalBytes:     1 << 40,
				FreeBytes:      1 << 39,
				FreeInodes:     1_000_000,
				SupportsInodes: true,
			}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.retainBackups = func(context.Context, string, storesqlite.BackupSettings, time.Time) (RetentionResult, error) {
		return RetentionResult{}, errors.New("forced retention cleanup failure")
	}
	run, err := manager.RunManual(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "succeeded" || run.BackupID == "" || !run.ContentVerified ||
		!strings.Contains(run.WarningMessage, "forced retention cleanup failure") {
		t.Fatalf("backup with retention warning=%+v", run)
	}
	runs, err := store.RecentBackupRuns(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "succeeded" || runs[0].ErrorMessage != "" ||
		!strings.Contains(runs[0].WarningMessage, "forced retention cleanup failure") {
		t.Fatalf("durable retention warning=%+v", runs)
	}
}
