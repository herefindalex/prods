package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/identity"
	"prods/internal/localization"
)

func TestInstallationCompletionIsAtomicAndCreatesMinimumSiteState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ready(context.Background()); !errors.Is(err, ErrDatabaseNotReady) {
		t.Fatalf("installing readiness = %v", err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(context.Background(), Installation{
		OwnerEmail: "Owner@Example.test", OwnerDisplayName: "First Owner", PasswordHash: passwordHash,
		DefaultLocale: "zh-TW", SupportedLocales: []string{"zh-TW", "en-US", "zh-TW"}, TimeZone: "Asia/Taipei",
	})
	if err != nil {
		t.Fatal(err)
	}
	if owner.Role != identity.RoleOwner || owner.Status != identity.UserActive {
		t.Fatalf("owner = %+v", owner)
	}
	if err := store.Ready(context.Background()); err != nil {
		t.Fatalf("completed readiness = %v", err)
	}
	if inspection := Inspect(path); inspection.State != DatabaseReady || inspection.Kind != DatabaseKindSite {
		t.Fatalf("inspection = %+v", inspection)
	}
	var categories, settings, audit int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM categories WHERE system_key IN ('root','uncategorized')`).Scan(&categories); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM site_settings`).Scan(&settings); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM admin_log WHERE action='installation.completed'`).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if categories != 2 || settings != 1 || audit != 1 {
		t.Fatalf("categories=%d settings=%d audit=%d", categories, settings, audit)
	}
	copyCatalog, err := store.PublicCopyCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if copyCatalog.OfficialBundle != localization.OfficialBundleVersion ||
		len(copyCatalog.Defaults) != len(copyCatalog.Definitions)*len(localization.BuiltinLocaleCodes()) {
		t.Fatalf("official public copy was not installed completely: definitions=%d defaults=%d bundle=%q", len(copyCatalog.Definitions), len(copyCatalog.Defaults), copyCatalog.OfficialBundle)
	}
	if _, err := store.CompleteInstallation(context.Background(), Installation{
		OwnerEmail: "other@example.test", PasswordHash: passwordHash, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	}); !errors.Is(err, ErrInstallationState) {
		t.Fatalf("second completion = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenInstalling(path); !errors.Is(err, ErrInstallationState) {
		t.Fatalf("OpenInstalling ready database = %v", err)
	}
}

func TestAdminSessionUsesDigestAndSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prods.db")
	store, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(context.Background(), Installation{
		OwnerEmail: "owner@example.test", PasswordHash: passwordHash, DefaultLocale: "en-US",
		SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	rawToken := "raw-session-token-that-must-not-be-stored"
	if err := store.CreateAdminSession(context.Background(), rawToken, "csrf-token", owner, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := store.db.QueryRow(`SELECT token_digest FROM admin_sessions`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == rawToken || stored != identity.TokenDigest(rawToken) {
		t.Fatalf("stored session credential = %q", stored)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenReady(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.AdminSession(context.Background(), rawToken, time.Now())
	if err != nil || session.User.ID != owner.ID || session.CSRFToken != "csrf-token" {
		t.Fatalf("session after restart = %+v, %v", session, err)
	}
	if err := store.RevokeAdminSession(context.Background(), rawToken); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdminSession(context.Background(), rawToken, time.Now()); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked session = %v", err)
	}
}
