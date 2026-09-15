//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func IsServiceProcess() (bool, error) { return svc.IsWindowsService() }

func RunService(name string, run ServiceRunFunc) error {
	if !serviceNamePattern.MatchString(name) || run == nil {
		return errors.New("invalid Windows service runner")
	}
	return svc.Run(name, &windowsServiceHandler{run: run})
}

type windowsServiceHandler struct {
	run ServiceRunFunc
}

func (handler *windowsServiceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	stop := make(chan struct{})
	var stopOnce sync.Once
	done := make(chan error, 1)
	go func() { done <- handler.run(stop) }()
	changes <- svc.Status{State: svc.Running, Accepts: accepts}
	for {
		select {
		case err := <-done:
			changes <- svc.Status{State: svc.StopPending}
			if err != nil {
				return false, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				stopOnce.Do(func() { close(stop) })
				if err := <-done; err != nil {
					return false, 1
				}
				return false, 0
			}
		}
	}
}

func InstallService(ctx context.Context, spec ServiceSpec) error {
	if err := ValidateServiceSpec(spec); err != nil {
		return err
	}
	if strings.TrimSpace(spec.User) != "" {
		return errors.New("--service-user is Linux-only; Windows uses a restricted virtual service account")
	}
	for name, path := range map[string]string{"executable": spec.Executable, "config": spec.ConfigPath} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("Windows service %s must be an existing regular file: %s", name, path)
		}
	}
	for _, path := range []string{spec.DataDir, spec.BackupDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return fmt.Errorf("prepare Windows service data path %s: %w", path, err)
		}
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager: %w", err)
	}
	defer manager.Disconnect()
	if existing, err := manager.OpenService(spec.Name); err == nil {
		_ = existing.Close()
		return fmt.Errorf("Windows service %s already exists; uninstall it before replacing its definition", spec.Name)
	}
	account := `NT SERVICE\` + spec.Name
	service, err := manager.CreateService(spec.Name, spec.Executable, mgr.Config{
		StartType:        mgr.StartAutomatic,
		ErrorControl:     mgr.ErrorNormal,
		DisplayName:      spec.DisplayName,
		Description:      spec.Description,
		ServiceStartName: account,
		SidType:          windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}, "--config", spec.ConfigPath)
	if err != nil {
		return fmt.Errorf("create Windows service %s: %w", spec.Name, err)
	}
	defer service.Close()
	installed := false
	defer func() {
		if !installed {
			_ = service.Delete()
		}
	}()
	for _, grant := range []struct {
		path       string
		permission string
	}{
		{spec.DataDir, "(OI)(CI)M"},
		{spec.BackupDir, "(OI)(CI)M"},
		{spec.ConfigPath, "R"},
		{spec.Executable, "RX"},
	} {
		output, err := exec.CommandContext(ctx, "icacls", grant.path, "/grant:r", account+":"+grant.permission).CombinedOutput()
		if err != nil {
			return fmt.Errorf("grant %s access to %s: %w: %s", account, grant.path, err, strings.TrimSpace(string(output)))
		}
	}
	if err := service.Start(); err != nil {
		return fmt.Errorf("start Windows service %s: %w", spec.Name, err)
	}
	if err := waitWindowsService(ctx, service, svc.Running, 15*time.Second); err != nil {
		return err
	}
	installed = true
	return nil
}

func UninstallService(ctx context.Context, name string) error {
	if !serviceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid service name %q", name)
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Windows Service Control Manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(name)
	if err != nil {
		return fmt.Errorf("open Windows service %s: %w", name, err)
	}
	defer service.Close()
	status, queryErr := service.Query()
	if queryErr == nil && status.State != svc.Stopped {
		if _, err := service.Control(svc.Stop); err != nil {
			return fmt.Errorf("stop Windows service %s: %w", name, err)
		}
		if err := waitWindowsService(ctx, service, svc.Stopped, 15*time.Second); err != nil {
			return err
		}
	}
	if err := service.Delete(); err != nil {
		return fmt.Errorf("delete Windows service %s: %w", name, err)
	}
	return nil
}

func waitWindowsService(ctx context.Context, service *mgr.Service, desired svc.State, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == desired {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("Windows service did not reach state %d before timeout", desired)
		case <-ticker.C:
		}
	}
}
