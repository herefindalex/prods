package webapp

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"prods/internal/inquiries"
	"prods/internal/maildelivery"
	"prods/internal/storage/sqlite"
)

type webMailSender struct {
	mu    sync.Mutex
	calls []maildelivery.Message
}

func (sender *webMailSender) Send(_ context.Context, message maildelivery.Message) maildelivery.Result {
	sender.mu.Lock()
	sender.calls = append(sender.calls, message)
	sender.mu.Unlock()
	return maildelivery.Result{Status: inquiries.DeliveryAccepted}
}

func (sender *webMailSender) count() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return len(sender.calls)
}

func TestAdminRFQLifecycleAndRecipientAPIs(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	key, err := sqlite.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Ada Buyer", Email: "ada@example.test", GeneralMessage: "Need samples",
		Items: []inquiries.Item{{Kind: "requested", Requested: "TPS54331DR"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	usersResponse, err := fixture.client.Get(fixture.server.URL + "/admin/api/rfq-recipient-users")
	if err != nil {
		t.Fatal(err)
	}
	if usersResponse.StatusCode != http.StatusOK {
		t.Fatalf("recipient users status = %d", usersResponse.StatusCode)
	}
	var recipientUsers []inquiries.RecipientUserOption
	decodeResponseJSON(t, usersResponse, &recipientUsers)
	if len(recipientUsers) != 1 || recipientUsers[0].ID != fixture.owner.ID {
		t.Fatalf("recipient users = %#v", recipientUsers)
	}
	settingsURL := fixture.server.URL + "/admin/api/rfq-recipient-settings"
	settingsResponse, err := fixture.client.Get(settingsURL)
	if err != nil {
		t.Fatal(err)
	}
	var settings inquiries.RecipientSettings
	decodeResponseJSON(t, settingsResponse, &settings)
	if settings.Revision != 1 || len(settings.Recipients) != 0 {
		t.Fatalf("initial recipient settings = %#v", settings)
	}
	settingsUpdate := putAdminJSON(t, fixture.client, settingsURL, fixture.csrf,
		`{"expected_revision":1,"recipients":[{"kind":"email","email":"default@example.test"}]}`)
	if settingsUpdate.StatusCode != http.StatusOK {
		t.Fatalf("recipient settings update = %d body=%s", settingsUpdate.StatusCode, responseBody(t, settingsUpdate))
	}
	decodeResponseJSON(t, settingsUpdate, &settings)
	if settings.Revision != 2 || len(settings.Recipients) != 1 || settings.Recipients[0].Email != "default@example.test" {
		t.Fatalf("updated recipient settings = %#v", settings)
	}
	staleSettings := putAdminJSON(t, fixture.client, settingsURL, fixture.csrf,
		`{"expected_revision":1,"recipients":[]}`)
	if staleSettings.StatusCode != http.StatusConflict {
		t.Fatalf("stale recipient settings = %d", staleSettings.StatusCode)
	}
	staleSettings.Body.Close()

	statusURL := fixture.server.URL + "/admin/api/rfqs/" + receipt.RFQID + "/status"
	withoutCSRF := putAdminJSON(t, fixture.client, statusURL, "", `{"expected_revision":1,"status":"in_progress"}`)
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("status update without CSRF = %d", withoutCSRF.StatusCode)
	}
	withoutCSRF.Body.Close()

	updatedResponse := putAdminJSON(t, fixture.client, statusURL, fixture.csrf, `{"expected_revision":1,"status":"in_progress"}`)
	if updatedResponse.StatusCode != http.StatusOK {
		t.Fatalf("status update = %d body=%s", updatedResponse.StatusCode, responseBody(t, updatedResponse))
	}
	var updated sqlite.RFQSummary
	decodeResponseJSON(t, updatedResponse, &updated)
	if updated.Status != inquiries.StatusInProgress || updated.Revision != 2 || updated.UpdatedBy != fixture.owner.ID {
		t.Fatalf("status result = %#v", updated)
	}

	stale := putAdminJSON(t, fixture.client, statusURL, fixture.csrf, `{"expected_revision":1,"status":"spam"}`)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale status update = %d", stale.StatusCode)
	}
	stale.Body.Close()
	invalid := putAdminJSON(t, fixture.client, statusURL, fixture.csrf, `{"expected_revision":2,"status":"new"}`)
	if invalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid status transition = %d", invalid.StatusCode)
	}
	invalid.Body.Close()

	usersBefore, err := fixture.store.ListUsers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	recipientsURL := fixture.server.URL + "/admin/api/rfqs/" + receipt.RFQID + "/recipients"
	recipients := putAdminJSON(t, fixture.client, recipientsURL, fixture.csrf,
		`{"expected_revision":2,"recipients":[{"kind":"user","user_id":"`+fixture.owner.ID+`"},{"kind":"email","email":"Sales@Example.test"}]}`)
	if recipients.StatusCode != http.StatusOK {
		t.Fatalf("recipient update = %d body=%s", recipients.StatusCode, responseBody(t, recipients))
	}
	var assigned sqlite.RFQSummary
	decodeResponseJSON(t, recipients, &assigned)
	if assigned.Revision != 3 || len(assigned.Recipients) != 2 || assigned.Recipients[1].Email != "sales@example.test" {
		t.Fatalf("recipient result = %#v", assigned)
	}
	usersAfter, err := fixture.store.ListUsers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(usersAfter) != len(usersBefore) {
		t.Fatalf("external recipient created an Admin user: before=%d after=%d", len(usersBefore), len(usersAfter))
	}

	listResponse, err := fixture.client.Get(fixture.server.URL + "/admin/api/rfqs")
	if err != nil {
		t.Fatal(err)
	}
	var listed []sqlite.RFQSummary
	decodeResponseJSON(t, listResponse, &listed)
	if len(listed) != 1 || listed[0].Status != inquiries.StatusInProgress || len(listed[0].Recipients) != 2 {
		t.Fatalf("listed RFQs = %#v", listed)
	}

	deliveryURL := fixture.server.URL + "/admin/api/rfqs/" + receipt.RFQID + "/deliveries"
	deliveryKey, err := sqlite.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	unconfigured := postAdminJSON(t, fixture.client, deliveryURL, fixture.csrf,
		`{"expected_revision":3,"delivery_key":"`+deliveryKey+`"}`)
	if unconfigured.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("unconfigured SMTP delivery = %d", unconfigured.StatusCode)
	}
	unconfigured.Body.Close()

	sender := &webMailSender{}
	manager, err := maildelivery.NewManager(fixture.store, sender)
	if err != nil {
		t.Fatal(err)
	}
	fixture.app.mailManager = manager
	createdDelivery := postAdminJSON(t, fixture.client, deliveryURL, fixture.csrf,
		`{"expected_revision":3,"delivery_key":"`+deliveryKey+`"}`)
	if createdDelivery.StatusCode != http.StatusAccepted {
		t.Fatalf("delivery create = %d body=%s", createdDelivery.StatusCode, responseBody(t, createdDelivery))
	}
	var attempt inquiries.DeliveryAttempt
	decodeResponseJSON(t, createdDelivery, &attempt)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		attempt, err = fixture.store.SMTPDeliveryAttempt(t.Context(), attempt.ID)
		if err == nil && attempt.Status == inquiries.DeliveryAccepted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if attempt.Status != inquiries.DeliveryAccepted || sender.count() != 2 {
		t.Fatalf("delivery attempt=%#v sends=%d", attempt, sender.count())
	}
	replayedDelivery := postAdminJSON(t, fixture.client, deliveryURL, fixture.csrf,
		`{"expected_revision":3,"delivery_key":"`+deliveryKey+`"}`)
	if replayedDelivery.StatusCode != http.StatusOK {
		t.Fatalf("delivery replay = %d", replayedDelivery.StatusCode)
	}
	decodeResponseJSON(t, replayedDelivery, &attempt)
	if !attempt.Replay || sender.count() != 2 {
		t.Fatalf("delivery replay attempt=%#v sends=%d", attempt, sender.count())
	}
	deliveriesResponse, err := fixture.client.Get(deliveryURL)
	if err != nil {
		t.Fatal(err)
	}
	var deliveries []inquiries.DeliveryAttempt
	decodeResponseJSON(t, deliveriesResponse, &deliveries)
	if len(deliveries) != 1 || deliveries[0].Status != inquiries.DeliveryAccepted {
		t.Fatalf("listed deliveries = %#v", deliveries)
	}
}

func TestAdminRFQAnonymizationRequiresCSRFAndReturnsScrubbedRecord(t *testing.T) {
	fixture := newBackupWebFixture(t, nil)
	key, err := sqlite.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := fixture.store.SubmitRFQ(t.Context(), key, inquiries.Submission{
		Name: "Privacy Buyer", Email: "privacy@example.test", GeneralMessage: "remove this message",
		Items: []inquiries.Item{{Kind: "requested", Requested: "PRIVATE-REQUEST", Notes: "remove this note"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := fixture.server.URL + "/admin/api/rfqs/" + receipt.RFQID + "/anonymize"
	withoutCSRF := postAdminJSON(t, fixture.client, endpoint, "", `{"expected_revision":1}`)
	if withoutCSRF.StatusCode != http.StatusForbidden {
		t.Fatalf("anonymize without CSRF=%d", withoutCSRF.StatusCode)
	}
	withoutCSRF.Body.Close()

	response := postAdminJSON(t, fixture.client, endpoint, fixture.csrf, `{"expected_revision":1}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("anonymize status=%d body=%s", response.StatusCode, responseBody(t, response))
	}
	var anonymized sqlite.RFQSummary
	decodeResponseJSON(t, response, &anonymized)
	if anonymized.PrivacyState != "anonymized" || anonymized.Revision != 2 || anonymized.Email != "" ||
		anonymized.GeneralMessage != "" || len(anonymized.Items) != 1 || anonymized.Items[0].Requested != "" || anonymized.Items[0].Notes != "" {
		t.Fatalf("anonymized response=%#v", anonymized)
	}

	stale := postAdminJSON(t, fixture.client, endpoint, fixture.csrf, `{"expected_revision":1}`)
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale anonymization=%d body=%s", stale.StatusCode, responseBody(t, stale))
	}
}
