package recovery

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const recoveryAuditVersion = 1

type AuditRecord struct {
	Version     int    `json:"version"`
	EventID     string `json:"event_id"`
	OperationID string `json:"operation_id"`
	Action      string `json:"action"`
	BackupID    string `json:"backup_id,omitempty"`
	Result      string `json:"result"`
	CreatedUTC  string `json:"created_utc"`
}

func WriteAuditRecord(controlDir, operationID, action, backupID, result string) (AuditRecord, error) {
	if strings.TrimSpace(controlDir) == "" {
		return AuditRecord{}, errors.New("recovery audit control directory is empty")
	}
	if operationID == "" {
		var err error
		operationID, err = recoveryAuditID()
		if err != nil {
			return AuditRecord{}, err
		}
	}
	record := AuditRecord{
		Version: recoveryAuditVersion, EventID: "", OperationID: operationID,
		Action: strings.TrimSpace(action), BackupID: strings.TrimSpace(backupID),
		Result: strings.TrimSpace(result), CreatedUTC: time.Now().UTC().Format(time.RFC3339Nano),
	}
	var err error
	record.EventID, err = recoveryAuditID()
	if err != nil {
		return AuditRecord{}, err
	}
	if err := validateAuditRecord(record); err != nil {
		return AuditRecord{}, err
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return AuditRecord{}, err
	}
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		return AuditRecord{}, err
	}
	path := filepath.Join(controlDir, "recovery-audit-"+record.EventID+".json")
	if err := writeDurableFile(path, append(body, '\n'), 0o600); err != nil {
		return AuditRecord{}, err
	}
	if err := syncDirectory(controlDir); err != nil {
		return AuditRecord{}, err
	}
	return record, nil
}

func ListAuditRecords(controlDir string) ([]AuditRecord, error) {
	entries, err := os.ReadDir(controlDir)
	if errors.Is(err, os.ErrNotExist) {
		return []AuditRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]AuditRecord, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "recovery-audit-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(controlDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record AuditRecord
		if err := json.Unmarshal(body, &record); err != nil {
			return nil, err
		}
		if err := validateAuditRecord(record); err != nil {
			return nil, err
		}
		if entry.Name() != "recovery-audit-"+record.EventID+".json" {
			return nil, errors.New("recovery audit filename does not match event ID")
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedUTC == records[j].CreatedUTC {
			return records[i].EventID < records[j].EventID
		}
		return records[i].CreatedUTC < records[j].CreatedUTC
	})
	return records, nil
}

func validateAuditRecord(record AuditRecord) error {
	if record.Version != recoveryAuditVersion || !validRecoveryAuditID(record.EventID) ||
		!validRecoveryAuditID(record.OperationID) || filepath.Base(record.BackupID) != record.BackupID ||
		strings.ContainsAny(record.BackupID, `/\\`) {
		return errors.New("invalid recovery audit record")
	}
	switch record.Action {
	case "restore.requested", "restore.completed", "restore.failed":
	default:
		return errors.New("invalid recovery audit action")
	}
	switch record.Result {
	case "started", "success", "failure":
	default:
		return errors.New("invalid recovery audit result")
	}
	if _, err := time.Parse(time.RFC3339Nano, record.CreatedUTC); err != nil {
		return errors.New("invalid recovery audit timestamp")
	}
	return nil
}

func recoveryAuditID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func validRecoveryAuditID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && strings.ToLower(value) == value
}
