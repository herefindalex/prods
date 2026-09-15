package platform

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateServiceSpecRequiresSafeNameAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	valid := ServiceSpec{
		Name:        "prods",
		DisplayName: "Prods",
		Description: "Prods catalog and RFQ service",
		Executable:  filepath.Join(root, "prods"),
		ConfigPath:  filepath.Join(root, "prods.ini"),
		DataDir:     filepath.Join(root, "data"),
		BackupDir:   filepath.Join(root, "backups"),
	}
	if err := ValidateServiceSpec(valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Name = "../other"
	if err := ValidateServiceSpec(invalid); err == nil {
		t.Fatal("unsafe service name was accepted")
	}
	invalid = valid
	invalid.DataDir = "relative/data"
	if err := ValidateServiceSpec(invalid); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative service data path error=%v", err)
	}
}
