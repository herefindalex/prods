package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

var ErrInvalidBackupState = errors.New("invalid backup settings or run state")

type BackupSettings struct {
	Enabled             bool   `json:"enabled"`
	LocalTime           string `json:"local_time"`
	TimeZone            string `json:"time_zone"`
	RetentionDaily      int    `json:"retention_daily"`
	RetentionWeekly     int    `json:"retention_weekly"`
	RetentionMonthly    int    `json:"retention_monthly"`
	RetentionPreUpgrade int    `json:"retention_pre_upgrade"`
	RetentionPreRestore int    `json:"retention_pre_restore"`
	Version             int64  `json:"version"`
	UpdatedAt           string `json:"updated_at"`
}

type BackupRun struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	Status          string `json:"status"`
	ScheduledFor    string `json:"scheduled_for,omitempty"`
	StartedAt       string `json:"started_at"`
	CompletedAt     string `json:"completed_at,omitempty"`
	BackupID        string `json:"backup_id,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	ContentVerified bool   `json:"content_verified"`
	ReadOnlyApplied bool   `json:"read_only_applied"`
	ErrorMessage    string `json:"error_message,omitempty"`
	WarningMessage  string `json:"warning_message,omitempty"`
}

func (s *Store) BackupSettings(ctx context.Context) (BackupSettings, error) {
	var settings BackupSettings
	err := s.db.QueryRowContext(ctx, `SELECT b.enabled,b.local_time,b.retention_daily,b.retention_weekly,b.retention_monthly,
		b.retention_pre_upgrade,b.retention_pre_restore,b.version,b.updated_at,site.time_zone
		FROM backup_settings b CROSS JOIN site_settings site WHERE b.singleton=1 AND site.singleton=1`).Scan(
		&settings.Enabled, &settings.LocalTime, &settings.RetentionDaily, &settings.RetentionWeekly, &settings.RetentionMonthly,
		&settings.RetentionPreUpgrade, &settings.RetentionPreRestore, &settings.Version, &settings.UpdatedAt, &settings.TimeZone,
	)
	return settings, err
}

func (s *Store) UpdateBackupSettings(ctx context.Context, actorID string, expectedVersion int64, next BackupSettings) (BackupSettings, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || expectedVersion < 1 || !validBackupSettings(next) {
		return BackupSettings{}, ErrInvalidBackupState
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE backup_settings SET enabled=?,local_time=?,retention_daily=?,retention_weekly=?,retention_monthly=?,
			retention_pre_upgrade=?,retention_pre_restore=?,version=version+1,updated_at=? WHERE singleton=1 AND version=?`,
			next.Enabled, next.LocalTime, next.RetentionDaily, next.RetentionWeekly, next.RetentionMonthly,
			next.RetentionPreUpgrade, next.RetentionPreRestore, time.Now().UTC().Format(time.RFC3339Nano), expectedVersion)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return catalog.ErrRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "backup.settings_updated", "backup_settings", "singleton", map[string]any{
			"enabled": next.Enabled, "local_time": next.LocalTime,
		})
	})
	if err != nil {
		return BackupSettings{}, err
	}
	return s.BackupSettings(ctx)
}

func validBackupSettings(settings BackupSettings) bool {
	if _, err := time.Parse("15:04", settings.LocalTime); err != nil {
		return false
	}
	values := []int{settings.RetentionDaily, settings.RetentionWeekly, settings.RetentionMonthly, settings.RetentionPreUpgrade, settings.RetentionPreRestore}
	for _, value := range values {
		if value < 1 || value > 3660 {
			return false
		}
	}
	return true
}

func (s *Store) ClaimBackupRun(ctx context.Context, kind, scheduledFor string, now time.Time) (BackupRun, bool, error) {
	kind = strings.TrimSpace(kind)
	scheduledFor = strings.TrimSpace(scheduledFor)
	if !validBackupKind(kind) || (kind == "scheduled" && scheduledFor == "") {
		return BackupRun{}, false, ErrInvalidBackupState
	}
	id, err := randomID("bkp")
	if err != nil {
		return BackupRun{}, false, err
	}
	startedAt := now.UTC().Format(time.RFC3339Nano)
	claimed := false
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO backup_runs(id,kind,status,scheduled_for,started_at)
			VALUES(?,?,'running',NULLIF(?,''),?) ON CONFLICT(kind,scheduled_for) DO NOTHING`, id, kind, scheduledFor, startedAt)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		claimed = rows == 1
		return nil
	})
	if err != nil || !claimed {
		return BackupRun{}, claimed, err
	}
	return BackupRun{ID: id, Kind: kind, Status: "running", ScheduledFor: scheduledFor, StartedAt: startedAt}, true, nil
}

func (s *Store) CompleteBackupRun(ctx context.Context, runID, backupID string, sizeBytes int64, contentVerified, readOnlyApplied bool, now time.Time) error {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(backupID) == "" || sizeBytes < 0 || !contentVerified {
		return ErrInvalidBackupState
	}
	return s.finishBackupRun(ctx, runID, "succeeded", backupID, sizeBytes, contentVerified, readOnlyApplied, "", now)
}

func (s *Store) FailBackupRun(ctx context.Context, runID string, failure error, now time.Time) error {
	if strings.TrimSpace(runID) == "" || failure == nil {
		return ErrInvalidBackupState
	}
	message := strings.TrimSpace(failure.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.finishBackupRun(ctx, runID, "failed", "", 0, false, false, message, now)
}

func (s *Store) finishBackupRun(ctx context.Context, runID, status, backupID string, sizeBytes int64, verified, protected bool, message string, now time.Time) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE backup_runs SET status=?,completed_at=?,backup_id=NULLIF(?,''),size_bytes=?,
			content_verified=?,read_only_applied=?,error_message=NULLIF(?,'') WHERE id=? AND status='running'`,
			status, now.UTC().Format(time.RFC3339Nano), backupID, sizeBytes, verified, protected, message, runID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrInvalidBackupState
		}
		return nil
	})
}

func (s *Store) ReconcileBackupRuns(ctx context.Context, now time.Time) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE backup_runs SET status='failed',completed_at=?,error_message='process restarted before backup completion'
			WHERE status='running'`, now.UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) SetBackupRunWarning(ctx context.Context, runID string, warning error) error {
	if strings.TrimSpace(runID) == "" || warning == nil {
		return ErrInvalidBackupState
	}
	message := strings.TrimSpace(warning.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE backup_runs SET warning_message=? WHERE id=? AND status='succeeded'`, message, runID)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return ErrInvalidBackupState
		}
		return nil
	})
}

func (s *Store) RecentBackupRuns(ctx context.Context, limit int) ([]BackupRun, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,status,COALESCE(scheduled_for,''),started_at,COALESCE(completed_at,''),
		COALESCE(backup_id,''),COALESCE(size_bytes,0),COALESCE(content_verified,0),COALESCE(read_only_applied,0),COALESCE(error_message,''),COALESCE(warning_message,'')
		FROM backup_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]BackupRun, 0)
	for rows.Next() {
		var run BackupRun
		if err := rows.Scan(&run.ID, &run.Kind, &run.Status, &run.ScheduledFor, &run.StartedAt, &run.CompletedAt,
			&run.BackupID, &run.SizeBytes, &run.ContentVerified, &run.ReadOnlyApplied, &run.ErrorMessage, &run.WarningMessage); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) RunningBackupRuns(ctx context.Context) ([]BackupRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,status,COALESCE(scheduled_for,''),started_at,COALESCE(completed_at,''),
		COALESCE(backup_id,''),COALESCE(size_bytes,0),COALESCE(content_verified,0),COALESCE(read_only_applied,0),COALESCE(error_message,''),COALESCE(warning_message,'')
		FROM backup_runs WHERE status='running' ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]BackupRun, 0)
	for rows.Next() {
		var run BackupRun
		if err := rows.Scan(&run.ID, &run.Kind, &run.Status, &run.ScheduledFor, &run.StartedAt, &run.CompletedAt,
			&run.BackupID, &run.SizeBytes, &run.ContentVerified, &run.ReadOnlyApplied, &run.ErrorMessage, &run.WarningMessage); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func validBackupKind(kind string) bool {
	switch kind {
	case "scheduled", "manual", "pre-upgrade", "pre-restore":
		return true
	default:
		return false
	}
}
