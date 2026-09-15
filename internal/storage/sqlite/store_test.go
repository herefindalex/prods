package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"prods/internal/catalog"
	"prods/internal/inquiries"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := CreatePOC(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPublicVisibilityAndUnicodeSearch(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	if _, err := store.PublishedProduct(ctx, "synthetic-hidden"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("hidden product lookup error = %v", err)
	}
	for _, product := range []catalog.Product{
		{ID: "upper-accent", PartNumber: "ÉCHO", Manufacturer: "Example", Status: catalog.Published},
		{ID: "lower-accent", PartNumber: "écho", Manufacturer: "Example", Status: catalog.Published},
	} {
		if err := store.InsertProduct(ctx, product); err != nil {
			t.Fatal(err)
		}
	}
	products, total, err := store.ListPublished(ctx, "écho", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(products) != 2 {
		t.Fatalf("accented search returned %d/%d, want 2", len(products), total)
	}
	products, total, err = store.ListPublished(ctx, "%", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(products) != 0 {
		t.Fatal("literal percent must not act as wildcard")
	}
}

func TestEmbeddedSQLiteVersionIsQueryable(t *testing.T) {
	store := openTestStore(t)
	var version string
	if err := store.db.QueryRowContext(context.Background(), "SELECT sqlite_version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("embedded SQLite did not report a version")
	}
	t.Logf("embedded SQLite version: %s", version)
}

func TestIdempotencyReceiptAndRequestedPart(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	before, err := store.ProductCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	submission := inquiries.Submission{Name: "Ada", Email: "ada@example.test", Items: []inquiries.Item{
		{Kind: "catalog", ProductID: "synthetic-published"},
		{Kind: "requested", Requested: "TI TPS54331DR", RawQuery: "TI TPS54331DR"},
	}}
	first, err := store.SubmitRFQ(ctx, key, submission)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.SubmitRFQ(ctx, key, submission)
	if err != nil {
		t.Fatal(err)
	}
	if first.RFQID != second.RFQID || !second.Replay {
		t.Fatalf("receipt was not replayed: %#v %#v", first, second)
	}
	changed := submission
	changed.GeneralMessage = "changed"
	if _, err := store.SubmitRFQ(ctx, key, changed); !errors.Is(err, inquiries.ErrIdempotencyConflict) {
		t.Fatalf("changed payload error = %v", err)
	}
	after, err := store.ProductCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("requested RFQ created a Product: before=%d after=%d", before, after)
	}
	rfqs, err := store.ListRFQs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rfqs) != 1 || len(rfqs[0].Items) != 2 {
		t.Fatalf("stored RFQ = %#v", rfqs)
	}
}
