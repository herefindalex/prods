package maildelivery

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/storage/sqlite"
)

type failingRuntimeStore struct {
	claimErr    error
	completeErr error
	claimed     bool
}

func (store *failingRuntimeStore) ReconcileSMTPDeliveries(context.Context) error { return nil }

func (store *failingRuntimeStore) ClaimSMTPDeliveryRecipient(context.Context) (inquiries.DeliveryWork, bool, error) {
	if store.claimErr != nil {
		return inquiries.DeliveryWork{}, false, store.claimErr
	}
	if store.claimed {
		return inquiries.DeliveryWork{}, false, nil
	}
	store.claimed = true
	return inquiries.DeliveryWork{AttemptID: "attempt-1", To: "recipient@example.test", Subject: "RFQ", Body: "body"}, true, nil
}

func (store *failingRuntimeStore) CompleteSMTPDeliveryRecipient(context.Context, string, int, inquiries.DeliveryStatus, string, string) error {
	return store.completeErr
}

type recordingSender struct {
	mu      sync.Mutex
	calls   []Message
	started chan struct{}
	release chan struct{}
}

func (sender *recordingSender) Send(_ context.Context, message Message) Result {
	sender.mu.Lock()
	sender.calls = append(sender.calls, message)
	call := len(sender.calls)
	sender.mu.Unlock()
	if call == 1 && sender.started != nil {
		close(sender.started)
		<-sender.release
	}
	return Result{Status: inquiries.DeliveryAccepted}
}

func (sender *recordingSender) count() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return len(sender.calls)
}

func TestManagerRuntimeFailureIsVisibleForClaimAndCompletion(t *testing.T) {
	for _, test := range []struct {
		name      string
		store     *failingRuntimeStore
		wantStage string
	}{
		{name: "claim", store: &failingRuntimeStore{claimErr: errors.New("claim unavailable")}, wantStage: "claim"},
		{name: "completion", store: &failingRuntimeStore{completeErr: errors.New("completion unavailable")}, wantStage: "completion"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, err := NewManager(test.store, &recordingSender{})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				health := manager.RuntimeHealth()
				if health.ConsecutiveFailures > 0 {
					if !health.Running || health.FailureStage != test.wantStage || health.LastFailureUTC.IsZero() {
						t.Fatalf("SMTP runtime health=%+v", health)
					}
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
			t.Fatal("SMTP runtime failure did not become observable")
		})
	}
}

func TestManagerDrainFinishesInFlightAndLeavesUnstartedRecipientDurable(t *testing.T) {
	store, owner := deliveryTestStore(t)
	if _, err := store.UpdateRFQRecipientSettings(t.Context(), owner.ID, 1, []inquiries.Recipient{
		{Kind: inquiries.RecipientEmail, Email: "first@example.test"},
		{Kind: inquiries.RecipientEmail, Email: "second@example.test"},
	}); err != nil {
		t.Fatal(err)
	}
	submissionKey, _ := sqlite.NewKey()
	receipt, err := store.SubmitRFQ(t.Context(), submissionKey, inquiries.Submission{
		Name: "Buyer", Email: "buyer@example.test",
		Items: []inquiries.Item{{Kind: "requested", Requested: "MGR-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveryKey, _ := sqlite.NewKey()
	attempt, err := store.CreateSMTPDeliveryAttempt(t.Context(), owner.ID, receipt.RFQID, 1, deliveryKey)
	if err != nil {
		t.Fatal(err)
	}

	sender := &recordingSender{started: make(chan struct{}), release: make(chan struct{})}
	manager, err := NewManager(store, sender)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-sender.started:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery did not start")
	}
	manager.BeginDrain()
	closed := make(chan struct{})
	go func() {
		manager.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("manager close did not drain the in-flight delivery")
	default:
	}
	close(sender.release)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not finish after in-flight delivery completed")
	}
	if sender.count() != 1 {
		t.Fatalf("draining manager started %d deliveries", sender.count())
	}
	partial, err := store.SMTPDeliveryAttempt(t.Context(), attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Recipients[0].Status != inquiries.DeliveryAccepted || partial.Recipients[1].Status != inquiries.DeliveryPending {
		t.Fatalf("post-drain attempt = %#v", partial)
	}

	restartedSender := &recordingSender{}
	restarted, err := NewManager(store, restartedSender)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	waitForDeliveryStatus(t, store, attempt.ID, inquiries.DeliveryAccepted)
	if restartedSender.count() != 1 {
		t.Fatalf("restart sent %d pending recipients, want 1", restartedSender.count())
	}
}

func deliveryTestStore(t *testing.T) (*sqlite.Store, identity.User) {
	t.Helper()
	store, err := sqlite.Create(filepath.Join(t.TempDir(), "delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	password, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: password,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}

func waitForDeliveryStatus(t *testing.T, store *sqlite.Store, attemptID string, status inquiries.DeliveryStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		attempt, err := store.SMTPDeliveryAttempt(t.Context(), attemptID)
		if err == nil && attempt.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	attempt, err := store.SMTPDeliveryAttempt(t.Context(), attemptID)
	t.Fatalf("delivery status = %#v error=%v, want %s", attempt, err, status)
}
