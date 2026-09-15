package main

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"prods/internal/hostconfig"
)

func TestFreshInstallerSelectsAndPersistsNextPortOnlyForBuiltInDefault(t *testing.T) {
	occupied := listenBelowFallbackCeiling(t)
	defer occupied.Close()
	requested := occupied.Addr().String()
	_, requestedPortText, err := net.SplitHostPort(requested)
	if err != nil {
		t.Fatal(err)
	}
	requestedPort, err := strconv.Atoi(requestedPortText)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	options := hostconfig.Options{
		ConfigPath:    filepath.Join(root, "prods.ini"),
		Listen:        requested,
		ListenSource:  "default",
		DataDir:       filepath.Join(root, "data"),
		BackupDir:     filepath.Join(root, "backups"),
		BaseURL:       "http://127.0.0.1:" + requestedPortText,
		BaseURLSource: "default",
	}
	listener, err := prepareInstallerListener(&options)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, selectedPortText, err := net.SplitHostPort(options.Listen)
	if err != nil {
		t.Fatal(err)
	}
	selectedPort, err := strconv.Atoi(selectedPortText)
	if err != nil {
		t.Fatal(err)
	}
	if selectedPort <= requestedPort || selectedPort > requestedPort+19 ||
		options.BaseURL != "http://127.0.0.1:"+selectedPortText || !options.ConfigLoaded {
		t.Fatalf("installer fallback options=%+v", options)
	}
	resolved, err := (hostconfig.Resolver{
		Executable: filepath.Join(root, "prods"),
		Args:       []string{"--config", options.ConfigPath},
		LookupEnv:  func(string) (string, bool) { return "", false },
	}).Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Listen != options.Listen || resolved.BaseURL != options.BaseURL || resolved.DataDir != options.DataDir {
		t.Fatalf("persisted installer fallback=%+v", resolved)
	}
}

func TestInstalledOrConfiguredListenerConflictDoesNotChangePort(t *testing.T) {
	occupied := listenBelowFallbackCeiling(t)
	defer occupied.Close()
	root := t.TempDir()
	options := hostconfig.Options{
		ConfigPath:    filepath.Join(root, "prods.ini"),
		ConfigLoaded:  true,
		Listen:        occupied.Addr().String(),
		ListenSource:  "config",
		DataDir:       filepath.Join(root, "data"),
		BackupDir:     filepath.Join(root, "backups"),
		BaseURL:       "http://127.0.0.1:8080",
		BaseURLSource: "config",
	}
	original := options.Listen
	if _, err := prepareInstallerListener(&options); err == nil || !strings.Contains(err.Error(), "update the effective listen setting") {
		t.Fatalf("configured port conflict error=%v", err)
	}
	if options.Listen != original {
		t.Fatalf("configured listener changed from %s to %s", original, options.Listen)
	}
}

func listenBelowFallbackCeiling(t *testing.T) net.Listener {
	t.Helper()
	for attempt := 0; attempt < 20; attempt++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		_, portText, err := net.SplitHostPort(listener.Addr().String())
		if err != nil {
			_ = listener.Close()
			t.Fatal(err)
		}
		port, err := strconv.Atoi(portText)
		if err == nil && port <= 65516 {
			return listener
		}
		_ = listener.Close()
	}
	t.Fatal(fmt.Errorf("could not allocate a test port below fallback ceiling"))
	return nil
}
