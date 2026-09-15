package recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"prods/internal/platform"
	"prods/internal/storage/sqlite"
)

var ErrBackupManagerClosed = errors.New("backup manager is closed")

type BackupManagerConfig struct {
	DatabasePath         string
	BackupDir            string
	AssetDir             string
	Roots                map[string]string
	Files                map[string]string
	ApplicationVersion   string
	ExternalRequirements []string
	ResourceGate         *platform.ResourceGate
	ByteHeadroom         uint64
	InodeHeadroom        uint64
	MinFreePercent       uint8
	PollInterval         time.Duration
	Now                  func() time.Time
}

type BackupManager struct {
	store         *sqlite.Store
	config        BackupManagerConfig
	health        *platform.RuntimeHealthTracker
	mu            sync.Mutex
	lifecycleMu   sync.Mutex
	closing       bool
	cancel        context.CancelFunc
	schedulerWG   sync.WaitGroup
	operationWG   sync.WaitGroup
	createBackup  func(context.Context, Snapshotter, BackupConfig) (Manifest, error)
	retainBackups func(context.Context, string, sqlite.BackupSettings, time.Time) (RetentionResult, error)
}

func NewBackupManager(store *sqlite.Store, config BackupManagerConfig) (*BackupManager, error) {
	if store == nil || strings.TrimSpace(config.DatabasePath) == "" || strings.TrimSpace(config.BackupDir) == "" || strings.TrimSpace(config.AssetDir) == "" {
		return nil, ErrInvalidBackup
	}
	if config.ResourceGate == nil {
		config.ResourceGate = platform.NewResourceGate(nil)
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Minute
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &BackupManager{
		store:         store,
		config:        config,
		health:        platform.NewRuntimeHealthTracker(config.Now),
		createBackup:  CreateBackup,
		retainBackups: ApplyBackupRetention,
	}, nil
}

func (manager *BackupManager) Start(parent context.Context) {
	if manager == nil {
		return
	}
	manager.lifecycleMu.Lock()
	if manager.closing || manager.cancel != nil {
		manager.lifecycleMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	manager.cancel = cancel
	manager.schedulerWG.Add(1)
	manager.health.Started()
	manager.lifecycleMu.Unlock()
	go func() {
		defer manager.schedulerWG.Done()
		defer manager.health.Stopped()
		manager.runCycle(ctx)
		ticker := time.NewTicker(manager.config.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				manager.runCycle(ctx)
			}
		}
	}()
}

func (manager *BackupManager) runCycle(ctx context.Context) {
	if err := manager.Reconcile(ctx); err != nil {
		if !errors.Is(err, context.Canceled) {
			manager.health.Failed("reconcile")
		}
		return
	}
	if _, err := manager.RunDue(ctx, manager.config.Now()); err != nil {
		if !errors.Is(err, context.Canceled) {
			manager.health.Failed("scheduled_backup")
		}
		return
	}
	manager.health.Succeeded()
}

func (manager *BackupManager) RuntimeHealth() platform.RuntimeHealthSnapshot {
	if manager == nil {
		return platform.RuntimeHealthSnapshot{}
	}
	return manager.health.Snapshot()
}

func (manager *BackupManager) Close() {
	if manager == nil {
		return
	}
	manager.BeginDrain()
	manager.schedulerWG.Wait()
	manager.operationWG.Wait()
}

// BeginDrain prevents new scheduled or manual operations and cooperatively
// cancels the scheduler without waiting for already-admitted operations.
func (manager *BackupManager) BeginDrain() {
	if manager == nil {
		return
	}
	manager.lifecycleMu.Lock()
	manager.closing = true
	cancel := manager.cancel
	if cancel != nil {
		cancel()
	}
	manager.lifecycleMu.Unlock()
}

func (manager *BackupManager) RunDue(ctx context.Context, now time.Time) (bool, error) {
	if err := manager.beginOperation(); err != nil {
		return false, err
	}
	defer manager.operationWG.Done()
	settings, err := manager.store.BackupSettings(ctx)
	if err != nil || !settings.Enabled {
		return false, err
	}
	scheduledFor, due, err := backupScheduleSlot(now, settings)
	if err != nil || !due {
		return false, err
	}
	return manager.run(ctx, "scheduled", scheduledFor)
}

func (manager *BackupManager) RunManual(ctx context.Context) (sqlite.BackupRun, error) {
	if err := manager.beginOperation(); err != nil {
		return sqlite.BackupRun{}, err
	}
	defer manager.operationWG.Done()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	run, claimed, err := manager.store.ClaimBackupRun(ctx, "manual", "", manager.config.Now())
	if err != nil {
		return sqlite.BackupRun{}, err
	}
	if !claimed {
		return sqlite.BackupRun{}, errors.New("manual backup was not admitted")
	}
	return manager.execute(ctx, run)
}

func (manager *BackupManager) run(ctx context.Context, kind, scheduledFor string) (bool, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	run, claimed, err := manager.store.ClaimBackupRun(ctx, kind, scheduledFor, manager.config.Now())
	if err != nil || !claimed {
		return false, err
	}
	_, err = manager.execute(ctx, run)
	return true, err
}

func (manager *BackupManager) execute(ctx context.Context, run sqlite.BackupRun) (sqlite.BackupRun, error) {
	requiredBytes, requiredInodes, err := EstimateBackupResourcesWithFiles(ctx, manager.config.DatabasePath, manager.config.AssetDir, manager.config.Roots, manager.config.Files)
	published := false
	if err == nil {
		var manifest Manifest
		manifest, err = manager.createBackup(ctx, manager.store, BackupConfig{
			BackupDir: manager.config.BackupDir, AssetDir: manager.config.AssetDir, Roots: manager.config.Roots, Files: manager.config.Files,
			ApplicationVersion: manager.config.ApplicationVersion, Kind: run.Kind, RunID: run.ID,
			ExternalRequirements: manager.config.ExternalRequirements,
			ResourceGate:         manager.config.ResourceGate, RequiredBytes: requiredBytes, RequiredInodes: requiredInodes,
			ByteHeadroom: manager.config.ByteHeadroom, InodeHeadroom: manager.config.InodeHeadroom,
			MinFreePercent: manager.config.MinFreePercent, ApplyReadOnly: true,
		})
		if err == nil {
			published = true
			size, sizeErr := backupDirectorySize(filepath.Join(manager.config.BackupDir, manifest.ID))
			if sizeErr != nil {
				err = sizeErr
			} else {
				err = manager.store.CompleteBackupRun(ctx, run.ID, manifest.ID, size, manifest.ContentVerified, manifest.ReadOnlyApplied, manager.config.Now())
				if err == nil {
					run.Status = "succeeded"
					run.BackupID = manifest.ID
					run.SizeBytes = size
					run.ContentVerified = manifest.ContentVerified
					run.ReadOnlyApplied = manifest.ReadOnlyApplied
					run.CompletedAt = manager.config.Now().UTC().Format(time.RFC3339Nano)
					if warning := manager.applyRetention(ctx); warning != nil {
						run.WarningMessage = warning.Error()
						warningCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						_ = manager.store.SetBackupRunWarning(warningCtx, run.ID, warning)
						cancel()
					}
					return run, nil
				}
			}
		}
	}
	if published {
		return run, fmt.Errorf("backup completed but durable run receipt was not recorded: %w", err)
	}
	manager.recordFailure(run.ID, err)
	return run, err
}

func (manager *BackupManager) applyRetention(ctx context.Context) error {
	settings, err := manager.store.BackupSettings(ctx)
	if err != nil {
		return fmt.Errorf("load backup retention settings: %w", err)
	}
	_, err = manager.retainBackups(ctx, manager.config.BackupDir, settings, manager.config.Now())
	if err != nil {
		return fmt.Errorf("apply backup retention: %w", err)
	}
	return nil
}

func (manager *BackupManager) recordFailure(runID string, failure error) {
	if failure == nil {
		failure = ErrInvalidBackup
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = manager.store.FailBackupRun(ctx, runID, failure, manager.config.Now())
}

func (manager *BackupManager) Reconcile(ctx context.Context) error {
	if err := manager.beginOperation(); err != nil {
		return err
	}
	defer manager.operationWG.Done()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	running, err := manager.store.RunningBackupRuns(ctx)
	if err != nil || len(running) == 0 {
		return err
	}
	manifests, err := ListBackups(manager.config.BackupDir)
	if err != nil {
		return err
	}
	byRun := make(map[string]Manifest, len(manifests))
	for _, manifest := range manifests {
		if manifest.RunID != "" && manifest.ContentVerified {
			byRun[manifest.RunID] = manifest
		}
	}
	for _, run := range running {
		if manifest, ok := byRun[run.ID]; ok {
			size, sizeErr := backupDirectorySize(filepath.Join(manager.config.BackupDir, manifest.ID))
			if sizeErr == nil {
				if err := manager.store.CompleteBackupRun(ctx, run.ID, manifest.ID, size, true, manifest.ReadOnlyApplied, manager.config.Now()); err == nil {
					continue
				}
			}
		}
		if err := manager.store.FailBackupRun(ctx, run.ID, errors.New("process restarted before backup completion"), manager.config.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (manager *BackupManager) beginOperation() error {
	if manager == nil {
		return ErrBackupManagerClosed
	}
	manager.lifecycleMu.Lock()
	defer manager.lifecycleMu.Unlock()
	if manager.closing {
		return ErrBackupManagerClosed
	}
	manager.operationWG.Add(1)
	return nil
}

func backupScheduleSlot(now time.Time, settings sqlite.BackupSettings) (string, bool, error) {
	slot, err := localBackupSlot(now, settings, 0)
	if err != nil {
		return "", false, err
	}
	return slot.UTC().Format(time.RFC3339), !now.Before(slot), nil
}

func BackupScheduleState(now time.Time, settings sqlite.BackupSettings, runs []sqlite.BackupRun) (string, bool, error) {
	if !settings.Enabled {
		return "", false, nil
	}
	slot, err := localBackupSlot(now, settings, 0)
	if err != nil {
		return "", false, err
	}
	encoded := slot.UTC().Format(time.RFC3339)
	if now.Before(slot) {
		return encoded, false, nil
	}
	for _, run := range runs {
		if run.Kind == "scheduled" && run.ScheduledFor == encoded {
			next, err := localBackupSlot(now, settings, 1)
			if err != nil {
				return "", false, err
			}
			return next.UTC().Format(time.RFC3339), false, nil
		}
	}
	return encoded, true, nil
}

func localBackupSlot(now time.Time, settings sqlite.BackupSettings, addDays int) (time.Time, error) {
	location, err := time.LoadLocation(settings.TimeZone)
	if err != nil {
		return time.Time{}, fmt.Errorf("backup time zone %q: %w", settings.TimeZone, err)
	}
	clock, err := time.Parse("15:04", settings.LocalTime)
	if err != nil {
		return time.Time{}, err
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day()+addDays, clock.Hour(), clock.Minute(), 0, 0, location), nil
}

func backupDirectorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > int64(^uint64(0)>>1)-total {
			return ErrInvalidBackup
		}
		total += info.Size()
		return nil
	})
	return total, err
}
