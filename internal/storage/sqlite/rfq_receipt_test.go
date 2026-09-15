package sqlite

import (
	"errors"
	"path/filepath"
	"testing"

	"prods/internal/inquiries"
)

func TestLookupRFQReceiptReplaysBeforeAdmissionAndRejectsChangedPayload(t *testing.T) {
	store, err := CreatePOC(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	submission := inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "ABC-123"}},
	}
	if receipt, found, err := store.LookupRFQReceipt(t.Context(), key, submission); err != nil || found || receipt.RFQID != "" {
		t.Fatalf("missing receipt lookup = %+v found=%v err=%v", receipt, found, err)
	}
	original, err := store.SubmitRFQ(t.Context(), key, submission)
	if err != nil {
		t.Fatal(err)
	}
	replayed, found, err := store.LookupRFQReceipt(t.Context(), key, submission)
	if err != nil || !found || !replayed.Replay || replayed.RFQID != original.RFQID {
		t.Fatalf("receipt lookup = %+v found=%v err=%v", replayed, found, err)
	}
	changed := submission
	changed.Items = []inquiries.Item{{Kind: "requested", Requested: "DIFFERENT"}}
	if _, _, err := store.LookupRFQReceipt(t.Context(), key, changed); !errors.Is(err, inquiries.ErrIdempotencyConflict) {
		t.Fatalf("changed receipt lookup error = %v", err)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE idempotency_receipts SET canonical_version='unknown' WHERE idempotency_key=?`, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.LookupRFQReceipt(t.Context(), key, submission); !errors.Is(err, ErrInvalidRFQReceipt) {
		t.Fatalf("unknown canonical receipt lookup error = %v", err)
	}
	if _, err := store.SubmitRFQ(t.Context(), key, submission); !errors.Is(err, ErrInvalidRFQReceipt) {
		t.Fatalf("unknown canonical receipt submission error = %v", err)
	}
}
