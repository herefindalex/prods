package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryAuditIsDurableTokenFreeAndStrictlyParsed(t *testing.T) {
	control := filepath.Join(t.TempDir(), "control")
	requested, err := WriteAuditRecord(control, "", "restore.requested", "backup-20260914", "started")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAuditRecord(control, requested.OperationID, "restore.completed", "backup-20260914", "success"); err != nil {
		t.Fatal(err)
	}
	records, err := ListAuditRecords(control)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].OperationID != requested.OperationID || records[1].Result != "success" {
		t.Fatalf("recovery audit records=%+v", records)
	}
	for _, entry := range records {
		body, err := os.ReadFile(filepath.Join(control, "recovery-audit-"+entry.EventID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "token") || strings.Contains(string(body), "session") {
			t.Fatalf("recovery audit contains secret-shaped field: %s", body)
		}
	}
	if err := os.WriteFile(filepath.Join(control, "recovery-audit-00000000000000000000000000000000.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListAuditRecords(control); err == nil {
		t.Fatal("malformed recovery audit record was accepted")
	}
}
