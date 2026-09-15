//go:build linux

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseOwnershipIsExclusiveAndReusable(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "data", "prods.db")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireDatabase(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireDatabase(databasePath); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("second ownership = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireDatabase(databasePath)
	if err != nil {
		t.Fatalf("ownership after release = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseOwnershipCanonicalizesSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	realData := filepath.Join(root, "real-data")
	if err := os.Mkdir(realData, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias-data")
	if err := os.Symlink(realData, alias); err != nil {
		t.Fatal(err)
	}
	first, err := AcquireDatabase(filepath.Join(realData, "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := AcquireDatabase(filepath.Join(alias, "prods.db")); !errors.Is(err, ErrAlreadyOwned) {
		t.Fatalf("symlinked ownership = %v", err)
	}
}
