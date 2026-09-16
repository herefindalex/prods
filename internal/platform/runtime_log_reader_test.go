package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRuntimeLogTailIsBoundedAndSelectsOnlyGenerations(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prods.log")
	lines := make([]string, 0, 700)
	for index := 0; index < 700; index++ {
		lines = append(lines, `{"level":"INFO","message":"bounded line"}`)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("older\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tail, err := ReadRuntimeLogTail(path, 0, 2, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if tail.FileName != "prods.log" || len(tail.Lines) != MaxRuntimeLogTailLines || !tail.Truncated {
		t.Fatalf("tail = %+v", tail)
	}
	older, err := ReadRuntimeLogTail(path, 1, 2, 20)
	if err != nil || len(older.Lines) != 1 || older.Lines[0] != "older" || older.FileName != "prods.log.1" {
		t.Fatalf("older tail = %+v, %v", older, err)
	}
	if _, err := ReadRuntimeLogTail(path, 3, 2, 20); err == nil {
		t.Fatal("out-of-range generation was accepted")
	}
}

func TestReadRuntimeLogTailRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret")
	path := filepath.Join(root, "prods.log")
	if err := os.WriteFile(secret, []byte("do-not-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, path); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("symlinks are not available")
		}
		t.Fatal(err)
	}
	if _, err := ReadRuntimeLogTail(path, 0, 1, 20); err == nil {
		t.Fatal("runtime log symlink was accepted")
	}
}

func TestReadRuntimeLogContentReadsCompleteSelectedGeneration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prods.log")
	current := "first\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(current), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("older\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	content, err := ReadRuntimeLogContent(path, 0, 2, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if content.FileName != "prods.log" || content.Generation != 0 || content.Body != current || content.SizeBytes != int64(len(current)) {
		t.Fatalf("content = %+v", content)
	}

	older, err := ReadRuntimeLogContent(path, 1, 2, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if older.FileName != "prods.log.1" || older.Generation != 1 || older.Body != "older\n" {
		t.Fatalf("older content = %+v", older)
	}
	if _, err := ReadRuntimeLogContent(path, 3, 2, 1024); err == nil {
		t.Fatal("out-of-range generation accepted")
	}
}

func TestReadRuntimeLogContentRejectsUnsafeOrOversizedFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prods.log")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRuntimeLogContent(path, 0, 1, 4); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized runtime log error = %v", err)
	}

	secret := filepath.Join(root, "secret")
	linked := filepath.Join(root, "linked.log")
	if err := os.WriteFile(secret, []byte("do-not-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, linked); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("symlinks are not available")
		}
		t.Fatal(err)
	}
	if _, err := ReadRuntimeLogContent(linked, 0, 1, 1024); err == nil {
		t.Fatal("runtime log symlink was accepted")
	}
}
