package sqlite

import (
	"context"
	"errors"
	"testing"

	"prods/internal/catalog"
	"prods/internal/importing"
)

func importWorkbook(headers []string, rows ...[]string) importing.Workbook {
	workbook := importing.Workbook{Sheet: "Products", HeaderRow: 1, Headers: headers, FullyScanned: true, CheckedRows: len(rows) + 1}
	for index, cells := range rows {
		workbook.Rows = append(workbook.Rows, importing.Row{Number: index + 2, Cells: cells})
	}
	return workbook
}

func TestImportPreviewScansAllRowErrorsAndDoesNotGuessAmbiguousCurrent(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	makerA, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{Kind: catalog.DictionaryManufacturer, Name: "Maker A"})
	if err != nil {
		t.Fatal(err)
	}
	makerB, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{Kind: catalog.DictionaryManufacturer, Name: "Maker B"})
	if err != nil {
		t.Fatal(err)
	}
	for _, makerID := range []string{makerA.ID, makerB.ID} {
		if _, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "AMB-1", ManufacturerID: makerID}); err != nil {
			t.Fatal(err)
		}
	}

	headers := []string{"P/N", "Maker", "Category"}
	mappings := []importing.ColumnMapping{
		{SourceIndex: 0, Target: importing.TargetPartNumber},
		{SourceIndex: 1, Target: importing.TargetManufacturerID},
		{SourceIndex: 2, Target: importing.TargetCategoryID},
	}
	preview, err := store.BuildImportPreview(ctx, owner.ID, importWorkbook(headers,
		[]string{"", makerA.ID, "cat_uncategorized"},
		[]string{"BAD-MAKER", "missing-maker", "cat_uncategorized"},
		[]string{"BAD-CATEGORY", makerA.ID, "missing-category"},
	), mappings, importing.IdentityComposite)
	if err != nil {
		t.Fatal(err)
	}
	if preview.FullyValidated || len(preview.Issues) != 3 {
		t.Fatalf("full error preview = %+v", preview)
	}
	seen := map[int]bool{}
	for _, issue := range preview.Issues {
		seen[issue.Row] = true
	}
	if !seen[2] || !seen[3] || !seen[4] {
		t.Fatalf("error rows = %+v", preview.Issues)
	}

	ambiguous, err := store.BuildImportPreview(ctx, owner.ID, importWorkbook([]string{"P/N"}, []string{"AMB-1"}),
		[]importing.ColumnMapping{{SourceIndex: 0, Target: importing.TargetPartNumber}}, importing.IdentityPartNumber)
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.FullyValidated || len(ambiguous.Issues) != 1 || ambiguous.Issues[0].Code != "ambiguous_current_match" {
		t.Fatalf("ambiguous preview = %+v", ambiguous)
	}
}

func TestAtomicImportCommitRevalidatesRevisionAndReplaysReceipt(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	maker, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{Kind: catalog.DictionaryManufacturer, Name: "Maker"})
	if err != nil {
		t.Fatal(err)
	}
	existing, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "EXIST-1", ManufacturerID: maker.ID, Name: "Old"})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "SAME-1", ManufacturerID: maker.ID, Name: "Same"})
	if err != nil {
		t.Fatal(err)
	}
	headers := []string{"P/N", "Maker", "Name"}
	mappings := []importing.ColumnMapping{
		{SourceIndex: 0, Target: importing.TargetPartNumber},
		{SourceIndex: 1, Target: importing.TargetManufacturerID},
		{SourceIndex: 2, Target: importing.TargetProductName},
	}
	preview, err := store.BuildImportPreview(ctx, owner.ID, importWorkbook(headers,
		[]string{"EXIST-1", maker.ID, "Updated"},
		[]string{"NEW-1", maker.ID, "New"},
		[]string{"SAME-1", maker.ID, "Same"},
	), mappings, importing.IdentityComposite)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullyValidated || preview.CreateCount != 1 || preview.UpdateCount != 1 || preview.NoChangeCount != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	receipt, err := store.CommitImport(ctx, owner.ID, preview)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Created != 1 || receipt.Updated != 1 || receipt.NoChange != 1 || receipt.Replay {
		t.Fatalf("receipt = %+v", receipt)
	}
	replayed, err := store.CommitImport(ctx, owner.ID, preview)
	if err != nil || !replayed.Replay || replayed.OperationID != receipt.OperationID {
		t.Fatalf("replayed receipt = %+v, %v", replayed, err)
	}
	updated, err := store.Product(ctx, existing.ID)
	if err != nil || updated.Name != "Updated" || updated.Status != catalog.Hidden || updated.Revision != existing.Revision+1 {
		t.Fatalf("updated product = %+v, %v", updated, err)
	}
	unchangedAfter, err := store.Product(ctx, unchanged.ID)
	if err != nil || unchangedAfter.Revision != unchanged.Revision {
		t.Fatalf("unchanged product = %+v, %v", unchangedAfter, err)
	}

	stalePreview, err := store.BuildImportPreview(ctx, owner.ID, importWorkbook(headers,
		[]string{"EXIST-1", maker.ID, "Import wants this"},
		[]string{"STALE-NEW", maker.ID, "Must not be inserted"},
	), mappings, importing.IdentityComposite)
	if err != nil || !stalePreview.FullyValidated {
		t.Fatalf("stale preview setup = %+v, %v", stalePreview, err)
	}
	updated.Name = "Concurrent edit"
	if _, err := store.UpdateProduct(ctx, owner.ID, updated.Revision, updated); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitImport(ctx, owner.ID, stalePreview); !errors.Is(err, importing.ErrImportConflict) {
		t.Fatalf("stale commit = %v", err)
	}
	var staleNewCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE identity_part_number='STALE-NEW'`).Scan(&staleNewCount); err != nil {
		t.Fatal(err)
	}
	if staleNewCount != 0 {
		t.Fatal("stale import left a new product behind")
	}
}

func TestImportRollsBackEveryProductWhenLaterWriteFails(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	headers := []string{"P/N"}
	preview, err := store.BuildImportPreview(ctx, owner.ID, importWorkbook(headers,
		[]string{"ROLLBACK-1"}, []string{"ROLLBACK-FAIL"},
	), []importing.ColumnMapping{{SourceIndex: 0, Target: importing.TargetPartNumber}}, importing.IdentityPartNumber)
	if err != nil || !preview.FullyValidated {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fail_second_import BEFORE INSERT ON products
		WHEN NEW.identity_part_number='ROLLBACK-FAIL' BEGIN SELECT RAISE(ABORT,'injected import failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitImport(ctx, owner.ID, preview); err == nil {
		t.Fatal("injected Import failure unexpectedly committed")
	}
	var productCount, runCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE identity_part_number LIKE 'ROLLBACK-%'`).Scan(&productCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_runs WHERE id=?`, preview.OperationID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if productCount != 0 || runCount != 0 {
		t.Fatalf("partial import product_count=%d run_count=%d", productCount, runCount)
	}
}

func TestImportTemplateVersionsRemainTraceable(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	first, err := store.SaveImportTemplate(ctx, owner.ID, 0, importing.Template{
		Name: "Supplier format",
		Snapshot: importing.TemplateSnapshot{HeaderRow: 1, IdentityMode: importing.IdentityPartNumber,
			Mappings: []importing.ColumnMapping{{SourceIndex: 0, SourceName: "P/N", Target: importing.TargetPartNumber}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Snapshot.Mappings = append(second.Snapshot.Mappings,
		importing.ColumnMapping{SourceIndex: 1, SourceName: "Name", Target: importing.TargetProductName})
	second, err = store.SaveImportTemplate(ctx, owner.ID, first.Version, second)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 {
		t.Fatalf("second version = %d", second.Version)
	}
	v1, err := store.ImportTemplate(ctx, first.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.ImportTemplate(ctx, first.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || len(v1.Snapshot.Mappings) != 1 || current.Version != 2 || len(current.Snapshot.Mappings) != 2 {
		t.Fatalf("template snapshots v1=%+v current=%+v", v1, current)
	}
	if _, err := store.SaveImportTemplate(ctx, owner.ID, 1, second); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale template save = %v", err)
	}
}

func TestImportJobRestartReconcilesFromDurableCommitReceipt(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	snapshot := importing.TemplateSnapshot{HeaderRow: 1, IdentityMode: importing.IdentityPartNumber,
		Mappings: []importing.ColumnMapping{{SourceIndex: 0, Target: importing.TargetPartNumber}}}
	interrupted, err := store.CreateImportJob(ctx, owner.ID, "interrupted.xlsx", "checksum-a", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkImportJobParsing(ctx, interrupted.ID); err != nil {
		t.Fatal(err)
	}

	committed, err := store.CreateImportJob(ctx, owner.ID, "committed.xlsx", "checksum-b", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkImportJobParsing(ctx, committed.ID); err != nil {
		t.Fatal(err)
	}
	preview, err := store.BuildImportPreview(ctx, owner.ID, importWorkbook([]string{"P/N"}, []string{"RESTART-TRUTH"}),
		snapshot.Mappings, snapshot.IdentityMode)
	if err != nil || !preview.FullyValidated {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	preview.OperationID = committed.ID
	if err := store.SaveImportJobPreview(ctx, committed.ID, "/private/preview", "/private/report", preview); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginImportCommit(ctx, owner.ID, committed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitImport(ctx, owner.ID, preview); err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileImportJobs(ctx); err != nil {
		t.Fatal(err)
	}
	committedAfter, err := store.ImportJob(ctx, committed.ID)
	if err != nil || committedAfter.Status != importing.JobCommitted {
		t.Fatalf("committed reconciliation = %+v, %v", committedAfter, err)
	}
	interruptedAfter, err := store.ImportJob(ctx, interrupted.ID)
	if err != nil || interruptedAfter.Status != importing.JobInterrupted {
		t.Fatalf("interrupted reconciliation = %+v, %v", interruptedAfter, err)
	}
}
