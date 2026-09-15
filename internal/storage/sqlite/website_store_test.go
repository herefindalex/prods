package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func TestWebsiteWorkingSaveAndRestoreDoNotChangeActiveVersion(t *testing.T) {
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}

	initial, err := store.WebsiteState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	working := initial.Working
	working.Organization.DisplayName = "Example Components"
	saved, err := store.SaveWebsiteWorking(t.Context(), owner.ID, initial.WorkingRevision, working)
	if err != nil {
		t.Fatal(err)
	}
	if saved.WorkingRevision != initial.WorkingRevision+1 || saved.Working.Organization.DisplayName != "Example Components" {
		t.Fatalf("unexpected saved state: %+v", saved)
	}
	if saved.ActiveVersion != initial.ActiveVersion || saved.Active.Organization.DisplayName != initial.Active.Organization.DisplayName {
		t.Fatalf("working save changed active version: before=%+v after=%+v", initial, saved)
	}
	if _, err := store.SaveWebsiteWorking(t.Context(), owner.ID, initial.WorkingRevision, working); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale save error=%v", err)
	}

	versions, err := store.WebsiteVersions(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].SiteEpoch != initial.ActiveEpoch {
		t.Fatalf("initial versions=%+v", versions)
	}
	restored, err := store.RestoreWebsiteVersion(t.Context(), owner.ID, saved.WorkingRevision, versions[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Working.Organization.DisplayName != initial.Active.Organization.DisplayName || restored.ActiveVersion != initial.ActiveVersion {
		t.Fatalf("restore did not create working copy only: %+v", restored)
	}
}

type websiteVisibilityStub struct{}

func (websiteVisibilityStub) LockVisibility()                         {}
func (websiteVisibilityStub) InstallVisibility(context.Context) error { return nil }
func (websiteVisibilityStub) UnlockVisibility()                       {}

func TestCustomCSSSafeModeRequiresOwnerAndIsDurable(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "prods.db")
	store, err := Create(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	passwordHash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(t.Context(), Installation{
		OwnerEmail: "owner@example.test", OwnerDisplayName: "Owner", PasswordHash: passwordHash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	role, err := store.CreateRole(t.Context(), owner.ID, identity.Role{
		Name: "Website manager", Capabilities: []identity.Capability{identity.CapabilityAdminAccess, identity.CapabilitySystemManage},
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.CreateUser(t.Context(), owner.ID, "manager@example.test", "Manager", role.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	managerHash, err := identity.HashPassword("managerpass1")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := store.SetUserPassword(t.Context(), grant.Token, managerHash, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetWebsiteCustomCSSDisabled(t.Context(), manager.ID, true, websiteVisibilityStub{}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("non-owner safe mode error=%v", err)
	}
	if err := store.SetWebsiteCustomCSSDisabled(t.Context(), owner.ID, true, websiteVisibilityStub{}); err != nil {
		t.Fatal(err)
	}
	disabled, generation, err := store.WebsiteCustomCSSState(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !disabled || generation != 2 {
		t.Fatalf("safe mode disabled=%v generation=%d", disabled, generation)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReady(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	disabled, generation, err = reopened.WebsiteCustomCSSState(t.Context())
	if err != nil || !disabled || generation != 2 {
		t.Fatalf("restarted safe mode disabled=%v generation=%d error=%v", disabled, generation, err)
	}
}
