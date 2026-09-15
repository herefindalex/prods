package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"prods/internal/identity"
)

func TestUserGrantDisableAndLastOwnerContracts(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	viewerRole, err := store.CreateRole(ctx, owner.ID, identity.Role{Name: "Catalog viewer", Capabilities: []identity.Capability{
		identity.CapabilityAdminAccess, identity.CapabilityCatalogView,
	}})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateUser(ctx, owner.ID, "viewer@example.test", "Viewer", viewerRole.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if grant.Token == "" || grant.User.PasswordHash != "" {
		t.Fatalf("grant = %+v", grant)
	}
	var storedToken string
	if err := store.db.QueryRowContext(ctx, `SELECT token_digest FROM set_password_tokens WHERE user_id=?`, grant.User.ID).Scan(&storedToken); err != nil {
		t.Fatal(err)
	}
	if storedToken == grant.Token || storedToken != identity.TokenDigest(grant.Token) {
		t.Fatalf("stored grant token = %q", storedToken)
	}
	if _, err := store.ActiveUserByEmail(ctx, grant.User.Email); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login before set-password = %v", err)
	}
	passwordHash, err := identity.HashPassword("viewerpass1")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := store.SetUserPassword(ctx, grant.Token, passwordHash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetUserPassword(ctx, grant.Token, passwordHash, time.Now().UTC()); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("reused set-password grant = %v", err)
	}
	viewer, err = store.ActiveUserByEmail(ctx, viewer.Email)
	if err != nil || !viewer.Can(identity.CapabilityCatalogView) || viewer.Can(identity.CapabilityCatalogEdit) {
		t.Fatalf("viewer capabilities = %+v, %v", viewer.Capabilities, err)
	}
	sessionToken := "viewer-session-token"
	if err := store.CreateAdminSession(ctx, sessionToken, "csrf", viewer, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.DisableUser(ctx, owner.ID, viewer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdminSession(ctx, sessionToken, time.Now()); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled session = %v", err)
	}
	if _, err := store.ActiveUserByEmail(ctx, viewer.Email); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled password login = %v", err)
	}

	if err := store.DisableUser(ctx, owner.ID, owner.ID); !errors.Is(err, ErrLastActiveOwner) {
		t.Fatalf("last Owner disable = %v", err)
	}
	secondGrant, err := store.CreateUser(ctx, owner.ID, "owner2@example.test", "Owner Two", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ownerHash, err := identity.HashPassword("ownerpass2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetUserPassword(ctx, secondGrant.Token, ownerHash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.DisableUser(ctx, owner.ID, owner.ID); err != nil {
		t.Fatalf("disable with another usable Owner = %v", err)
	}
}

func TestOwnerConsoleRecoveryGrantIsScopedOneTimeAndRevokesSessionsOnUse(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	oldSession := "owner-session-before-recovery"
	if err := store.CreateAdminSession(ctx, oldSession, "csrf-before-recovery", owner, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC()
	first, err := store.CreateOwnerRecoveryGrant(ctx, "  OWNER@EXAMPLE.TEST ", 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.User.ID != owner.ID || first.Token == "" || first.ExpiresAt.Before(before.Add(29*time.Minute)) ||
		first.ExpiresAt.After(before.Add(31*time.Minute)) {
		t.Fatalf("default recovery grant=%+v", first)
	}
	if _, err := store.AdminSession(ctx, oldSession, time.Now()); err != nil {
		t.Fatalf("creating a grant revoked the session before password reset: %v", err)
	}

	second, err := store.CreateOwnerRecoveryGrant(ctx, owner.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := identity.HashPassword("newownerpass2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetUserPassword(ctx, first.Token, newHash, time.Now().UTC()); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("superseded recovery grant=%v", err)
	}

	var storedDigest, createdBy string
	if err := store.db.QueryRowContext(ctx, `SELECT token_digest,COALESCE(created_by,'')
		FROM set_password_tokens WHERE token_digest=?`, identity.TokenDigest(second.Token)).Scan(&storedDigest, &createdBy); err != nil {
		t.Fatal(err)
	}
	if storedDigest == second.Token || storedDigest != identity.TokenDigest(second.Token) || createdBy != "" {
		t.Fatalf("stored recovery grant digest=%q created_by=%q", storedDigest, createdBy)
	}
	var rawTokenRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM set_password_tokens WHERE token_digest=?`, second.Token).Scan(&rawTokenRows); err != nil {
		t.Fatal(err)
	}
	if rawTokenRows != 0 {
		t.Fatal("raw recovery token was persisted")
	}
	var auditActor sql.NullString
	var auditDetails string
	if err := store.db.QueryRowContext(ctx, `SELECT actor_id,details_json FROM admin_log
		WHERE action='user.owner_recovery_grant_created' ORDER BY created_at DESC LIMIT 1`).Scan(&auditActor, &auditDetails); err != nil {
		t.Fatal(err)
	}
	if auditActor.Valid || !strings.Contains(auditDetails, `"source":"host_console"`) ||
		strings.Contains(auditDetails, first.Token) || strings.Contains(auditDetails, second.Token) {
		t.Fatalf("recovery audit actor=%v details=%s", auditActor, auditDetails)
	}

	updated, err := store.SetUserPassword(ctx, second.Token, newHash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if updated.AuthRevision != owner.AuthRevision+1 {
		t.Fatalf("auth revision=%d want=%d", updated.AuthRevision, owner.AuthRevision+1)
	}
	if _, err := store.AdminSession(ctx, oldSession, time.Now()); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old session after recovery password reset=%v", err)
	}
	persisted, err := store.ActiveUserByEmail(ctx, owner.Email)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := identity.VerifyPassword(persisted.PasswordHash, "newownerpass2"); err != nil || !ok {
		t.Fatalf("recovered password verification ok=%v err=%v", ok, err)
	}
	if _, err := store.SetUserPassword(ctx, second.Token, newHash, time.Now().UTC()); !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("reused recovery grant=%v", err)
	}
}

func TestOwnerConsoleRecoveryRejectsUnknownNonOwnerAndDisabledOwner(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)
	if _, err := store.CreateOwnerRecoveryGrant(ctx, "missing@example.test", time.Hour); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown Owner recovery=%v", err)
	}

	viewerRole, err := store.CreateRole(ctx, owner.ID, identity.Role{Name: "Viewer", Capabilities: []identity.Capability{
		identity.CapabilityAdminAccess,
	}})
	if err != nil {
		t.Fatal(err)
	}
	viewerGrant, err := store.CreateUser(ctx, owner.ID, "viewer-recovery@example.test", "Viewer", viewerRole.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	viewerHash, err := identity.HashPassword("viewerpass2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetUserPassword(ctx, viewerGrant.Token, viewerHash, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOwnerRecoveryGrant(ctx, viewerGrant.User.Email, time.Hour); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("non-Owner recovery=%v", err)
	}

	disabledGrant, err := store.CreateUser(ctx, owner.ID, "disabled-owner@example.test", "Disabled Owner", identity.RoleOwner, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	disabledHash, err := identity.HashPassword("disabledpass2")
	if err != nil {
		t.Fatal(err)
	}
	disabledOwner, err := store.SetUserPassword(ctx, disabledGrant.Token, disabledHash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DisableUser(ctx, owner.ID, disabledOwner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOwnerRecoveryGrant(ctx, disabledOwner.Email, time.Hour); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled Owner recovery=%v", err)
	}
}
