package recovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"prods/internal/platform"
	"prods/internal/storage/sqlite"
)

const (
	DefaultAssetGCGracePeriod = 7 * 24 * time.Hour
	DefaultAssetGCInterval    = 6 * time.Hour
	defaultAssetGCBatchSize   = 128
	assetGCHistoryRetention   = 30 * 24 * time.Hour
)

type AssetGCStore interface {
	ClearAssetPins(context.Context) error
	PlanAssetGarbageCollection(context.Context, time.Time, time.Duration, int) (int, int, error)
	PendingAssetFileDeletions(context.Context, int) ([]sqlite.AssetFileDeletion, error)
	RecordAssetFileDeletion(context.Context, string, error) error
	PurgeAssetDeletionHistory(context.Context, time.Time) error
}

type AssetGCConfig struct {
	AssetDir   string
	Grace      time.Duration
	Interval   time.Duration
	BatchSize  int
	OnFailure  func(error)
	OnProgress func(AssetGCResult)
}

type AssetGCResult struct {
	Marked         int
	Planned        int
	FilesDeleted   int
	DeletionFailed int
}

type AssetGCManager struct {
	store       AssetGCStore
	config      AssetGCConfig
	health      *platform.RuntimeHealthTracker
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	once        sync.Once
	draining    atomic.Bool
	lifecycleMu sync.Mutex
}

func NewAssetGCManager(ctx context.Context, store AssetGCStore, config AssetGCConfig) (*AssetGCManager, error) {
	if store == nil || strings.TrimSpace(config.AssetDir) == "" {
		return nil, errors.New("asset GC requires a store and asset directory")
	}
	if config.Grace <= 0 {
		config.Grace = DefaultAssetGCGracePeriod
	}
	if config.Interval <= 0 {
		config.Interval = DefaultAssetGCInterval
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaultAssetGCBatchSize
	}
	config.AssetDir = filepath.Clean(config.AssetDir)
	if err := store.ClearAssetPins(ctx); err != nil {
		return nil, fmt.Errorf("clear stale asset backup pins: %w", err)
	}
	return &AssetGCManager{
		store: store, config: config,
		health: platform.NewRuntimeHealthTracker(nil),
	}, nil
}

func (manager *AssetGCManager) Start(parent context.Context) {
	if manager == nil {
		return
	}
	manager.once.Do(func() {
		manager.lifecycleMu.Lock()
		defer manager.lifecycleMu.Unlock()
		if manager.draining.Load() {
			return
		}
		ctx, cancel := context.WithCancel(parent)
		manager.cancel = cancel
		manager.wg.Add(1)
		manager.health.Started()
		go func() {
			defer manager.wg.Done()
			defer manager.health.Stopped()
			manager.run(ctx)
		}()
	})
}

func (manager *AssetGCManager) Close() {
	if manager == nil {
		return
	}
	manager.BeginDrain()
	manager.wg.Wait()
}

func (manager *AssetGCManager) BeginDrain() {
	if manager == nil {
		return
	}
	manager.draining.Store(true)
	manager.lifecycleMu.Lock()
	cancel := manager.cancel
	manager.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (manager *AssetGCManager) run(ctx context.Context) {
	manager.runCycle(ctx)
	ticker := time.NewTicker(manager.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			manager.runCycle(ctx)
		}
	}
}

func (manager *AssetGCManager) runCycle(ctx context.Context) {
	result, err := manager.RunOnce(ctx, time.Now().UTC())
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			manager.health.Failed("garbage_collection")
			slog.Error("asset garbage collection failed", "error", err)
			if manager.config.OnFailure != nil {
				manager.config.OnFailure(err)
			}
		}
		return
	}
	manager.health.Succeeded()
	if manager.config.OnProgress != nil && (result.Marked != 0 || result.Planned != 0 || result.FilesDeleted != 0 || result.DeletionFailed != 0) {
		manager.config.OnProgress(result)
	}
}

func (manager *AssetGCManager) RuntimeHealth() platform.RuntimeHealthSnapshot {
	if manager == nil {
		return platform.RuntimeHealthSnapshot{}
	}
	return manager.health.Snapshot()
}

func (manager *AssetGCManager) RunOnce(ctx context.Context, now time.Time) (AssetGCResult, error) {
	if manager == nil {
		return AssetGCResult{}, errors.New("asset GC manager is nil")
	}
	if manager.draining.Load() {
		return AssetGCResult{}, context.Canceled
	}
	marked, planned, err := manager.store.PlanAssetGarbageCollection(ctx, now, manager.config.Grace, manager.config.BatchSize)
	if err != nil {
		return AssetGCResult{}, err
	}
	result := AssetGCResult{Marked: marked, Planned: planned}
	pending, err := manager.store.PendingAssetFileDeletions(ctx, manager.config.BatchSize)
	if err != nil {
		return result, err
	}
	for _, item := range pending {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		deletionErr := removeManagedAssetFile(manager.config.AssetDir, item.StoragePath)
		if err := manager.store.RecordAssetFileDeletion(ctx, item.AssetID, deletionErr); err != nil {
			return result, err
		}
		if deletionErr != nil {
			result.DeletionFailed++
		} else {
			result.FilesDeleted++
		}
	}
	if err := manager.store.PurgeAssetDeletionHistory(ctx, now.Add(-assetGCHistoryRetention)); err != nil {
		return result, err
	}
	return result, nil
}

func removeManagedAssetFile(assetRoot, storagePath string) error {
	rootInfo, err := os.Stat(assetRoot)
	if err != nil {
		return fmt.Errorf("asset root unavailable: %w", err)
	}
	if !rootInfo.IsDir() {
		return errors.New("asset root is not a directory")
	}
	resolvedRoot, err := filepath.EvalSymlinks(assetRoot)
	if err != nil {
		return fmt.Errorf("resolve asset root: %w", err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return err
	}

	clean := filepath.Clean(strings.TrimSpace(storagePath))
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("unsafe asset storage path")
	}
	target := filepath.Join(resolvedRoot, clean)
	if !pathInside(resolvedRoot, target) {
		return errors.New("asset storage path escapes root")
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("managed asset target is not a regular file")
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return err
	}
	resolvedTarget := filepath.Join(resolvedParent, filepath.Base(target))
	if !pathInside(resolvedRoot, resolvedTarget) {
		return errors.New("resolved asset storage path escapes root")
	}
	if err := os.Remove(resolvedTarget); err != nil {
		return err
	}
	// Asset-specific directories are removed only when already empty. Never use
	// recursive deletion for paths derived from database metadata.
	_ = os.Remove(resolvedParent)
	return nil
}

func pathInside(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
