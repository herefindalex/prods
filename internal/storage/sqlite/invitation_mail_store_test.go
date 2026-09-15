package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"prods/internal/identity"
)

func TestUserInvitationMailAttemptIsDurableAndDoesNotPersistBearerToken(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	grant, err := store.CreateUser(ctx, owner.ID, "invitee@example.test", "Invitee", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	attempt, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, grant.User.ID, grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Status != identity.InvitationMailPending || attempt.RecipientEmail != grant.User.Email || attempt.AuthRevision != int64(grant.User.AuthRevision) {
		t.Fatalf("pending attempt = %+v", attempt)
	}
	var digest, status string
	if err := store.db.QueryRowContext(ctx, `SELECT token_digest,status FROM user_invitation_mail_attempts WHERE id=?`, attempt.ID).Scan(&digest, &status); err != nil {
		t.Fatal(err)
	}
	if digest != identity.TokenDigest(grant.Token) || digest == grant.Token || status != string(identity.InvitationMailPending) {
		t.Fatalf("stored digest=%q status=%q", digest, status)
	}
	var leaked int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_invitation_mail_attempts
		WHERE token_digest=? OR recipient_email LIKE ? OR error_message LIKE ?`, grant.Token, "%"+grant.Token+"%", "%"+grant.Token+"%").Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked != 0 {
		t.Fatal("raw bearer token was persisted in invitation attempt")
	}

	completed, err := store.CompleteUserInvitationMailAttempt(ctx, attempt.ID, identity.InvitationMailAccepted, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != identity.InvitationMailAccepted || completed.CompletedAt == "" {
		t.Fatalf("completed attempt = %+v", completed)
	}
	attempts, err := store.ListUserInvitationMailAttempts(ctx, grant.User.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Status != identity.InvitationMailAccepted {
		t.Fatalf("attempts = %+v", attempts)
	}
	if _, err := store.CompleteUserInvitationMailAttempt(ctx, attempt.ID, identity.InvitationMailFailed, "retry", "must not overwrite"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("second completion = %v", err)
	}

	newGrant, err := store.IssueSetPasswordGrant(ctx, owner.ID, grant.User.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if newGrant.Token == grant.Token {
		t.Fatal("grant was not rotated")
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, grant.User.ID, grant.Token); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("old grant accepted after reissue: %v", err)
	}
}

func TestUserInvitationMailAttemptValidationAndRestartReconciliation(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	grant, err := store.CreateUser(ctx, owner.ID, "pending@example.test", "Pending", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, grant.User.ID, "wrong-token"); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("wrong token = %v", err)
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, "missing-actor", grant.User.ID, grant.Token); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("unauthorized actor = %v", err)
	}

	attempt, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, grant.User.ID, grant.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReconcileUserInvitationMailAttempts(ctx); err != nil {
		t.Fatal(err)
	}
	attempts, err := store.ListUserInvitationMailAttempts(ctx, grant.User.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].ID != attempt.ID || attempts[0].Status != identity.InvitationMailUnknown ||
		attempts[0].ErrorClass != "restart_ambiguous" || attempts[0].CompletedAt == "" {
		t.Fatalf("reconciled attempts = %+v", attempts)
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, grant.User.ID, grant.Token); !errors.Is(err, ErrInvitationMailUnknown) {
		t.Fatalf("same grant accepted after unknown outcome: %v", err)
	}
	var details string
	if err := store.db.QueryRowContext(ctx, `SELECT details_json FROM admin_log
		WHERE action='user.invitation_mail_reconciled_unknown' AND target_id=?`, attempt.ID).Scan(&details); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(details, grant.Token) || !strings.Contains(details, grant.User.ID) {
		t.Fatalf("reconciliation audit = %s", details)
	}
}

func TestUserInvitationMailAttemptRejectsExpiredUsedAndDisabledGrants(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	grant, err := store.CreateUser(ctx, owner.ID, "expired@example.test", "Expired", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE set_password_tokens SET expires_at=? WHERE token_digest=?`,
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), identity.TokenDigest(grant.Token)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, grant.User.ID, grant.Token); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("expired grant = %v", err)
	}

	used, err := store.CreateUser(ctx, owner.ID, "used@example.test", "Used", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := identity.HashPassword("used-password-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetUserPassword(ctx, used.Token, hash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, used.User.ID, used.Token); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("used grant = %v", err)
	}

	disabled, err := store.CreateUser(ctx, owner.ID, "disabled@example.test", "Disabled", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DisableUser(ctx, owner.ID, disabled.User.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateUserInvitationMailAttempt(ctx, owner.ID, disabled.User.ID, disabled.Token); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("disabled grant = %v", err)
	}
}
