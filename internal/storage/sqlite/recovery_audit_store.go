package sqlite

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (s *Store) ImportRecoveryAudit(ctx context.Context, eventID, operationID, action, backupID, result, createdUTC string) error {
	backupID = strings.TrimSpace(backupID)
	if !validAuditID(eventID) || !validAuditID(operationID) ||
		(action != "restore.requested" && action != "restore.completed" && action != "restore.failed") ||
		(result != "started" && result != "success" && result != "failure") || backupID == "" ||
		strings.ContainsAny(backupID, `/\\`) {
		return errors.New("invalid recovery audit import")
	}
	if _, err := time.Parse(time.RFC3339Nano, createdUTC); err != nil {
		return errors.New("invalid recovery audit timestamp")
	}
	details, err := json.Marshal(map[string]any{
		"source": "recovery_ui", "operation_id": operationID, "backup_id": backupID,
	})
	if err != nil {
		return err
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO admin_log(
			id,actor_id,action,target_type,target_id,result,details_json,created_at
		) VALUES(?,NULL,?,'restore',?,?,?,?) ON CONFLICT(id) DO NOTHING`,
			"log_recovery_"+eventID, action, operationID, result, string(details), createdUTC)
		return err
	})
}

func validAuditID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && strings.ToLower(value) == value
}
