package sqlite

import (
	"context"
	"testing"

	"prods/internal/importing"
)

func TestImportCommitAtomicallyCompletesDurableJob(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	snapshot := importing.TemplateSnapshot{
		HeaderRow:    1,
		IdentityMode: importing.IdentityPartNumber,
		Mappings: []importing.ColumnMapping{
			{SourceIndex: 0, SourceName: "Part Number", Target: importing.TargetPartNumber},
		},
	}
	job, err := store.CreateImportJob(ctx, owner.ID, "commit-unknown.xlsx", "checksum", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkImportJobParsing(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	preview, err := store.BuildImportPreview(
		ctx,
		owner.ID,
		importWorkbook([]string{"Part Number"}, []string{"COMMIT-UNKNOWN-1"}),
		snapshot.Mappings,
		snapshot.IdentityMode,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullyValidated {
		t.Fatalf("preview was not valid: %+v", preview.Issues)
	}
	preview.OperationID = job.ID
	if err := store.SaveImportJobPreview(ctx, job.ID, "/private/preview", "/private/report", preview); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginImportCommit(ctx, owner.ID, job.ID); err != nil {
		t.Fatal(err)
	}

	receipt, err := store.CommitImport(ctx, owner.ID, preview)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.OperationID != job.ID || receipt.Replay {
		t.Fatalf("receipt = %+v", receipt)
	}
	completed, err := store.ImportJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != importing.JobCommitted || completed.Phase != "completed" {
		t.Fatalf("durable job after committed receipt = status %q phase %q", completed.Status, completed.Phase)
	}
	if err := store.BeginImportCommit(ctx, owner.ID, job.ID); err != nil {
		t.Fatalf("retry was not admitted from the durable receipt: %v", err)
	}
	replayed, err := store.CommitImport(ctx, owner.ID, preview)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replay || replayed.OperationID != job.ID {
		t.Fatalf("replayed receipt = %+v", replayed)
	}
}
