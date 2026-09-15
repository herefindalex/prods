package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"prods/internal/inquiries"
)

var ErrInvalidRFQReceipt = errors.New("invalid RFQ idempotency receipt")

// LookupRFQReceipt lets the public admission layer replay a committed result
// before applying a new-submission rate limit. It never creates an RFQ.
func (s *Store) LookupRFQReceipt(ctx context.Context, key string, submission inquiries.Submission) (inquiries.Receipt, bool, error) {
	if len(key) < 24 {
		return inquiries.Receipt{}, false, inquiries.ErrInvalidRFQ
	}
	hash, err := submission.CanonicalHash()
	if err != nil {
		return inquiries.Receipt{}, false, err
	}
	var canonicalVersion, existingHash, encodedReceipt string
	err = s.db.QueryRowContext(ctx, `SELECT canonical_version,request_hash,receipt_json
		FROM idempotency_receipts WHERE idempotency_key=?`, key).Scan(&canonicalVersion, &existingHash, &encodedReceipt)
	if errors.Is(err, sql.ErrNoRows) {
		return inquiries.Receipt{}, false, nil
	}
	if err != nil {
		return inquiries.Receipt{}, false, err
	}
	if canonicalVersion != inquiries.CanonicalVersion {
		return inquiries.Receipt{}, false, ErrInvalidRFQReceipt
	}
	if existingHash != hash {
		return inquiries.Receipt{}, false, inquiries.ErrIdempotencyConflict
	}
	var receipt inquiries.Receipt
	if err := json.Unmarshal([]byte(encodedReceipt), &receipt); err != nil || receipt.RFQID == "" {
		return inquiries.Receipt{}, false, ErrInvalidRFQReceipt
	}
	receipt.Replay = true
	return receipt, true, nil
}
