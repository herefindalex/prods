package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInspectNeverCreatesMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	inspection := Inspect(path)
	if inspection.State != DatabaseFresh {
		t.Fatalf("state = %s, reason=%s", inspection.State, inspection.Reason)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection created missing database: %v", err)
	}
	if _, err := OpenReady(path); !errors.Is(err, ErrDatabaseNotReady) {
		t.Fatalf("OpenReady missing database = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenReady created missing database: %v", err)
	}
}

func TestExplicitCreateTransitionsInstallingToReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	inspection := Inspect(path)
	if inspection.State != DatabaseInstalling {
		t.Fatalf("state after Create = %s, reason=%s", inspection.State, inspection.Reason)
	}
	if _, err := OpenReady(path); !errors.Is(err, ErrDatabaseNotReady) {
		t.Fatalf("OpenReady installing database = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `UPDATE system_state SET installation_state='ready' WHERE singleton=1`); err != nil {
		t.Fatal(err)
	}
	inspection = Inspect(path)
	if inspection.State != DatabaseReady {
		t.Fatalf("state after completion = %s, reason=%s", inspection.State, inspection.Reason)
	}
	ready, err := OpenReady(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ready.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(path); !errors.Is(err, ErrDatabaseExists) {
		t.Fatalf("second Create = %v", err)
	}
}

func TestCreatePOCIsExplicitAndReady(t *testing.T) {
	path := filepath.Join(t.TempDir(), "poc.db")
	store, err := CreatePOC(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if inspection := Inspect(path); inspection.State != DatabaseReady || inspection.Kind != DatabaseKindPOC {
		t.Fatalf("CreatePOC state = %s, reason=%s", inspection.State, inspection.Reason)
	}
	count, err := store.ProductCount(context.Background())
	if err != nil || count == 0 {
		t.Fatalf("explicit POC fixtures count=%d err=%v", count, err)
	}
}

func TestExistingUnrecognizedOrDamagedDatabaseRequiresRecovery(t *testing.T) {
	t.Run("valid SQLite without installation marker", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unrecognized.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE user_data(value TEXT); INSERT INTO user_data VALUES('preserve me')`); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inspection := Inspect(path)
		if inspection.State != DatabaseRecovery {
			t.Fatalf("state = %s, reason=%s", inspection.State, inspection.Reason)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("inspection rewrote an unrecognized database")
		}
	})

	t.Run("damaged bytes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "damaged.db")
		body := []byte("not a sqlite database; must not become a new site")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		inspection := Inspect(path)
		if inspection.State != DatabaseRecovery {
			t.Fatalf("state = %s, reason=%s", inspection.State, inspection.Reason)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(body) {
			t.Fatal("inspection rewrote damaged database")
		}
	})

	t.Run("directory at database path", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "prods.db")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if inspection := Inspect(path); inspection.State != DatabaseRecovery {
			t.Fatalf("state = %s, reason=%s", inspection.State, inspection.Reason)
		}
	})
}
