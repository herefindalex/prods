package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeLogRotatesAndKeepsPrivateBoundedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "logs")
	logger, err := OpenRuntimeLog(root, 32, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"first-record\n", "second-record-is-long\n", "third-record\n", "fourth-record\n"} {
		if _, err := logger.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 3 {
		t.Fatalf("runtime log retained %d files, want at most 3", len(entries))
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("runtime log %s mode=%o", entry.Name(), info.Mode().Perm())
		}
		if info.Size() > 32 {
			t.Fatalf("runtime log %s size=%d", entry.Name(), info.Size())
		}
	}
}

func TestRuntimeLogDropsOversizedRecordWithoutRetainingPayload(t *testing.T) {
	root := t.TempDir()
	logger, err := OpenRuntimeLog(root, 80, 1)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("secret-payload", 20)
	if written, err := logger.Write([]byte(secret)); err != nil || written != len(secret) {
		t.Fatalf("oversized write=%d err=%v", written, err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "prods.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secret) || !strings.Contains(string(body), "exceeded size limit") {
		t.Fatalf("oversized runtime log body=%q", body)
	}
}

func TestRuntimeLogRejectsSymlinkAndRestrictsExistingFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prods.log")
	if err := os.WriteFile(path, []byte("existing\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	logger, err := OpenRuntimeLog(root, 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("existing runtime log mode=%#o", info.Mode().Perm())
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "secret")
	if err := os.WriteFile(secret, []byte("do-not-append"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, path); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("symlinks are not available")
		}
		t.Fatal(err)
	}
	if _, err := OpenRuntimeLog(root, 1024, 1); err == nil {
		t.Fatal("runtime log writer accepted a symlink")
	}
	body, err := os.ReadFile(secret)
	if err != nil || string(body) != "do-not-append" {
		t.Fatalf("symlink target changed: %q, %v", body, err)
	}
}
