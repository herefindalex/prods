package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"prods/internal/identity"
	"prods/internal/inquiries"
	"prods/internal/maildelivery"
	"prods/internal/storage/sqlite"
)

type invitationTestSender struct {
	mu       sync.Mutex
	result   maildelivery.Result
	messages []maildelivery.Message
}

func (s *invitationTestSender) Send(_ context.Context, message maildelivery.Message) maildelivery.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return s.result
}

func (s *invitationTestSender) setResult(result maildelivery.Result) {
	s.mu.Lock()
	s.result = result
	s.mu.Unlock()
}

func (s *invitationTestSender) snapshot() []maildelivery.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]maildelivery.Message(nil), s.messages...)
}

type invitationWebFixture struct {
	store  *sqlite.Store
	app    *Server
	server *httptest.Server
	client *http.Client
	csrf   string
	owner  identity.User
	grant  identity.SetPasswordGrant
}

func newInvitationWebFixture(t *testing.T, sender maildelivery.Sender) invitationWebFixture {
	t.Helper()
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	store, err := sqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateUser(t.Context(), owner.ID, "invitee@example.test", "Invitee", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", DatabasePath: databasePath,
		WorkDir: filepath.Join(root, "work"), AssetDir: filepath.Join(root, "assets"),
		PublicDir: filepath.Join(root, "public"), BackupDir: filepath.Join(root, "backups"),
		MailSender: sender,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app)
	client := newCookieClient(t)
	csrf := loginNormalAdmin(t, client, server.URL, owner.Email, "ownerpass1")
	t.Cleanup(func() {
		server.Close()
		app.Close()
		_ = store.Close()
	})
	return invitationWebFixture{store: store, app: app, server: server, client: client, csrf: csrf, owner: owner, grant: grant}
}

func TestAdminUserInvitationMailRequiresConfiguredSMTPAndValidSameOriginURL(t *testing.T) {
	fixture := newInvitationWebFixture(t, nil)
	validURL := "https://catalog.example.test/set-password?token=" + fixture.grant.Token
	response := postInvitationURL(t, fixture, validURL)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("SMTP missing status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	sender := &invitationTestSender{result: maildelivery.Result{Status: inquiries.DeliveryAccepted}}
	configured := newInvitationWebFixture(t, sender)
	invalidURLs := []string{
		"https://attacker.example/set-password?token=" + configured.grant.Token,
		"https://catalog.example.test/admin?token=" + configured.grant.Token,
		"https://catalog.example.test/set-password?token=" + configured.grant.Token + "&next=/admin",
		"/set-password?token=" + configured.grant.Token,
	}
	for _, invalidURL := range invalidURLs {
		response := postInvitationURL(t, configured, invalidURL)
		if response.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("invalid URL %q status = %d", invalidURL, response.StatusCode)
		}
		_ = response.Body.Close()
	}
	if messages := sender.snapshot(); len(messages) != 0 {
		t.Fatalf("invalid URLs sent %d messages", len(messages))
	}
}

func TestAdminUserInvitationMailPersistsAcceptedFailedAndUnknownResults(t *testing.T) {
	sender := &invitationTestSender{result: maildelivery.Result{Status: inquiries.DeliveryAccepted}}
	fixture := newInvitationWebFixture(t, sender)
	setPasswordURL := "https://catalog.example.test/set-password?token=" + fixture.grant.Token

	accepted := sendInvitationAndDecode(t, fixture, setPasswordURL, http.StatusOK)
	if accepted.Status != identity.InvitationMailAccepted || accepted.CompletedAt == "" {
		t.Fatalf("accepted attempt = %+v", accepted)
	}
	sender.setResult(maildelivery.Result{Status: inquiries.DeliveryFailed, ErrorClass: "recipient", ErrorMessage: "mailbox unavailable"})
	failed := sendInvitationAndDecode(t, fixture, setPasswordURL, http.StatusOK)
	if failed.Status != identity.InvitationMailFailed || failed.ErrorClass != "recipient" || failed.ErrorMessage != "mailbox unavailable" {
		t.Fatalf("failed attempt = %+v", failed)
	}
	sender.setResult(maildelivery.Result{Status: inquiries.DeliveryUnknown, ErrorClass: "data", ErrorMessage: "connection lost after DATA"})
	unknown := sendInvitationAndDecode(t, fixture, setPasswordURL, http.StatusOK)
	if unknown.Status != identity.InvitationMailUnknown || unknown.ErrorClass != "data" {
		t.Fatalf("unknown attempt = %+v", unknown)
	}
	retry := postInvitationURL(t, fixture, setPasswordURL)
	if retry.StatusCode != http.StatusConflict {
		t.Fatalf("same-grant retry after unknown status = %d", retry.StatusCode)
	}
	_ = retry.Body.Close()

	messages := sender.snapshot()
	if len(messages) != 3 || messages[0].To != fixture.grant.User.Email ||
		!strings.Contains(messages[0].Body, setPasswordURL) {
		t.Fatalf("messages = %+v", messages)
	}
	attemptsResponse, err := fixture.client.Get(fixture.server.URL + "/admin/api/users/" + fixture.grant.User.ID + "/invitation-mail-attempts")
	if err != nil {
		t.Fatal(err)
	}
	defer attemptsResponse.Body.Close()
	if attemptsResponse.StatusCode != http.StatusOK {
		t.Fatalf("attempt list status = %d", attemptsResponse.StatusCode)
	}
	var attempts []identity.InvitationMailAttempt
	if err := json.NewDecoder(attemptsResponse.Body).Decode(&attempts); err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 3 || attempts[0].Status != identity.InvitationMailUnknown {
		t.Fatalf("attempt list = %+v", attempts)
	}

	newGrant, err := fixture.store.IssueSetPasswordGrant(t.Context(), fixture.owner.ID, fixture.grant.User.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	sender.setResult(maildelivery.Result{Status: inquiries.DeliveryAccepted})
	newURL := "https://catalog.example.test/set-password?token=" + newGrant.Token
	rotated := sendInvitationAndDecode(t, fixture, newURL, http.StatusOK)
	if rotated.Status != identity.InvitationMailAccepted {
		t.Fatalf("rotated grant attempt = %+v", rotated)
	}
	old := postInvitationURL(t, fixture, setPasswordURL)
	if old.StatusCode != http.StatusConflict {
		t.Fatalf("old grant status = %d", old.StatusCode)
	}
	_ = old.Body.Close()
}

func TestServerStartupReconcilesPendingInvitationMailToUnknown(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "prods.db")
	store, err := sqlite.Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), sqlite.Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateUser(t.Context(), owner.ID, "restart@example.test", "Restart", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserInvitationMailAttempt(t.Context(), owner.ID, grant.User.ID, grant.Token); err != nil {
		t.Fatal(err)
	}
	app, _, err := New(store, Config{
		BaseURL: "https://catalog.example.test", DatabasePath: databasePath,
		WorkDir: filepath.Join(root, "work"), AssetDir: filepath.Join(root, "assets"),
		PublicDir: filepath.Join(root, "public"), BackupDir: filepath.Join(root, "backups"),
	})
	if err != nil {
		t.Fatal(err)
	}
	app.Close()
	attempts, err := store.ListUserInvitationMailAttempts(t.Context(), grant.User.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Status != identity.InvitationMailUnknown || attempts[0].ErrorClass != "restart_ambiguous" {
		t.Fatalf("startup reconciliation = %+v", attempts)
	}
}

func postInvitationURL(t *testing.T, fixture invitationWebFixture, setPasswordURL string) *http.Response {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"set_password_url": setPasswordURL})
	if err != nil {
		t.Fatal(err)
	}
	return postAdminJSON(t, fixture.client,
		fixture.server.URL+"/admin/api/users/"+fixture.grant.User.ID+"/send-set-password",
		fixture.csrf, string(payload))
}

func sendInvitationAndDecode(t *testing.T, fixture invitationWebFixture, setPasswordURL string, wantStatus int) identity.InvitationMailAttempt {
	t.Helper()
	response := postInvitationURL(t, fixture, setPasswordURL)
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		t.Fatalf("send invitation status = %d", response.StatusCode)
	}
	var attempt identity.InvitationMailAttempt
	if err := json.NewDecoder(response.Body).Decode(&attempt); err != nil {
		t.Fatal(err)
	}
	return attempt
}
