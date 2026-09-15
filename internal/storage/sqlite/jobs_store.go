package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

var ErrJobNotRetryable = errors.New("job is not safely retryable")

type JobKinds struct {
	Publication bool
	Import      bool
	Backup      bool
	SMTP        bool
	Search      bool
}

// JobRecord is an Admin read model over each subsystem's durable metadata.
// It does not replace the subsystem tables or create a generic workflow engine.
type JobRecord struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Status          string   `json:"status"`
	Stage           string   `json:"stage"`
	TargetType      string   `json:"target_type,omitempty"`
	TargetID        string   `json:"target_id,omitempty"`
	Label           string   `json:"label,omitempty"`
	DesiredRevision int64    `json:"desired_revision,omitempty"`
	ProgressCurrent int64    `json:"progress_current,omitempty"`
	ProgressTotal   int64    `json:"progress_total,omitempty"`
	ErrorMessage    string   `json:"error_message,omitempty"`
	Retryable       bool     `json:"retryable"`
	Outputs         []string `json:"outputs,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

func (s *Store) RecentJobs(ctx context.Context, kinds JobKinds, limit int) ([]JobRecord, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	jobs := make([]JobRecord, 0, limit)
	loaders := []struct {
		enabled bool
		load    func(context.Context, int) ([]JobRecord, error)
	}{
		{kinds.Publication, s.recentPublicationJobs},
		{kinds.Import, s.recentImportJobs},
		{kinds.Backup, s.recentBackupJobs},
		{kinds.SMTP, s.recentSMTPJobs},
		{kinds.Search, s.recentSearchJobs},
	}
	for _, loader := range loaders {
		if !loader.enabled {
			continue
		}
		loaded, err := loader.load(ctx, limit)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, loaded...)
	}
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].UpdatedAt == jobs[j].UpdatedAt {
			return jobs[i].ID > jobs[j].ID
		}
		return jobs[i].UpdatedAt > jobs[j].UpdatedAt
	})
	if len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs, nil
}

func (s *Store) recentSearchJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	submissions, err := s.RecentSearchSubmissionJobs(ctx, limit)
	if err != nil {
		return nil, err
	}
	jobs := make([]JobRecord, 0, len(submissions))
	for _, submission := range submissions {
		job := JobRecord{
			ID: submission.ID, Kind: "search_submission", Status: submission.Status, Stage: "external_submission",
			TargetType: submission.Provider, TargetID: submission.Subject, ProgressTotal: 1,
			ErrorMessage: submission.ResponseMessage, Retryable: submission.Status == "failed",
			CreatedAt: submission.CreatedAt, UpdatedAt: submission.UpdatedAt,
			Outputs: []string{"accepted means submitted, not indexed"},
		}
		if submission.Status == "accepted" {
			job.ProgressCurrent = 1
			job.ErrorMessage = ""
			switch {
			case submission.Provider == "indexnow" && submission.HTTPStatus == 202:
				job.Label = "accepted; key validation may still be pending"
			case submission.Provider == "google_search_console":
				job.Label = "Sitemap submitted; indexing unknown"
			default:
				job.Label = "accepted; indexing unknown"
			}
			job.Outputs = append(job.Outputs, fmt.Sprintf("HTTP %d", submission.HTTPStatus))
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *Store) recentPublicationJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id,i.status,
		CASE i.status WHEN 'pending' THEN 'queued' WHEN 'processing' THEN 'render_activate' ELSE i.status END,
		i.entity_id,i.cause,i.desired_revision,i.error_message,i.created_at,i.updated_at,
		CASE WHEN i.status='failed'
			AND EXISTS(SELECT 1 FROM products p WHERE p.id=i.entity_id AND p.record_state='current'
				AND p.status='published' AND p.revision=i.desired_revision)
			AND NOT EXISTS(SELECT 1 FROM publication_intents newer WHERE newer.entity_type='product'
				AND newer.entity_id=i.entity_id AND newer.id<>i.id
				AND newer.status IN ('pending','processing') AND newer.desired_revision>=i.desired_revision)
			AND NOT EXISTS(SELECT 1 FROM public_activations a WHERE a.product_id=i.entity_id
				AND a.source_revision=i.desired_revision)
		THEN 1 ELSE 0 END
		FROM publication_intents i WHERE i.entity_type='product'
		ORDER BY i.updated_at DESC,i.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]JobRecord, 0)
	for rows.Next() {
		job := JobRecord{Kind: "publication", TargetType: "product", ProgressTotal: 1,
			Outputs: []string{"html", "json-ld", "json", "markdown", "sitemap", "catalog-manifest"}}
		if err := rows.Scan(&job.ID, &job.Status, &job.Stage, &job.TargetID, &job.Label,
			&job.DesiredRevision, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt, &job.Retryable); err != nil {
			return nil, err
		}
		if job.Status == "completed" || job.Status == "superseded" {
			job.ProgressCurrent = 1
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) recentImportJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,status,phase,original_filename,checked_rows,total_rows,error_message,created_at,updated_at
		FROM import_jobs ORDER BY updated_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]JobRecord, 0)
	for rows.Next() {
		job := JobRecord{Kind: "import", TargetType: "import"}
		if err := rows.Scan(&job.ID, &job.Status, &job.Stage, &job.Label, &job.ProgressCurrent,
			&job.ProgressTotal, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		job.TargetID = job.ID
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) recentBackupJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,status,COALESCE(backup_id,''),COALESCE(error_message,''),started_at,
		COALESCE(completed_at,started_at) FROM backup_runs ORDER BY started_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]JobRecord, 0)
	for rows.Next() {
		job := JobRecord{Kind: "backup", Stage: "snapshot_verify", TargetType: "backup", ProgressTotal: 1}
		if err := rows.Scan(&job.ID, &job.Label, &job.Status, &job.TargetID, &job.ErrorMessage,
			&job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		if job.TargetID == "" {
			job.TargetID = job.ID
		}
		if job.Status == "succeeded" {
			job.ProgressCurrent = 1
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) recentSMTPJobs(ctx context.Context, limit int) ([]JobRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.status,a.rfq_id,a.rfq_revision,a.created_at,a.updated_at,
		(SELECT COUNT(*) FROM smtp_delivery_recipients r WHERE r.attempt_id=a.id
			AND r.status IN ('accepted','failed','unknown')),
		(SELECT COUNT(*) FROM smtp_delivery_recipients r WHERE r.attempt_id=a.id),
		COALESCE((SELECT r.error_message FROM smtp_delivery_recipients r WHERE r.attempt_id=a.id
			AND r.error_message<>'' ORDER BY r.recipient_index LIMIT 1),'')
		FROM smtp_delivery_attempts a ORDER BY a.updated_at DESC,a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]JobRecord, 0)
	for rows.Next() {
		job := JobRecord{Kind: "smtp", Stage: "recipient_delivery", TargetType: "rfq"}
		if err := rows.Scan(&job.ID, &job.Status, &job.TargetID, &job.DesiredRevision, &job.CreatedAt,
			&job.UpdatedAt, &job.ProgressCurrent, &job.ProgressTotal, &job.ErrorMessage); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// RetryPublicationJob creates a new intent and preserves the failed intent as
// evidence. Exact revision and eligibility checks make this safe; import
// commits and SMTP unknown outcomes deliberately have no equivalent method.
func (s *Store) RetryPublicationJob(ctx context.Context, actorID, failedIntentID string) (JobRecord, error) {
	actorID = strings.TrimSpace(actorID)
	failedIntentID = strings.TrimSpace(failedIntentID)
	if actorID == "" || failedIntentID == "" {
		return JobRecord{}, ErrJobNotRetryable
	}
	newID, err := randomID("out")
	if err != nil {
		return JobRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var productID string
	var revision int64
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT entity_id,desired_revision FROM publication_intents
			WHERE id=? AND entity_type='product' AND status='failed'`, failedIntentID).Scan(&productID, &revision); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrJobNotRetryable
			}
			return err
		}
		var currentRevision int64
		var recordState catalog.RecordState
		var status catalog.Status
		if err := tx.QueryRowContext(ctx, `SELECT revision,record_state,status FROM products WHERE id=?`, productID).
			Scan(&currentRevision, &recordState, &status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrJobNotRetryable
			}
			return err
		}
		if currentRevision != revision || recordState != catalog.RecordCurrent || status != catalog.Published {
			return ErrJobNotRetryable
		}
		var conflicting int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_intents WHERE entity_type='product'
			AND entity_id=? AND id<>? AND status IN ('pending','processing') AND desired_revision>=?`,
			productID, failedIntentID, revision).Scan(&conflicting); err != nil {
			return err
		}
		if conflicting != 0 {
			return ErrJobNotRetryable
		}
		var alreadyActive int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM public_activations WHERE product_id=? AND source_revision=?`,
			productID, revision).Scan(&alreadyActive); err != nil {
			return err
		}
		if alreadyActive != 0 {
			return ErrJobNotRetryable
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO publication_intents(
			id,entity_type,entity_id,desired_revision,cause,status,error_message,created_at,updated_at
		) VALUES(?,'product',?,?,?,'pending','',?,?)`, newID, productID, revision,
			"admin.retry:"+failedIntentID, now, now); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "publication.retry_queued", "product", productID, map[string]any{
			"failed_intent_id": failedIntentID,
			"retry_intent_id":  newID,
			"desired_revision": revision,
		})
	})
	if err != nil {
		return JobRecord{}, err
	}
	return JobRecord{
		ID: newID, Kind: "publication", Status: "pending", Stage: "queued", TargetType: "product",
		TargetID: productID, Label: "admin.retry:" + failedIntentID, DesiredRevision: revision,
		ProgressTotal: 1, Outputs: []string{"html", "json-ld", "json", "markdown", "sitemap", "catalog-manifest"},
		CreatedAt: now, UpdatedAt: now,
	}, nil
}
