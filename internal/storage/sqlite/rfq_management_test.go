package sqlite

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"prods/internal/identity"
	"prods/internal/inquiries"
)

func TestRFQManagementIsRevisionedAuthorizedAndAudited(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "ABC-123"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	inProgress, err := store.UpdateRFQStatus(t.Context(), owner.ID, receipt.RFQID, 1, inquiries.StatusInProgress)
	if err != nil {
		t.Fatal(err)
	}
	if inProgress.Status != inquiries.StatusInProgress || inProgress.Revision != 2 || inProgress.UpdatedBy != owner.ID {
		t.Fatalf("updated RFQ = %#v", inProgress)
	}
	if _, err := store.UpdateRFQStatus(t.Context(), owner.ID, receipt.RFQID, 1, inquiries.StatusSpam); !errors.Is(err, inquiries.ErrRFQRevisionConflict) {
		t.Fatalf("stale status update = %v", err)
	}
	if _, err := store.UpdateRFQStatus(t.Context(), owner.ID, receipt.RFQID, 2, inquiries.StatusNew); !errors.Is(err, inquiries.ErrInvalidTransition) {
		t.Fatalf("invalid transition = %v", err)
	}

	assigned, err := store.ReplaceRFQRecipients(t.Context(), owner.ID, receipt.RFQID, 2, []inquiries.Recipient{
		{Kind: inquiries.RecipientUser, UserID: owner.ID},
		{Kind: inquiries.RecipientEmail, Email: "Sales@Example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Revision != 3 || len(assigned.Recipients) != 2 || assigned.Recipients[0].UserID != owner.ID || assigned.Recipients[1].Email != "sales@example.test" {
		t.Fatalf("assigned RFQ = %#v", assigned)
	}
	var auditCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE target_id=? AND action IN ('rfq.status_updated','rfq.recipients_replaced')`, receipt.RFQID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("RFQ audit count = %d", auditCount)
	}
	var publicationCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM publication_intents WHERE entity_id=?`, receipt.RFQID).Scan(&publicationCount); err != nil {
		t.Fatal(err)
	}
	if publicationCount != 0 {
		t.Fatalf("recipient assignment created unrelated publication/delivery work = %d", publicationCount)
	}
}

func TestRFQManagementRechecksCapabilityAndRecipientUser(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "ABC-456"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	now := "2026-09-14T00:00:00Z"
	if _, err := store.db.Exec(`INSERT INTO roles(id,name,status,revision,created_at,updated_at) VALUES('role_viewer','Viewer','active',1,?,?);
		INSERT INTO role_capabilities(role_id,capability) VALUES('role_viewer',?);
		INSERT INTO users(id,email,email_normalized,display_name,role,status,password_hash,auth_revision,created_at,updated_at)
		VALUES('usr_viewer','viewer@example.test','viewer@example.test','Viewer','role_viewer','active','x',1,?,?)`,
		now, now, identity.CapabilityRFQView, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateRFQStatus(t.Context(), "usr_viewer", receipt.RFQID, 1, inquiries.StatusInProgress); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("RFQ manage without capability = %v", err)
	}
	if _, err := store.ReplaceRFQRecipients(t.Context(), "usr_viewer", receipt.RFQID, 1, nil); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("recipient manage without capability = %v", err)
	}
	if _, err := store.ReplaceRFQRecipients(t.Context(), owner.ID, receipt.RFQID, 1, []inquiries.Recipient{{Kind: inquiries.RecipientUser, UserID: "missing"}}); !errors.Is(err, inquiries.ErrInvalidRecipient) {
		t.Fatalf("missing system user recipient = %v", err)
	}
	current, err := store.RFQ(t.Context(), receipt.RFQID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 || len(current.Recipients) != 0 {
		t.Fatalf("rejected operations mutated RFQ = %#v", current)
	}
}

func TestDefaultRFQRecipientsAreVersionedAndCopiedAtSubmission(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	settings, err := store.RFQRecipientSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Revision != 1 || len(settings.Recipients) != 0 {
		t.Fatalf("initial settings = %#v", settings)
	}
	settings, err = store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, []inquiries.Recipient{
		{Kind: inquiries.RecipientUser, UserID: owner.ID},
		{Kind: inquiries.RecipientEmail, Email: "default@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Revision != 2 || len(settings.Recipients) != 2 {
		t.Fatalf("updated settings = %#v", settings)
	}
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, nil); !errors.Is(err, inquiries.ErrRFQRevisionConflict) {
		t.Fatalf("stale default recipient update = %v", err)
	}

	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Default Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "DEFAULT-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rfq, err := store.RFQ(t.Context(), receipt.RFQID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rfq.Recipients) != 2 || rfq.Recipients[0].UserID != owner.ID || rfq.Recipients[1].Email != "default@example.test" {
		t.Fatalf("RFQ defaults = %#v", rfq.Recipients)
	}
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 2, nil); err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.RFQ(t.Context(), receipt.RFQID)
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.Recipients) != 2 || unchanged.Revision != 1 {
		t.Fatalf("default settings rewrote existing RFQ = %#v", unchanged)
	}
	var auditCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='rfq.default_recipients_updated'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("default recipient audit count = %d", auditCount)
	}
}

func openRFQManagementStore(t *testing.T) (*Store, identity.User) {
	t.Helper()
	store, err := Create(filepath.Join(t.TempDir(), "rfq-management.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: password,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}

func TestRFQAnonymizationScrubsLocalCopiesAndBlocksPayloadReplay(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, []inquiries.Recipient{
		{Kind: inquiries.RecipientEmail, Email: "sales-private@example.test"},
	}); err != nil {
		t.Fatal(err)
	}
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	submission := inquiries.Submission{
		Name: "Private Buyer", Email: "buyer-private@example.test", Company: "Private Company",
		Phone: "+1 555 0100", Country: "Private Country", GeneralMessage: "private general message",
		Items: []inquiries.Item{{Kind: "requested", Requested: "PRIVATE-PART", RawQuery: "private query", Quantity: "secret quantity", Notes: "private item notes"}},
	}
	receipt, err := store.SubmitRFQ(t.Context(), key, submission)
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

	anonymized, err := store.AnonymizeRFQ(t.Context(), owner.ID, receipt.RFQID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if anonymized.PrivacyState != "anonymized" || anonymized.PrivacyAt == "" || anonymized.PrivacyBy != owner.ID ||
		anonymized.Revision != 2 || anonymized.Name != "" || anonymized.Email != "" || anonymized.Company != "" ||
		anonymized.Phone != "" || anonymized.Country != "" || anonymized.GeneralMessage != "" || len(anonymized.Recipients) != 0 ||
		len(anonymized.Items) != 1 || anonymized.Items[0].Requested != "" || anonymized.Items[0].RawQuery != "" ||
		anonymized.Items[0].Quantity != "" || anonymized.Items[0].Notes != "" {
		t.Fatalf("anonymized RFQ=%#v", anonymized)
	}

	var content, subject, body, recipientEmail, recipientStatus string
	if err := store.db.QueryRow(`SELECT a.content_json,a.subject,a.body,r.email,r.status
		FROM smtp_delivery_attempts a JOIN smtp_delivery_recipients r ON r.attempt_id=a.id
		WHERE a.id=?`, attempt.ID).Scan(&content, &subject, &body, &recipientEmail, &recipientStatus); err != nil {
		t.Fatal(err)
	}
	storedDelivery := strings.Join([]string{content, subject, body, recipientEmail}, " ")
	for _, private := range []string{"Private Buyer", "buyer-private@example.test", "Private Company", "+1 555 0100",
		"Private Country", "private general message", "PRIVATE-PART", "private query", "secret quantity", "private item notes", "sales-private@example.test"} {
		if strings.Contains(storedDelivery, private) {
			t.Fatalf("stored delivery retained %q: %s", private, storedDelivery)
		}
	}
	if content != `{"anonymized":true}` || recipientEmail != "" || recipientStatus != "failed" {
		t.Fatalf("scrubbed delivery content=%q recipient=%q status=%q", content, recipientEmail, recipientStatus)
	}

	if _, err := store.SubmitRFQ(t.Context(), key, submission); !errors.Is(err, inquiries.ErrIdempotencyConflict) {
		t.Fatalf("anonymized submission replay=%v", err)
	}
	newDeliveryKey, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, anonymized.Revision, newDeliveryKey); !errors.Is(err, inquiries.ErrRFQAnonymized) {
		t.Fatalf("delivery after anonymization=%v", err)
	}
	replayed, err := store.AnonymizeRFQ(t.Context(), owner.ID, receipt.RFQID, anonymized.Revision)
	if err != nil || replayed.Revision != anonymized.Revision {
		t.Fatalf("idempotent anonymization=%#v err=%v", replayed, err)
	}
	var auditCount int
	var auditDetails string
	if err := store.db.QueryRow(`SELECT COUNT(*),MAX(details_json) FROM admin_log
		WHERE target_id=? AND action='rfq.personal_data_anonymized'`, receipt.RFQID).Scan(&auditCount, &auditDetails); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 || strings.Contains(auditDetails, "Private") || strings.Contains(auditDetails, "buyer-private") {
		t.Fatalf("privacy audit count=%d details=%s", auditCount, auditDetails)
	}
}

func TestRFQAnonymizationRefusesAnInFlightDeliveryWithoutPartialScrub(t *testing.T) {
	store, owner := openRFQManagementStore(t)
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, []inquiries.Recipient{
		{Kind: inquiries.RecipientEmail, Email: "sales@example.test"},
	}); err != nil {
		t.Fatal(err)
	}
	key, _ := NewKey()
	receipt, err := store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Buyer In Flight", Email: "in-flight@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "IN-FLIGHT-PART"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, _ := NewKey()
	if _, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.ClaimSMTPDeliveryRecipient(t.Context()); err != nil || !found {
		t.Fatalf("claim delivery found=%v err=%v", found, err)
	}
	if _, err := store.AnonymizeRFQ(t.Context(), owner.ID, receipt.RFQID, 1); !errors.Is(err, inquiries.ErrRFQDeliveryInFlight) {
		t.Fatalf("in-flight anonymization=%v", err)
	}
	retained, err := store.RFQ(t.Context(), receipt.RFQID)
	if err != nil || retained.PrivacyState != "retained" || retained.Email != "in-flight@example.test" || retained.Revision != 1 {
		t.Fatalf("RFQ was partially scrubbed=%#v err=%v", retained, err)
	}
}
