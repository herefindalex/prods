package sqlite

import (
	"errors"
	"testing"

	"prods/internal/inquiries"
)

func TestManualSMTPDeliveryIsDurableIdempotentAndRestartSafe(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, []inquiries.Recipient{
		{Kind: inquiries.RecipientUser, UserID: owner.ID},
		{Kind: inquiries.RecipientEmail, Email: "sales@example.test"},
	}); err != nil {
		t.Fatal(err)
	}
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test", GeneralMessage: "Please quote",
		Items: []inquiries.Item{{Kind: "requested", Requested: "ABC-123", Quantity: "10"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.ListSMTPDeliveryAttempts(t.Context(), receipt.RFQID)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("RFQ submission automatically created mail = %#v", before)
	}

	deliveryKey, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status != inquiries.DeliveryPending || len(attempt.Recipients) != 2 || attempt.ContentHash == "" {
		t.Fatalf("created attempt = %#v", attempt)
	}
	replay, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != attempt.ID || !replay.Replay {
		t.Fatalf("delivery replay = %#v, original = %#v", replay, attempt)
	}

	work, found, err := store.ClaimSMTPDeliveryRecipient(t.Context())
	if err != nil || !found {
		t.Fatalf("claim found=%v err=%v", found, err)
	}
	if work.AttemptID != attempt.ID || work.To == "" || work.Subject == "" || work.Body == "" {
		t.Fatalf("delivery work = %#v", work)
	}
	if err := store.ReconcileSMTPDeliveries(t.Context()); err != nil {
		t.Fatal(err)
	}
	afterRestart, err := store.SMTPDeliveryAttempt(t.Context(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRestart.Status != inquiries.DeliveryUnknown || afterRestart.Recipients[work.RecipientIndex].Status != inquiries.DeliveryUnknown {
		t.Fatalf("reconciled attempt = %#v", afterRestart)
	}

	second, found, err := store.ClaimSMTPDeliveryRecipient(t.Context())
	if err != nil || !found || second.RecipientIndex == work.RecipientIndex {
		t.Fatalf("second claim = %#v found=%v err=%v", second, found, err)
	}
	if err := store.CompleteSMTPDeliveryRecipient(t.Context(), second.AttemptID, second.RecipientIndex,
		inquiries.DeliveryAccepted, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimSMTPDeliveryRecipient(t.Context()); err != nil || found {
		t.Fatalf("unknown recipient was automatically reclaimed: found=%v err=%v", found, err)
	}
	completed, err := store.SMTPDeliveryAttempt(t.Context(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != inquiries.DeliveryUnknown {
		t.Fatalf("mixed accepted/unknown status = %s", completed.Status)
	}

	var auditCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='rfq.delivery_requested' AND target_id=?`, receipt.RFQID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("idempotent delivery audit count = %d", auditCount)
	}
}

func TestManualSMTPDeliveryRequiresRecipientsCurrentRevisionAndManageCapability(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	key, _ := NewKey()
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "EMPTY-RECIPIENT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, _ := NewKey()
	if _, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey); !errors.Is(err, inquiries.ErrNoDeliveryRecipient) {
		t.Fatalf("delivery without recipients = %v", err)
	}
	assigned, err := store.ReplaceRFQRecipients(t.Context(), owner.ID, receipt.RFQID, 1,
		[]inquiries.Recipient{{Kind: inquiries.RecipientEmail, Email: "sales@example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey); !errors.Is(err, inquiries.ErrRFQRevisionConflict) {
		t.Fatalf("delivery against stale RFQ = %v", err)
	}
	attempt, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, assigned.Revision, deliveryKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, assigned.Revision+1, deliveryKey); !errors.Is(err, inquiries.ErrDeliveryConflict) {
		t.Fatalf("changed delivery-key payload = %v", err)
	}
	if attempt.RFQRevision != assigned.Revision {
		t.Fatalf("attempt revision = %d", attempt.RFQRevision)
	}
	if _, err := store.CreateSMTPDeliveryAttempt(t.Context(), "missing-user", receipt.RFQID, assigned.Revision, "sub_012345678901234567890123456789"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("delivery without capability = %v", err)
	}
}
