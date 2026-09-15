//go:build poc && windows

package sqlite

import "time"

// POC-02 must collect Windows CPU evidence from a native Windows harness.
// Returning zero keeps cross-compilation honest without fabricating a metric.
func processCPU() time.Duration { return 0 }
