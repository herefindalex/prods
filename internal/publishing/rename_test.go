package publishing

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRenameWithRetryRecoversFromTransientFailure(t *testing.T) {
	transient := errors.New("transient rename failure")
	attempts := 0
	err := renameWithRetry(
		context.Background(),
		"stage",
		"unit",
		[]time.Duration{0, 0, 0},
		func(source, destination string) error {
			attempts++
			if source != "stage" || destination != "unit" {
				t.Fatalf("rename paths = %q, %q", source, destination)
			}
			if attempts < 3 {
				return transient
			}
			return nil
		},
		func(err error) bool { return errors.Is(err, transient) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("rename attempts = %d, want 3", attempts)
	}
}

func TestRenameWithRetryStopsForPermanentOrExhaustedFailure(t *testing.T) {
	transient := errors.New("transient rename failure")
	permanent := errors.New("permanent rename failure")

	t.Run("permanent", func(t *testing.T) {
		attempts := 0
		err := renameWithRetry(
			context.Background(), "stage", "unit", []time.Duration{0, 0},
			func(string, string) error { attempts++; return permanent },
			func(err error) bool { return errors.Is(err, transient) },
		)
		if !errors.Is(err, permanent) || attempts != 1 {
			t.Fatalf("rename error = %v, attempts = %d", err, attempts)
		}
	})

	t.Run("exhausted", func(t *testing.T) {
		attempts := 0
		err := renameWithRetry(
			context.Background(), "stage", "unit", []time.Duration{0, 0},
			func(string, string) error { attempts++; return transient },
			func(err error) bool { return errors.Is(err, transient) },
		)
		if !errors.Is(err, transient) || attempts != 3 {
			t.Fatalf("rename error = %v, attempts = %d", err, attempts)
		}
	})
}

func TestRenameWithRetryHonorsCancellation(t *testing.T) {
	transient := errors.New("transient rename failure")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	err := renameWithRetry(
		ctx, "stage", "unit", []time.Duration{time.Hour},
		func(string, string) error { attempts++; return transient },
		func(error) bool { return true },
	)
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("rename error = %v, attempts = %d", err, attempts)
	}
}
