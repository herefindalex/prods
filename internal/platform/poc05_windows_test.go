//go:build poc && windows

package platform

import "testing"

func TestPOC05WindowsRuntimeNotRun(t *testing.T) {
	t.Skip("requires an explicitly provisioned real Windows host and Windows service manager; cross-compilation is not runtime evidence")
}
