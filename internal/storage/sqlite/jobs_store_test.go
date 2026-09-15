package sqlite

import (
	"errors"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/importing"
	"prods/internal/inquiries"
)

func TestPublicationJobFailureIsStructuredAndSafeRetryPreservesEvidence(t *testing.T) {
	store, owner := installedStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber: "JOB-PUBLISH-1",
		Name:       "Publication job",
		Status:     catalog.Published,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, found, err := store.ClaimPublicationIntent(t.Context())
	if err != nil || !found {
		t.Fatalf("claim intent found=%v err=%v", found, err)
	}
	if err := store.FailPublicationIntent(t.Context(), intent.ID, " renderer unavailable "); err != nil {
		t.Fatal(err)
	}

	jobs, err := store.RecentJobs(t.Context(), JobKinds{Publication: true}, 20)
	if err != nil {
		t.Fatal(err)
	}
	failed := findJob(t, jobs, intent.ID)
	if failed.Status != "failed" || failed.ErrorMessage != "renderer unavailable" || !failed.Retryable {
		t.Fatalf("failed publication job = %+v", failed)
	}
	if failed.Label != "product.created" || failed.TargetID != product.ID || failed.DesiredRevision != product.Revision {
		t.Fatalf("failed publication identity = %+v", failed)
	}

	retry, err := store.RetryPublicationJob(t.Context(), owner.ID, intent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != "pending" || retry.TargetID != product.ID || retry.DesiredRevision != product.Revision || retry.Retryable {
		t.Fatalf("retry job = %+v", retry)
	}
	if _, err := store.RetryPublicationJob(t.Context(), owner.ID, intent.ID); !errors.Is(err, ErrJobNotRetryable) {
		t.Fatalf("duplicate retry error = %v", err)
	}

	jobs, err = store.RecentJobs(t.Context(), JobKinds{Publication: true}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if current := findJob(t, jobs, intent.ID); current.Retryable {
		t.Fatalf("original failed job remained retryable with queued replacement: %+v", current)
	}
	if queued := findJob(t, jobs, retry.ID); queued.Status != "pending" || queued.Stage != "queued" {
		t.Fatalf("queued retry = %+v", queued)
	}
	var auditCount int
	if err := store.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM admin_log
		WHERE action='publication.retry_queued' AND target_id=?`, product.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("retry audit count = %d", auditCount)
	}
}

func TestPublicationRetryRejectsSupersededRevision(t *testing.T) {
	store, owner := installedStore(t)
	product, err := store.CreateProduct(t.Context(), owner.ID, catalog.Product{
		PartNumber: "JOB-PUBLISH-2",
		Status:     catalog.Published,
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, found, err := store.ClaimPublicationIntent(t.Context())
	if err != nil || !found {
		t.Fatalf("claim intent found=%v err=%v", found, err)
	}
	if err := store.FailPublicationIntent(t.Context(), intent.ID, "failed"); err != nil {
		t.Fatal(err)
	}
	product.Name = "newer revision"
	if _, err := store.UpdateProduct(t.Context(), owner.ID, product.Revision, product); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RetryPublicationJob(t.Context(), owner.ID, intent.ID); !errors.Is(err, ErrJobNotRetryable) {
		t.Fatalf("superseded retry error = %v", err)
	}
}

func TestRecentJobsReadsSubsystemMetadataWithoutCreatingGenericJobs(t *testing.T) {
	store, owner := installedStore(t)
	importJob, err := store.CreateImportJob(t.Context(), owner.ID, "catalog.xlsx", "checksum", importing.TemplateSnapshot{HeaderRow: 1})
	if err != nil {
		t.Fatal(err)
	}
	backup, claimed, err := store.ClaimBackupRun(t.Context(), "manual", "", time.Now().UTC())
	if err != nil || !claimed {
		t.Fatalf("backup claimed=%v err=%v", claimed, err)
	}
	if err := store.FailBackupRun(t.Context(), backup.ID, errors.New("backup media unavailable"), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	jobs, err := store.RecentJobs(t.Context(), JobKinds{Import: true, Backup: true}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if job := findJob(t, jobs, importJob.ID); job.Kind != "import" || job.Label != "catalog.xlsx" || job.Retryable {
		t.Fatalf("import job = %+v", job)
	}
	if job := findJob(t, jobs, backup.ID); job.Kind != "backup" || job.Status != "failed" ||
		job.ErrorMessage != "backup media unavailable" || job.Retryable {
		t.Fatalf("backup job = %+v", job)
	}

	publicationOnly, err := store.RecentJobs(t.Context(), JobKinds{Publication: true}, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range publicationOnly {
		if job.Kind != "publication" {
			t.Fatalf("unexpected filtered job = %+v", job)
		}
	}
}

func TestRecentJobsIncludesSMTPProgressButNeverOffersUnknownOutcomeRetry(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, []inquiries.Recipient{
		{Kind: inquiries.RecipientEmail, Email: "sales@example.test"},
	}); err != nil {
		t.Fatal(err)
	}
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "JOB-SMTP-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey)
	if err != nil {
		t.Fatal(err)
	}
	work, found, err := store.ClaimSMTPDeliveryRecipient(t.Context())
	if err != nil || !found {
		t.Fatalf("claim SMTP work found=%v err=%v", found, err)
	}
	if err := store.CompleteSMTPDeliveryRecipient(t.Context(), work.AttemptID, work.RecipientIndex,
		inquiries.DeliveryUnknown, "transport", "SMTP acceptance could not be determined"); err != nil {
		t.Fatal(err)
	}

	jobs, err := store.RecentJobs(t.Context(), JobKinds{SMTP: true}, 20)
	if err != nil {
		t.Fatal(err)
	}
	job := findJob(t, jobs, attempt.ID)
	if job.Kind != "smtp" || job.Status != "unknown" || job.ProgressCurrent != 1 || job.ProgressTotal != 1 ||
		job.ErrorMessage != "SMTP acceptance could not be determined" || job.Retryable {
		t.Fatalf("SMTP job = %+v", job)
	}
}

func findJob(t *testing.T, jobs []JobRecord, id string) JobRecord {
	t.Helper()
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	t.Fatalf("job %q not found in %+v", id, jobs)
	return JobRecord{}
}
