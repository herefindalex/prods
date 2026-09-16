//go:build !windows

package publishing

import (
	"context"
	"os"
)

func renameStagedDirectory(_ context.Context, source, destination string) error {
	return os.Rename(source, destination)
}
