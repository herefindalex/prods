package platform

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestResourceGateAccountsForSharedVolumeReservations(t *testing.T) {
	gate := NewResourceGate(func(string) (ResourceStats, error) {
		return ResourceStats{VolumeID: "volume-a", FreeBytes: 100, FreeInodes: 10, SupportsInodes: true}, nil
	})
	first, _, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "import", Path: "/data/work", RequiredBytes: 60, RequiredInodes: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, snapshot, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "backup", Path: "/same-volume/backups", RequiredBytes: 35, ByteHeadroom: 10,
	}); !errors.Is(err, ErrResourceCritical) || snapshot.ReservedBytes != 60 {
		t.Fatalf("second admission error=%v snapshot=%+v", err, snapshot)
	}
	first.Release()
	first.Release()
	second, snapshot, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "backup", Path: "/same-volume/backups", RequiredBytes: 35, ByteHeadroom: 10,
	})
	if err != nil || snapshot.ReservedBytes != 0 {
		t.Fatalf("admission after release error=%v snapshot=%+v", err, snapshot)
	}
	second.Release()
}

func TestResourceGateChecksInodesOverflowProbeAndContext(t *testing.T) {
	gate := NewResourceGate(func(string) (ResourceStats, error) {
		return ResourceStats{VolumeID: "volume-a", FreeBytes: 1000, FreeInodes: 1, SupportsInodes: true}, nil
	})
	if _, _, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "assets", Path: "/assets", RequiredBytes: 1, RequiredInodes: 1, InodeHeadroom: 1,
	}); !errors.Is(err, ErrResourceCritical) {
		t.Fatalf("inode admission error = %v", err)
	}
	if _, _, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "overflow", Path: "/assets", RequiredBytes: ^uint64(0), ByteHeadroom: 1,
	}); !errors.Is(err, ErrResourceCritical) {
		t.Fatalf("overflow admission error = %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := gate.Admit(cancelled, ResourceRequest{Operation: "cancelled", Path: "/assets"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled admission error = %v", err)
	}
	probeFailure := NewResourceGate(func(string) (ResourceStats, error) { return ResourceStats{}, errors.New("probe failed") })
	if _, _, err := probeFailure.Admit(t.Context(), ResourceRequest{Operation: "backup", Path: "/backup"}); !errors.Is(err, ErrResourceUnavailable) {
		t.Fatalf("probe error = %v", err)
	}
}

func TestResourceGateEnforcesPercentageAndAbsoluteHeadroom(t *testing.T) {
	gate := NewResourceGate(func(string) (ResourceStats, error) {
		return ResourceStats{VolumeID: "volume-a", TotalBytes: 1_000, FreeBytes: 150}, nil
	})

	if _, _, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "percentage", Path: "/data", RequiredBytes: 60, ByteHeadroom: 20, MinFreePercent: 10,
	}); !errors.Is(err, ErrResourceCritical) {
		t.Fatalf("percentage admission error = %v", err)
	}
	reservation, _, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "absolute", Path: "/data", RequiredBytes: 60, ByteHeadroom: 20, MinFreePercent: 5,
	})
	if err != nil {
		t.Fatalf("absolute admission: %v", err)
	}
	reservation.Release()

	unknownTotal := NewResourceGate(func(string) (ResourceStats, error) {
		return ResourceStats{VolumeID: "volume-b", FreeBytes: 1_000}, nil
	})
	if _, _, err := unknownTotal.Admit(t.Context(), ResourceRequest{
		Operation: "percentage", Path: "/data", MinFreePercent: 1,
	}); !errors.Is(err, ErrResourceUnavailable) {
		t.Fatalf("unknown total admission error = %v", err)
	}
	if _, _, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "invalid", Path: "/data", MinFreePercent: 101,
	}); !errors.Is(err, ErrResourceCritical) {
		t.Fatalf("invalid percentage admission error = %v", err)
	}
}

func TestResourceGateConcurrentAdmissionDoesNotOvercommit(t *testing.T) {
	gate := NewResourceGate(func(string) (ResourceStats, error) {
		return ResourceStats{VolumeID: "volume-a", FreeBytes: 100}, nil
	})
	start := make(chan struct{})
	release := make(chan struct{})
	results := make(chan bool, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			reservation, _, err := gate.Admit(t.Context(), ResourceRequest{
				Operation: "concurrent", Path: "/data", RequiredBytes: 60,
			})
			if err != nil {
				if errors.Is(err, ErrResourceCritical) {
					results <- false
					return
				}
				t.Errorf("unexpected admission error: %v", err)
				return
			}
			results <- true
			<-release
			reservation.Release()
		}()
	}
	close(start)
	admitted, rejected := 0, 0
	for range 2 {
		if <-results {
			admitted++
		} else {
			rejected++
		}
	}
	close(release)
	wait.Wait()
	if admitted != 1 || rejected != 1 {
		t.Fatalf("admitted=%d rejected=%d", admitted, rejected)
	}
}

func TestDefaultResourceProbeSupportsExistingAncestor(t *testing.T) {
	gate := NewResourceGate(nil)
	reservation, snapshot, err := gate.Admit(t.Context(), ResourceRequest{
		Operation: "small-write", Path: t.TempDir() + "/not-created/yet", RequiredBytes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reservation.Release()
	if snapshot.VolumeID == "" || snapshot.FreeBytes == 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
