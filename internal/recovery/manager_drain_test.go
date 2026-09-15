package recovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackupManagerBeginDrainRejectsNewOperations(t *testing.T) {
	manager := &BackupManager{}
	manager.BeginDrain()
	if _, err := manager.RunManual(t.Context()); !errors.Is(err, ErrBackupManagerClosed) {
		t.Fatalf("manual backup after drain error=%v", err)
	}
	if _, err := manager.RunDue(t.Context(), time.Now()); !errors.Is(err, ErrBackupManagerClosed) {
		t.Fatalf("scheduled backup after drain error=%v", err)
	}
	manager.Close()
}

func TestAssetGCManagerBeginDrainRejectsNewCycle(t *testing.T) {
	manager := &AssetGCManager{}
	manager.BeginDrain()
	if _, err := manager.RunOnce(t.Context(), time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("asset GC after drain error=%v", err)
	}
	manager.Close()
}
