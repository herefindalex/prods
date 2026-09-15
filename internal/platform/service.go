package platform

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultServiceName = "prods"

type ServiceSpec struct {
	Name        string
	DisplayName string
	Description string
	Executable  string
	ConfigPath  string
	DataDir     string
	BackupDir   string
	User        string
}

type ServiceRunFunc func(stop <-chan struct{}) error

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,63}$`)

func ValidateServiceSpec(spec ServiceSpec) error {
	if !serviceNamePattern.MatchString(spec.Name) {
		return fmt.Errorf("invalid service name %q", spec.Name)
	}
	for name, value := range map[string]string{
		"executable": spec.Executable,
		"config":     spec.ConfigPath,
		"data":       spec.DataDir,
		"backup":     spec.BackupDir,
	} {
		if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
			return fmt.Errorf("service %s path must be absolute", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("service %s path contains an invalid character", name)
		}
	}
	if strings.ContainsAny(spec.DisplayName+spec.Description+spec.User, "\r\n\x00") {
		return errors.New("service metadata contains an invalid character")
	}
	return nil
}
