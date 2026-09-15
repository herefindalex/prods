package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
)

func (s *Store) CreateCategory(ctx context.Context, actorID string, category catalog.Category) (catalog.Category, error) {
	if category.ID == "" {
		id, err := randomID("cat")
		if err != nil {
			return catalog.Category{}, err
		}
		category.ID = id
	}
	if category.ParentID == "" {
		category.ParentID = "cat_root"
	}
	category.SystemKey = ""
	category.Revision = 1
	if err := category.Prepare(); err != nil {
		return catalog.Category{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if err := requireActiveCategory(ctx, tx, category.ParentID); err != nil {
			return err
		}
		if err := ensureCategorySlugAvailable(ctx, tx, category.ParentID, category.Slug, category.ID); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO categories(
			id,parent_id,system_key,name,slug,status,revision,created_by,updated_by,created_at,updated_at
		) VALUES(?,?,NULL,?,?,?,?,?,?,?,?)`, category.ID, category.ParentID, category.Name, category.Slug,
			category.Status, category.Revision, actorID, actorID, now, now)
		if err != nil {
			return fmt.Errorf("create category: %w", err)
		}
		if err := appendAudit(ctx, tx, actorID, "category.created", "category", category.ID, map[string]any{
			"parent_id": category.ParentID, "slug": category.Slug,
		}); err != nil {
			return err
		}
		return nil
	})
	return category, err
}

func (s *Store) Category(ctx context.Context, id string) (catalog.Category, error) {
	return scanCategory(s.db.QueryRowContext(ctx, `SELECT id,COALESCE(parent_id,''),COALESCE(system_key,''),name,slug,status,revision
		FROM categories WHERE id=?`, id))
}

func scanCategory(scanner interface{ Scan(...any) error }) (catalog.Category, error) {
	var category catalog.Category
	err := scanner.Scan(&category.ID, &category.ParentID, &category.SystemKey, &category.Name, &category.Slug, &category.Status, &category.Revision)
	return category, err
}

func (s *Store) ListCategories(ctx context.Context) ([]catalog.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(parent_id,''),COALESCE(system_key,''),name,slug,status,revision
		FROM categories ORDER BY CASE WHEN system_key='root' THEN 0 WHEN system_key='uncategorized' THEN 1 ELSE 2 END,name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var categories []catalog.Category
	for rows.Next() {
		category, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (s *Store) MoveCategory(ctx context.Context, actorID, id, newParentID string, expectedRevision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		category, err := scanCategory(tx.QueryRowContext(ctx, `SELECT id,COALESCE(parent_id,''),COALESCE(system_key,''),name,slug,status,revision
			FROM categories WHERE id=?`, id))
		if err != nil {
			return err
		}
		if category.SystemKey != "" {
			return catalog.ErrSystemCategory
		}
		if category.Revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		if err := requireActiveCategory(ctx, tx, newParentID); err != nil {
			return err
		}
		if id == newParentID {
			return catalog.ErrCategoryCycle
		}
		var createsCycle int
		err = tx.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (
			SELECT id FROM categories WHERE parent_id=?
			UNION ALL
			SELECT c.id FROM categories c JOIN descendants d ON c.parent_id=d.id
		) SELECT EXISTS(SELECT 1 FROM descendants WHERE id=?)`, id, newParentID).Scan(&createsCycle)
		if err != nil {
			return err
		}
		if createsCycle != 0 {
			return catalog.ErrCategoryCycle
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE categories SET parent_id=?,revision=revision+1,updated_by=?,updated_at=?
			WHERE id=? AND revision=?`, newParentID, actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if err := appendAudit(ctx, tx, actorID, "category.moved", "category", id, map[string]any{
			"from_parent_id": category.ParentID, "to_parent_id": newParentID,
			"from_revision": expectedRevision, "to_revision": expectedRevision + 1,
		}); err != nil {
			return err
		}
		_, err = requeueAffectedProductsTx(ctx, tx, actorID, "category", id, "category.moved", now)
		return err
	})
}

func (s *Store) DisableCategory(ctx context.Context, actorID, id string, expectedRevision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		category, err := scanCategory(tx.QueryRowContext(ctx, `SELECT id,COALESCE(parent_id,''),COALESCE(system_key,''),name,slug,status,revision
			FROM categories WHERE id=?`, id))
		if err != nil {
			return err
		}
		if category.SystemKey != "" {
			return catalog.ErrSystemCategory
		}
		if category.Revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx, `UPDATE categories SET status='disabled',revision=revision+1,updated_by=?,updated_at=?
			WHERE id=? AND revision=?`, actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "category.disabled", "category", id, map[string]any{"revision": expectedRevision + 1}); err != nil {
			return err
		}
		_, err = requeueAffectedProductsTx(ctx, tx, actorID, "category", id, "category.disabled", now)
		return err
	})
}

func requireActiveCategory(ctx context.Context, tx *sql.Tx, id string) error {
	var exists int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM categories WHERE id=? AND status='active'`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.ErrInvalidCategory
	}
	return err
}

func (s *Store) CreateDictionaryEntry(ctx context.Context, actorID string, entry catalog.DictionaryEntry) (catalog.DictionaryEntry, error) {
	if entry.ID == "" {
		id, err := randomID("dic")
		if err != nil {
			return catalog.DictionaryEntry{}, err
		}
		entry.ID = id
	}
	entry.Revision = 1
	if entry.Slug == "" && routeBearingDictionary(entry.Kind) {
		entry.Slug = catalog.SuggestedSlug(entry.Name, entry.ID)
	}
	if err := entry.Prepare(); err != nil {
		return catalog.DictionaryEntry{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := ensureDictionarySlugAvailable(ctx, tx, entry.Kind, entry.Slug, entry.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO dictionary_entries(
			id,kind,name,slug,status,revision,created_by,updated_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, entry.ID, entry.Kind, entry.Name, entry.Slug, entry.Status, entry.Revision,
			actorID, actorID, now, now)
		if err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "dictionary.created", string(entry.Kind), entry.ID, map[string]any{"name": entry.Name}); err != nil {
			return err
		}
		return nil
	})
	return entry, err
}

func (s *Store) DisableDictionaryEntry(ctx context.Context, actorID, id string, expectedRevision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var kind catalog.DictionaryKind
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT kind,revision FROM dictionary_entries WHERE id=?`, id).Scan(&kind, &revision); err != nil {
			return err
		}
		if revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `UPDATE dictionary_entries SET status='disabled',revision=revision+1,updated_by=?,updated_at=?
			WHERE id=? AND revision=?`, actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "dictionary.disabled", string(kind), id, map[string]any{"revision": expectedRevision + 1}); err != nil {
			return err
		}
		_, err = requeueAffectedProductsTx(ctx, tx, actorID, string(kind), id, "dictionary.disabled", now)
		return err
	})
}

func (s *Store) ListDictionaryEntries(ctx context.Context, kind catalog.DictionaryKind) ([]catalog.DictionaryEntry, error) {
	query := `SELECT id,kind,name,slug,status,revision FROM dictionary_entries`
	var args []any
	if kind != "" {
		query += ` WHERE kind=?`
		args = append(args, kind)
	}
	query += ` ORDER BY kind,name,id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []catalog.DictionaryEntry
	for rows.Next() {
		var entry catalog.DictionaryEntry
		if err := rows.Scan(&entry.ID, &entry.Kind, &entry.Name, &entry.Slug, &entry.Status, &entry.Revision); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func affectedProductIDs(ctx context.Context, tx *sql.Tx, entityType, entityID string) ([]string, error) {
	var query string
	switch entityType {
	case "category":
		query = `WITH RECURSIVE affected(id) AS (
			SELECT id FROM categories WHERE id=?
			UNION ALL
			SELECT c.id FROM categories c JOIN affected a ON c.parent_id=a.id
		) SELECT p.id FROM products p JOIN affected a ON a.id=p.category_id
		WHERE p.record_state='current' ORDER BY p.id`
	case string(catalog.DictionaryManufacturer):
		query = `SELECT id FROM products WHERE record_state='current' AND manufacturer_id=? ORDER BY id`
	case string(catalog.DictionaryBrand):
		query = `SELECT id FROM products WHERE record_state='current' AND brand_id=? ORDER BY id`
	case string(catalog.DictionaryLifecycle):
		query = `SELECT id FROM products WHERE record_state='current' AND lifecycle_id=? ORDER BY id`
	case string(catalog.DictionaryApplication):
		query = `SELECT DISTINCT p.id FROM products p JOIN product_applications pa ON pa.product_id=p.id
			WHERE p.record_state='current' AND pa.application_id=? ORDER BY p.id`
	case string(catalog.DictionaryDocumentType):
		query = `SELECT DISTINCT p.id FROM products p JOIN product_documents pd ON pd.product_id=p.id
			WHERE p.record_state='current' AND pd.document_type_id=? ORDER BY p.id`
	default:
		return nil, catalog.ErrInvalidDictionary
	}
	rows, err := tx.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func taxonomyImpactTx(ctx context.Context, tx *sql.Tx, entityType, entityID string) (catalog.TaxonomyImpact, error) {
	ids, err := affectedProductIDs(ctx, tx, entityType, entityID)
	if err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	impact := catalog.TaxonomyImpact{EntityType: entityType, EntityID: entityID, AffectedProducts: make([]catalog.TaxonomyProductImpact, 0, len(ids))}
	var routeConfig publishing.SiteRouteConfig
	if err := tx.QueryRowContext(ctx, `SELECT product_prefix,url_pattern FROM public_site_state WHERE singleton=1`).Scan(&routeConfig.ProductPrefix, &routeConfig.Pattern); err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	for _, id := range ids {
		product, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, id))
		if err != nil {
			return catalog.TaxonomyImpact{}, err
		}
		item := catalog.TaxonomyProductImpact{ProductID: product.ID, PartNumber: product.PartNumber}
		if err := tx.QueryRowContext(ctx, `SELECT route FROM public_activations WHERE product_id=?`, product.ID).Scan(&item.CurrentRoute); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return catalog.TaxonomyImpact{}, err
		}
		if product.Status == catalog.Published {
			source := publishing.Source{Product: product}
			source.Category, source.CategoryPath, err = publicCategoryIdentityQuery(ctx, tx, product.CategoryID)
			if err != nil {
				return catalog.TaxonomyImpact{}, err
			}
			if product.ManufacturerID != "" {
				if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.ManufacturerID).Scan(&source.ManufacturerSlug); err != nil {
					return catalog.TaxonomyImpact{}, err
				}
			}
			if product.BrandID != "" {
				if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.BrandID).Scan(&source.BrandSlug); err != nil {
					return catalog.TaxonomyImpact{}, err
				}
			}
			item.ProposedRoute, _ = resolvedProductRoute(source, routeConfig)
			if item.ProposedRoute == "" {
				return catalog.TaxonomyImpact{}, publishing.ErrSiteRouteInvalid
			}
			if err := ensureActiveRouteAvailable(ctx, tx, product, product.ID); err != nil {
				return catalog.TaxonomyImpact{}, err
			}
		}
		impact.AffectedProducts = append(impact.AffectedProducts, item)
	}
	return impact, nil
}

func requeueAffectedProductsTx(ctx context.Context, tx *sql.Tx, actorID, entityType, entityID, cause, now string) (catalog.TaxonomyImpact, error) {
	impact, err := taxonomyImpactTx(ctx, tx, entityType, entityID)
	if err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	for _, item := range impact.AffectedProducts {
		product, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, item.ProductID))
		if err != nil {
			return catalog.TaxonomyImpact{}, err
		}
		if err := product.Prepare(); err != nil {
			return catalog.TaxonomyImpact{}, err
		}
		newRevision := product.Revision + 1
		result, err := tx.ExecContext(ctx, `UPDATE products SET revision=?,updated_by=?,search_folded=?,search_projection_version=?,updated_at=?
			WHERE id=? AND revision=? AND record_state='current'`, newRevision, actorID, product.SearchFolded, product.ProjectionVer, now, product.ID, product.Revision)
		if err != nil {
			return catalog.TaxonomyImpact{}, err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.TaxonomyImpact{}, catalog.ErrRevisionConflict
		}
		if err := appendPublicationIntent(ctx, tx, product.ID, newRevision, cause, now); err != nil {
			return catalog.TaxonomyImpact{}, err
		}
	}
	return impact, nil
}

func applyCategoryUpdateTx(ctx context.Context, tx *sql.Tx, actorID, id string, expectedRevision int64, candidate catalog.Category) (catalog.Category, error) {
	if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
		return catalog.Category{}, err
	}
	current, err := scanCategory(tx.QueryRowContext(ctx, `SELECT id,COALESCE(parent_id,''),COALESCE(system_key,''),name,slug,status,revision
		FROM categories WHERE id=?`, id))
	if err != nil {
		return catalog.Category{}, err
	}
	if current.Revision != expectedRevision {
		return catalog.Category{}, catalog.ErrRevisionConflict
	}
	candidate.ID = current.ID
	candidate.SystemKey = current.SystemKey
	candidate.Status = current.Status
	candidate.Revision = current.Revision + 1
	if candidate.ParentID == "" {
		candidate.ParentID = current.ParentID
	}
	if current.SystemKey == "root" {
		return catalog.Category{}, catalog.ErrSystemCategory
	}
	if current.SystemKey == "uncategorized" && (candidate.ParentID != current.ParentID || candidate.Name != current.Name) {
		return catalog.Category{}, catalog.ErrSystemCategory
	}
	if err := candidate.Prepare(); err != nil {
		return catalog.Category{}, err
	}
	if err := requireActiveCategory(ctx, tx, candidate.ParentID); err != nil {
		return catalog.Category{}, err
	}
	if candidate.ID == candidate.ParentID {
		return catalog.Category{}, catalog.ErrCategoryCycle
	}
	var createsCycle int
	if err := tx.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (
		SELECT id FROM categories WHERE parent_id=?
		UNION ALL SELECT c.id FROM categories c JOIN descendants d ON c.parent_id=d.id
	) SELECT EXISTS(SELECT 1 FROM descendants WHERE id=?)`, candidate.ID, candidate.ParentID).Scan(&createsCycle); err != nil {
		return catalog.Category{}, err
	}
	if createsCycle != 0 {
		return catalog.Category{}, catalog.ErrCategoryCycle
	}
	if err := ensureCategorySlugAvailable(ctx, tx, candidate.ParentID, candidate.Slug, candidate.ID); err != nil {
		return catalog.Category{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE categories SET parent_id=?,name=?,slug=?,revision=?,updated_by=?,updated_at=?
		WHERE id=? AND revision=?`, candidate.ParentID, candidate.Name, candidate.Slug, candidate.Revision, actorID, now, candidate.ID, expectedRevision)
	if err != nil {
		return catalog.Category{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return catalog.Category{}, catalog.ErrRevisionConflict
	}
	return candidate, nil
}

func (s *Store) PreviewCategoryUpdate(ctx context.Context, actorID, id string, expectedRevision int64, candidate catalog.Category) (catalog.TaxonomyImpact, error) {
	tx, release, err := s.beginWrite(ctx)
	if err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	defer release()
	defer tx.Rollback()
	if _, err := applyCategoryUpdateTx(ctx, tx, actorID, id, expectedRevision, candidate); err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	return taxonomyImpactTx(ctx, tx, "category", id)
}

func (s *Store) UpdateCategory(ctx context.Context, actorID, id string, expectedRevision int64, candidate catalog.Category) (catalog.Category, catalog.TaxonomyImpact, error) {
	var updated catalog.Category
	var impact catalog.TaxonomyImpact
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		current, err := scanCategory(tx.QueryRowContext(ctx, `SELECT id,COALESCE(parent_id,''),COALESCE(system_key,''),name,slug,status,revision FROM categories WHERE id=?`, id))
		if err != nil {
			return err
		}
		updated, err = applyCategoryUpdateTx(ctx, tx, actorID, id, expectedRevision, candidate)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		impact, err = requeueAffectedProductsTx(ctx, tx, actorID, "category", id, "category.updated", now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "category.updated", "category", id, map[string]any{
			"from_name": current.Name, "to_name": updated.Name, "from_slug": current.Slug, "to_slug": updated.Slug,
			"from_parent_id": current.ParentID, "to_parent_id": updated.ParentID, "affected_products": len(impact.AffectedProducts),
		})
	})
	return updated, impact, err
}

func scanDictionaryEntry(scanner interface{ Scan(...any) error }) (catalog.DictionaryEntry, error) {
	var entry catalog.DictionaryEntry
	err := scanner.Scan(&entry.ID, &entry.Kind, &entry.Name, &entry.Slug, &entry.Status, &entry.Revision)
	return entry, err
}

func applyDictionaryUpdateTx(ctx context.Context, tx *sql.Tx, actorID, id string, expectedRevision int64, candidate catalog.DictionaryEntry) (catalog.DictionaryEntry, error) {
	if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
		return catalog.DictionaryEntry{}, err
	}
	current, err := scanDictionaryEntry(tx.QueryRowContext(ctx, `SELECT id,kind,name,slug,status,revision FROM dictionary_entries WHERE id=?`, id))
	if err != nil {
		return catalog.DictionaryEntry{}, err
	}
	if current.Revision != expectedRevision {
		return catalog.DictionaryEntry{}, catalog.ErrRevisionConflict
	}
	candidate.ID = current.ID
	candidate.Kind = current.Kind
	candidate.Status = current.Status
	candidate.Revision = current.Revision + 1
	if candidate.Slug == "" && routeBearingDictionary(candidate.Kind) {
		candidate.Slug = catalog.SuggestedSlug(candidate.Name, candidate.ID)
	}
	if err := candidate.Prepare(); err != nil {
		return catalog.DictionaryEntry{}, err
	}
	if candidate.Slug != "" && !catalog.ValidSlug(candidate.Slug) {
		return catalog.DictionaryEntry{}, catalog.ErrInvalidDictionary
	}
	if err := ensureDictionarySlugAvailable(ctx, tx, candidate.Kind, candidate.Slug, candidate.ID); err != nil {
		return catalog.DictionaryEntry{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE dictionary_entries SET name=?,slug=?,revision=?,updated_by=?,updated_at=? WHERE id=? AND revision=?`,
		candidate.Name, candidate.Slug, candidate.Revision, actorID, now, candidate.ID, expectedRevision)
	if err != nil {
		return catalog.DictionaryEntry{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return catalog.DictionaryEntry{}, catalog.ErrRevisionConflict
	}
	switch candidate.Kind {
	case catalog.DictionaryManufacturer:
		_, err = tx.ExecContext(ctx, `UPDATE products SET manufacturer=? WHERE manufacturer_id=?`, candidate.Name, candidate.ID)
	case catalog.DictionaryBrand:
		_, err = tx.ExecContext(ctx, `UPDATE products SET brand=? WHERE brand_id=?`, candidate.Name, candidate.ID)
	}
	if err != nil {
		return catalog.DictionaryEntry{}, err
	}
	return candidate, nil
}

func (s *Store) PreviewDictionaryUpdate(ctx context.Context, actorID, id string, expectedRevision int64, candidate catalog.DictionaryEntry) (catalog.TaxonomyImpact, error) {
	tx, release, err := s.beginWrite(ctx)
	if err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	defer release()
	defer tx.Rollback()
	updated, err := applyDictionaryUpdateTx(ctx, tx, actorID, id, expectedRevision, candidate)
	if err != nil {
		return catalog.TaxonomyImpact{}, err
	}
	return taxonomyImpactTx(ctx, tx, string(updated.Kind), id)
}

func (s *Store) UpdateDictionaryEntry(ctx context.Context, actorID, id string, expectedRevision int64, candidate catalog.DictionaryEntry) (catalog.DictionaryEntry, catalog.TaxonomyImpact, error) {
	var updated catalog.DictionaryEntry
	var impact catalog.TaxonomyImpact
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		current, err := scanDictionaryEntry(tx.QueryRowContext(ctx, `SELECT id,kind,name,slug,status,revision FROM dictionary_entries WHERE id=?`, id))
		if err != nil {
			return err
		}
		updated, err = applyDictionaryUpdateTx(ctx, tx, actorID, id, expectedRevision, candidate)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		impact, err = requeueAffectedProductsTx(ctx, tx, actorID, string(updated.Kind), id, "dictionary.updated", now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "dictionary.updated", string(updated.Kind), id, map[string]any{
			"from_name": current.Name, "to_name": updated.Name, "from_slug": current.Slug, "to_slug": updated.Slug,
			"affected_products": len(impact.AffectedProducts),
		})
	})
	return updated, impact, err
}

func ensureCategorySlugAvailable(ctx context.Context, tx *sql.Tx, parentID, slug, excludeID string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE COALESCE(parent_id,'')=? AND slug=? AND id<>?`, parentID, slug, excludeID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return catalog.ErrRouteConflict
	}
	return nil
}

func routeBearingDictionary(kind catalog.DictionaryKind) bool {
	return kind == catalog.DictionaryManufacturer || kind == catalog.DictionaryBrand || kind == catalog.DictionaryApplication
}

func ensureDictionarySlugAvailable(ctx context.Context, tx *sql.Tx, kind catalog.DictionaryKind, slug, excludeID string) error {
	if !routeBearingDictionary(kind) {
		return nil
	}
	if slug == "" || !catalog.ValidSlug(slug) {
		return catalog.ErrInvalidDictionary
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM dictionary_entries WHERE kind=? AND slug=? AND id<>?`, kind, slug, excludeID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return catalog.ErrRouteConflict
	}
	return nil
}
