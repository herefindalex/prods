//go:build windows

package recovery

import "testing"

func TestWindowsDirectorySyncSkipsUnsupportedDirectoryFlush(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
