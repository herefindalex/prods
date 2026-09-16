//go:build windows

package publishing

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

var windowsRenameRetryDelays = []time.Duration{
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	200 * time.Millisecond,
	400 * time.Millisecond,
	800 * time.Millisecond,
	1 * time.Second,
	1 * time.Second,
	1 * time.Second,
}

func renameStagedDirectory(ctx context.Context, source, destination string) error {
	return renameWithRetry(ctx, source, destination, windowsRenameRetryDelays, os.Rename, isRetryableWindowsRenameError)
}

func isRetryableWindowsRenameError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
