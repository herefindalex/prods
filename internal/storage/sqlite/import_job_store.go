package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"prods/internal/identity"
	"prods/internal/importing"
)

func (s *Store) CreateImportJob(ctx context.Context, actorID, filename, checksum string, snapshot importing.TemplateSnapshot) (importing.Job, error) {
	id, err := randomID("imp")
	if err != nil {
		return importing.Job{}, err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return importing.Job{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	job := importing.Job{ID: id, ActorID: actorID, Status: importing.JobQueued, Phase: "upload_complete",
		OriginalFilename: filename, Checksum: checksum, TemplateSnapshot: snapshot, CreatedAt: now, UpdatedAt: now}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogImport); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO import_jobs(
			id,actor_id,status,phase,original_filename,checksum,template_snapshot_json,
			checked_rows,total_rows,create_count,update_count,no_change_count,failed_count,
			preview_path,report_path,error_message,cancel_requested,created_at,updated_at
		) VALUES(?,?,'queued','upload_complete',?,?,?,0,0,0,0,0,0,'','','',0,?,?)`,
			id, actorID, filename, checksum, string(encoded), now, now)
		return err
	})
	return job, err
}

func (s *Store) MarkImportJobParsing(ctx context.Context, id string) error {
	return s.transitionImportJob(ctx, id, []importing.JobStatus{importing.JobQueued}, importing.JobParsing, "parse_validate", "")
}

func (s *Store) SaveImportJobPreview(ctx context.Context, id, previewPath, reportPath string, preview importing.Preview) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='preview_ready',phase='preview',checked_rows=?,total_rows=?,
			create_count=?,update_count=?,no_change_count=?,failed_count=?,preview_path=?,report_path=?,error_message='',updated_at=?
			WHERE id=? AND status='parsing' AND cancel_requested=0`, preview.CheckedRows, preview.TotalRows,
			preview.CreateCount, preview.UpdateCount, preview.NoChangeCount, len(preview.Issues), previewPath, reportPath,
			time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return importing.ErrImportConflict
		}
		return nil
	})
}

func (s *Store) FailImportJob(ctx context.Context, id, phase, message string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='failed',phase=?,error_message=?,updated_at=?
			WHERE id=? AND status IN ('queued','parsing','preview_ready','committing')`, phase, message,
			time.Now().UTC().Format(time.RFC3339Nano), id)
		return err
	})
}

func (s *Store) RequestImportCancel(ctx context.Context, actorID, id string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogImport); err != nil {
			return err
		}
		var status importing.JobStatus
		if err := tx.QueryRowContext(ctx, `SELECT status FROM import_jobs WHERE id=?`, id).Scan(&status); err != nil {
			return err
		}
		if status == importing.JobCommitting {
			return importing.ErrImportConflict
		}
		if status != importing.JobQueued && status != importing.JobParsing && status != importing.JobPreviewReady {
			return nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE import_jobs SET cancel_requested=1,status='cancelled',phase='cancelled',updated_at=? WHERE id=?`,
			time.Now().UTC().Format(time.RFC3339Nano), id)
		return err
	})
}

func (s *Store) BeginImportCommit(ctx context.Context, actorID, id string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogImport); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='committing',phase='final_commit',updated_at=?
			WHERE id=? AND status='preview_ready' AND cancel_requested=0`, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed == 1 {
			return nil
		}
		var receiptStatus string
		err = tx.QueryRowContext(ctx, `SELECT r.status
			FROM import_jobs j JOIN import_runs r ON r.id=j.id
			WHERE j.id=? AND j.status='committed' AND r.status='committed'`, id).Scan(&receiptStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return importing.ErrImportConflict
		}
		return err
	})
}

func (s *Store) FinishImportJob(ctx context.Context, id string) error {
	return s.transitionImportJob(ctx, id, []importing.JobStatus{importing.JobCommitting}, importing.JobCommitted, "completed", "")
}

func (s *Store) ImportJob(ctx context.Context, id string) (importing.Job, error) {
	var job importing.Job
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT id,actor_id,status,phase,original_filename,checksum,template_snapshot_json,
		checked_rows,total_rows,create_count,update_count,no_change_count,failed_count,preview_path,report_path,error_message,
		cancel_requested,created_at,updated_at FROM import_jobs WHERE id=?`, id).Scan(
		&job.ID, &job.ActorID, &job.Status, &job.Phase, &job.OriginalFilename, &job.Checksum, &encoded,
		&job.CheckedRows, &job.TotalRows, &job.CreateCount, &job.UpdateCount, &job.NoChangeCount, &job.FailedCount,
		&job.PreviewPath, &job.ReportPath, &job.ErrorMessage, &job.CancelRequested, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return importing.Job{}, err
	}
	if err := json.Unmarshal([]byte(encoded), &job.TemplateSnapshot); err != nil {
		return importing.Job{}, err
	}
	return job, nil
}

func (s *Store) ReconcileImportJobs(ctx context.Context) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='committed',phase='completed',updated_at=?
			WHERE status='committing' AND EXISTS(SELECT 1 FROM import_runs r WHERE r.id=import_jobs.id AND r.status='committed')`, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status='interrupted',phase='interrupted',
			error_message='process restarted before the operation reached a durable completion receipt',updated_at=?
			WHERE status IN ('queued','parsing','committing')`, now)
		return err
	})
}

func (s *Store) transitionImportJob(ctx context.Context, id string, from []importing.JobStatus, to importing.JobStatus, phase, message string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var status importing.JobStatus
		if err := tx.QueryRowContext(ctx, `SELECT status FROM import_jobs WHERE id=?`, id).Scan(&status); err != nil {
			return err
		}
		allowed := false
		for _, candidate := range from {
			allowed = allowed || status == candidate
		}
		if !allowed {
			return importing.ErrImportConflict
		}
		result, err := tx.ExecContext(ctx, `UPDATE import_jobs SET status=?,phase=?,error_message=?,updated_at=? WHERE id=? AND status=?`,
			to, phase, message, time.Now().UTC().Format(time.RFC3339Nano), id, status)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return importing.ErrImportConflict
		}
		return nil
	})
}

func IsImportJobMissing(err error) bool { return errors.Is(err, sql.ErrNoRows) }
