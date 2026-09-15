package sqlite

import (
	"testing"

	"prods/internal/catalog"
	"prods/internal/importing"
)

func TestAtomicImportMapsMultipleApplicationIDs(t *testing.T) {
	ctx := t.Context()
	store, owner := installedStore(t)
	first, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryApplication, Name: "Robotics", Slug: "robotics", Status: catalog.EntryActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateDictionaryEntry(ctx, owner.ID, catalog.DictionaryEntry{
		Kind: catalog.DictionaryApplication, Name: "Energy", Slug: "energy", Status: catalog.EntryActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	workbook := importing.Workbook{
		Sheet: "Products", HeaderRow: 1, Headers: []string{"Part Number", "Applications"},
		Rows:         []importing.Row{{Number: 2, Cells: []string{"IMP-APP-1", first.ID + "; " + second.ID + "," + first.ID}}},
		FullyScanned: true, CheckedRows: 2,
	}
	mappings := []importing.ColumnMapping{
		{SourceIndex: 0, SourceName: "Part Number", Target: importing.TargetPartNumber},
		{SourceIndex: 1, SourceName: "Applications", Target: importing.TargetApplicationIDs},
	}
	preview, err := store.BuildImportPreview(ctx, owner.ID, workbook, mappings, importing.IdentityPartNumber)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullyValidated || len(preview.Items) != 1 || len(preview.Items[0].Product.ApplicationIDs) != 2 {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := store.CommitImport(ctx, owner.ID, preview); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Product(ctx, preview.Items[0].Product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Applications) != 2 || stored.Applications[0].Name != "Robotics" || stored.Applications[1].Name != "Energy" {
		t.Fatalf("stored applications = %+v", stored.Applications)
	}
}
