//go:build linux

package platform

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemdUnitUsesOneConfigAndRestrictsWritableRoots(t *testing.T) {
	root := t.TempDir()
	spec := ServiceSpec{
		Name:        "prods",
		DisplayName: "Prods",
		Description: "Prods catalog and RFQ service",
		Executable:  filepath.Join(root, "Program Files", "prods"),
		ConfigPath:  filepath.Join(root, "configuration", "prods.ini"),
		DataDir:     filepath.Join(root, "state"),
		BackupDir:   filepath.Join(root, "backup target"),
		User:        "prods-service",
	}
	unit, err := renderSystemdUnit(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`User=prods-service`,
		`ExecStart="` + spec.Executable + `" --config "` + spec.ConfigPath + `"`,
		`ReadWritePaths="` + spec.DataDir + `" "` + spec.BackupDir + `"`,
		`NoNewPrivileges=true`,
		`ProtectSystem=strict`,
		`KillSignal=SIGTERM`,
	} {
		if !strings.Contains(unit, expected) {
			t.Errorf("systemd unit missing %q:\n%s", expected, unit)
		}
	}
	if strings.Contains(unit, "--data-dir") || strings.Contains(unit, "--backup-dir") {
		t.Fatalf("service duplicated host configuration on command line:\n%s", unit)
	}
}

func TestLinuxServiceInstallAndUninstallUseExactUnitWithoutTouchingSystem(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	executable := filepath.Join(root, "prods")
	configPath := filepath.Join(root, "prods.ini")
	if err := os.WriteFile(executable, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[prods]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := ServiceSpec{
		Name:        "prods-test",
		DisplayName: "Prods Test",
		Description: "Prods test service",
		Executable:  executable,
		ConfigPath:  configPath,
		DataDir:     filepath.Join(root, "data"),
		BackupDir:   filepath.Join(root, "backups"),
		User:        current.Username,
	}
	originalDirectory := systemdUnitDirectory
	originalRunner := runSystemctl
	systemdUnitDirectory = filepath.Join(root, "systemd")
	var calls []string
	runSystemctl = func(_ context.Context, arguments ...string) ([]byte, error) {
		calls = append(calls, strings.Join(arguments, " "))
		return nil, nil
	}
	t.Cleanup(func() {
		systemdUnitDirectory = originalDirectory
		runSystemctl = originalRunner
	})
	if err := InstallService(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(systemdUnitDirectory, spec.Name+".service")
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0] != "daemon-reload" || calls[1] != "enable --now prods-test.service" ||
		calls[2] != "is-active --quiet prods-test.service" {
		t.Fatalf("install systemctl calls=%v", calls)
	}
	if err := UninstallService(t.Context(), spec.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Fatalf("unit remained after uninstall: %v", err)
	}
	if len(calls) != 5 || calls[3] != "disable --now prods-test.service" || calls[4] != "daemon-reload" {
		t.Fatalf("uninstall systemctl calls=%v", calls)
	}
}

func TestLinuxServiceRemovalStillDeletesUnitWhenStopFails(t *testing.T) {
	root := t.TempDir()
	originalDirectory := systemdUnitDirectory
	originalRunner := runSystemctl
	systemdUnitDirectory = root
	runSystemctl = func(_ context.Context, arguments ...string) ([]byte, error) {
		if len(arguments) > 0 && arguments[0] == "disable" {
			return []byte("forced stop failure"), os.ErrPermission
		}
		return nil, nil
	}
	t.Cleanup(func() {
		systemdUnitDirectory = originalDirectory
		runSystemctl = originalRunner
	})
	unitPath := filepath.Join(root, "prods-test.service")
	if err := os.WriteFile(unitPath, []byte("unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := UninstallService(t.Context(), "prods-test")
	if err == nil || !strings.Contains(err.Error(), "forced stop failure") {
		t.Fatalf("uninstall error=%v", err)
	}
	if _, statErr := os.Stat(unitPath); !os.IsNotExist(statErr) {
		t.Fatalf("unit remained after failed stop: %v", statErr)
	}
}
