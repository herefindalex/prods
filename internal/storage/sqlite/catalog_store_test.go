package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func installedStore(t *testing.T) (*Store, identity.User) {
	t.Helper()
	store, err := Create(filepath.Join(t.TempDir(), "prods.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	hash, err := identity.HashPassword("ownerpass1")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := store.CompleteInstallation(context.Background(), Installation{
		OwnerEmail: "owner@example.test", PasswordHash: hash,
		DefaultLocale: "en-US", SupportedLocales: []string{"en-US"}, TimeZone: "UTC",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, owner
}

func TestControlledProductWritesEnforceIdentityRevisionArchiveAndDurableEvidence(t *testing.T) {
	ctx := context.Background()
	store, owner := installedStore(t)

	created, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "  ABC123  "})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.PartNumber != "  ABC123  " || created.IdentityPart != "ABC123" {
		t.Fatalf("created identity = %+v", created)
	}
	if created.RecordState != catalog.RecordCurrent || created.Status != catalog.Hidden || created.CategoryID != catalog.UncategorizedCategoryID {
		t.Fatalf("created defaults = %+v", created)
	}

	if _, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "ABC123"}); !errors.Is(err, catalog.ErrIdentityConflict) {
		t.Fatalf("trimmed duplicate = %v", err)
	}
	caseVariant, err := store.CreateProduct(ctx, owner.ID, catalog.Product{PartNumber: "abc123"})
	if err != nil {
		t.Fatalf("case-sensitive identity variant: %v", err)
	}
	if caseVariant.ID == created.ID {
		t.Fatal("case variant reused product identity")
	}

	created.Name = "First saved name"
	updated, err := store.UpdateProduct(ctx, owner.ID, created.Revision, created)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.Name != "First saved name" {
		t.Fatalf("updated = %+v", updated)
	}
	stale := created
	stale.Name = "must not win"
	if _, err := store.UpdateProduct(ctx, owner.ID, created.Revision, stale); !errors.Is(err, catalog.ErrRevisionConflict) {
		t.Fatalf("stale update = %v", err)
	}
	persisted, err := store.Product(ctx, created.ID)
	if err != nil || persisted.Name != "First saved name" || persisted.Revision != 2 {
		t.Fatalf("persisted after stale update = %+v, %v", persisted, err)
	}

	if err := store.ArchiveProduct(ctx, owner.ID, created.ID, updated.Revision); err != nil {
		t.Fatal(err)
	}
	archived, err := store.Product(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.RecordState != catalog.RecordArchived || archived.Status != catalog.Hidden || archived.Revision != 3 {
		t.Fatalf("archived = %+v", archived)
	}
	if _, err := store.UpdateProduct(ctx, owner.ID, archived.Revision, archived); !errors.Is(err, catalog.ErrArchivedProduct) {
		t.Fatalf("archived update = %v", err)
	}
	if _, err := store.PublishedProduct(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("archived public lookup = %v", err)
	}

	var auditCount, intentCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_log WHERE target_id=?`, created.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM publication_intents WHERE entity_id=?`, created.ID).Scan(&intentCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 3 || intentCount != 3 {
		t.Fatalf("durable mutation evidence audit=%d intent=%d", auditCount, intentCount)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE admin_log SET result='changed' WHERE target_id=?`, created.ID); err == nil {
		t.Fatal("Admin Log update unexpectedly succeeded")
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM admin_log WHERE target_id=?`, created.ID); err == nil {
		t.Fatal("Admin Log delete unexpectedly succeeded")
	}
}

func TestControlledWriteRechecksCapabilityInsideTransaction(t *testing.T) {
	ctx := context.Background()
	store, _ := installedStore(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	hash, err := identity.HashPassword("viewerpass1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO roles(id,name,status,created_at,updated_at) VALUES('role_viewer','Viewer','active',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO role_capabilities(role_id,capability) VALUES('role_viewer',?)`, identity.CapabilityCatalogView); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO users(
		id,email,email_normalized,display_name,role,status,password_hash,auth_revision,created_at,updated_at
	) VALUES('usr_viewer','viewer@example.test','viewer@example.test','Viewer','role_viewer','active',?,1,?,?)`, hash, now, now); err != nil {
		t.Fatal(err)
	}

	if _, err := store.CreateProduct(ctx, "usr_viewer", catalog.Product{PartNumber: "DENIED-1"}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("viewer create = %v", err)
	}
	var deniedCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE identity_part_number='DENIED-1'`).Scan(&deniedCount); err != nil {
		t.Fatal(err)
	}
	if deniedCount != 0 {
		t.Fatal("permission failure left a product behind")
	}

	if _, err := store.db.ExecContext(ctx, `INSERT INTO role_capabilities(role_id,capability) VALUES('role_viewer',?)`, identity.CapabilityCatalogEdit); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateProduct(ctx, "usr_viewer", catalog.Product{PartNumber: "EDIT-OK"}); err != nil {
		t.Fatalf("catalog editor create hidden: %v", err)
	}
	if _, err := store.CreateProduct(ctx, "usr_viewer", catalog.Product{PartNumber: "PUBLISH-DENIED", Status: catalog.Published}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("publish without capability = %v", err)
	}
}
