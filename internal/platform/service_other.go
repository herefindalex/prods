//go:build !linux && !windows

package platform

import (
	"context"
	"errors"
)

func IsServiceProcess() (bool, error) { return false, nil }

func RunService(string, ServiceRunFunc) error {
	return errors.New("service mode is unsupported on this platform")
}

func InstallService(context.Context, ServiceSpec) error {
	return errors.New("service installation is supported only on Linux and Windows")
}

func UninstallService(context.Context, string) error {
	return errors.New("service removal is supported only on Linux and Windows")
}
