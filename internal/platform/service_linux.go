//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

var systemdUnitDirectory = "/etc/systemd/system"
var runSystemctl = func(ctx context.Context, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "systemctl", arguments...).CombinedOutput()
}

func IsServiceProcess() (bool, error) { return false, nil }

func RunService(string, ServiceRunFunc) error {
	return errors.New("systemd services run the normal foreground entrypoint")
}

func InstallService(ctx context.Context, spec ServiceSpec) error {
	if err := ValidateServiceSpec(spec); err != nil {
		return err
	}
	if strings.TrimSpace(spec.User) == "" {
		return errors.New("service installation on Linux requires --service-user")
	}
	if _, err := user.Lookup(spec.User); err != nil {
		return fmt.Errorf("look up service user %q: %w", spec.User, err)
	}
	unit, err := renderSystemdUnit(spec)
	if err != nil {
		return err
	}
	unitPath := filepath.Join(systemdUnitDirectory, spec.Name+".service")
	if err := writeServiceFile(unitPath, []byte(unit)); err != nil {
		return err
	}
	if output, err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := runSystemctl(ctx, "enable", "--now", spec.Name+".service"); err != nil {
		return fmt.Errorf("enable service %s: %w: %s", spec.Name, err, strings.TrimSpace(string(output)))
	}
	if output, err := runSystemctl(ctx, "is-active", "--quiet", spec.Name+".service"); err != nil {
		return fmt.Errorf("service %s was installed but did not remain active: %w: %s", spec.Name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func UninstallService(ctx context.Context, name string) error {
	if !serviceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid service name %q", name)
	}
	unit := name + ".service"
	var result error
	if output, err := runSystemctl(ctx, "disable", "--now", unit); err != nil {
		result = errors.Join(result, fmt.Errorf("disable service %s: %w: %s", name, err, strings.TrimSpace(string(output))))
	}
	unitPath := filepath.Join(systemdUnitDirectory, unit)
	if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		result = errors.Join(result, fmt.Errorf("remove service unit %s: %w", unitPath, err))
	}
	if output, err := runSystemctl(ctx, "daemon-reload"); err != nil {
		result = errors.Join(result, fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(output))))
	}
	return result
}

func renderSystemdUnit(spec ServiceSpec) (string, error) {
	if err := ValidateServiceSpec(spec); err != nil {
		return "", err
	}
	if strings.TrimSpace(spec.User) == "" || strings.ContainsAny(spec.User, " /\\\t") {
		return "", errors.New("systemd service user is required and must be a simple account name")
	}
	return "[Unit]\n" +
		"Description=" + spec.Description + "\n" +
		"After=network.target\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"User=" + spec.User + "\n" +
		"WorkingDirectory=" + systemdQuote(filepath.Dir(spec.Executable)) + "\n" +
		"ExecStart=" + systemdQuote(spec.Executable) + " --config " + systemdQuote(spec.ConfigPath) + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=5s\n" +
		"TimeoutStopSec=15s\n" +
		"KillSignal=SIGTERM\n" +
		"NoNewPrivileges=true\n" +
		"PrivateTmp=true\n" +
		"ProtectSystem=strict\n" +
		"ProtectHome=read-only\n" +
		"ReadWritePaths=" + systemdQuote(spec.DataDir) + " " + systemdQuote(spec.BackupDir) + "\n\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n", nil
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func writeServiceFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create service directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".prods-service-*.tmp")
	if err != nil {
		return fmt.Errorf("stage service unit: %w", err)
	}
	temporaryPath := temporary.Name()
	complete := false
	defer func() {
		_ = temporary.Close()
		if !complete {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("activate service unit %s: %w", path, err)
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open service directory for sync: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return fmt.Errorf("sync service directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close service directory: %w", closeErr)
	}
	complete = true
	return nil
}
