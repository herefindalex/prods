package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
)

type AuditEntry struct {
	ID         string          `json:"id"`
	ActorID    string          `json:"actor_id,omitempty"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   string          `json:"target_id"`
	Result     string          `json:"result"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  time.Time       `json:"created_at"`
}

var ErrPermissionDenied = errors.New("permission denied")

func (s *Store) CreateProduct(ctx context.Context, actorID string, product catalog.Product) (catalog.Product, error) {
	if product.ID == "" {
		id, err := randomID("prd")
		if err != nil {
			return catalog.Product{}, err
		}
		product.ID = id
	}
	product.Revision = 1
	product.CreatedBy = actorID
	product.UpdatedBy = actorID
	if err := product.Prepare(); err != nil {
		return catalog.Product{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if product.Status == catalog.Published {
			if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
				return err
			}
		}
		if err := validateProductReferences(ctx, tx, &product); err != nil {
			return err
		}
		if err := product.Prepare(); err != nil {
			return err
		}
		if err := ensureCurrentIdentityAvailable(ctx, tx, product, ""); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := insertProductTx(ctx, tx, product, now); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "product.created", "product", product.ID, map[string]any{
			"revision":         product.Revision,
			"record_state":     product.RecordState,
			"publishing_state": product.Status,
		}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, product.ID, product.Revision, "product.created", now)
	})
	return product, err
}

func (s *Store) Product(ctx context.Context, id string) (catalog.Product, error) {
	return scanProduct(s.db.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, id))
}

func (s *Store) ListProducts(ctx context.Context, includeArchived bool, limit int) ([]catalog.Product, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	where := "record_state='current'"
	if includeArchived {
		where = "1=1"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+productColumns+` FROM products WHERE `+where+`
		ORDER BY updated_at DESC,id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []catalog.Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) UpdateProduct(ctx context.Context, actorID string, expectedRevision int64, product catalog.Product) (catalog.Product, error) {
	if expectedRevision < 1 || product.ID == "" {
		return catalog.Product{}, catalog.ErrInvalidProduct
	}
	var updated catalog.Product
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		current, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, product.ID))
		if err != nil {
			return err
		}
		if current.RecordState == catalog.RecordArchived {
			return catalog.ErrArchivedProduct
		}
		if current.Revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		if product.Status == "" {
			product.Status = current.Status
		}
		product.Slug = current.Slug
		product.CustomPath = current.CustomPath
		if product.Status != current.Status {
			if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
				return err
			}
		}
		product.RecordState = catalog.RecordCurrent
		product.Revision = expectedRevision + 1
		product.CreatedBy = current.CreatedBy
		product.UpdatedBy = actorID
		if product.ClonedFromID == "" {
			product.ClonedFromID = current.ClonedFromID
		}
		if err := product.Prepare(); err != nil {
			return err
		}
		if err := validateProductReferences(ctx, tx, &product); err != nil {
			return err
		}
		if err := product.Prepare(); err != nil {
			return err
		}
		if err := ensureCurrentIdentityAvailable(ctx, tx, product, product.ID); err != nil {
			return err
		}
		if err := ensureActiveRouteAvailable(ctx, tx, product, product.ID); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE products SET
			category_id=?,part_number=?,identity_part_number=?,name=?,manufacturer_id=?,manufacturer=?,identity_manufacturer=?,
			brand_id=?,brand=?,lifecycle_id=?,package_form_factor=?,description=?,features=?,specification=?,document_url=?,status=?,
			revision=?,updated_by=?,search_folded=?,search_projection_version=?,updated_at=?
			WHERE id=? AND revision=? AND record_state='current'`,
			product.CategoryID, product.PartNumber, product.IdentityPart, product.Name, nullable(product.ManufacturerID),
			product.Manufacturer, product.IdentityMaker, nullable(product.BrandID), product.Brand, nullable(product.LifecycleID), product.PackageFormFactor,
			product.Description, product.Features, product.Specification, product.DocumentURL, product.Status,
			product.Revision, nullable(product.UpdatedBy), product.SearchFolded, product.ProjectionVer, now,
			product.ID, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, `UPDATE product_url_names SET slug=?,custom_path=?,revision=revision+1,updated_at=? WHERE product_id=?`, product.Slug, product.CustomPath, now, product.ID); err != nil {
			return err
		}
		if err := replaceProductApplications(ctx, tx, product.ID, product.ApplicationIDs); err != nil {
			return err
		}
		if current.CategoryID != product.CategoryID {
			if err := reconcileProductSpecValues(ctx, tx, product.ID); err != nil {
				return err
			}
		}
		if err := appendAudit(ctx, tx, actorID, "product.updated", "product", product.ID, map[string]any{
			"from_revision":           current.Revision,
			"to_revision":             product.Revision,
			"publishing_state_before": current.Status,
			"publishing_state_after":  product.Status,
		}); err != nil {
			return err
		}
		if err := appendPublicationIntent(ctx, tx, product.ID, product.Revision, "product.updated", now); err != nil {
			return err
		}
		updated = product
		return nil
	})
	return updated, err
}

func (s *Store) UpdateProductURL(ctx context.Context, actorID, productID string, expectedRevision int64, slug, customPath string) (catalog.Product, error) {
	var updated catalog.Product
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		current, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, productID))
		if err != nil {
			return err
		}
		if current.RecordState != catalog.RecordCurrent {
			return catalog.ErrArchivedProduct
		}
		if current.Revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		if current.Status == catalog.Published {
			if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
				return err
			}
		}
		candidate := current
		candidate.Slug = slug
		candidate.CustomPath = customPath
		candidate.Revision++
		candidate.UpdatedBy = actorID
		if err := candidate.Prepare(); err != nil {
			return err
		}
		if err := ensureActiveRouteAvailable(ctx, tx, candidate, candidate.ID); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE product_url_names SET slug=?,custom_path=?,revision=revision+1,updated_at=? WHERE product_id=?`, candidate.Slug, candidate.CustomPath, now, candidate.ID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE products SET revision=?,updated_by=?,updated_at=? WHERE id=? AND revision=? AND record_state='current'`, candidate.Revision, actorID, now, candidate.ID, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if err := appendAudit(ctx, tx, actorID, "product.url_updated", "product", candidate.ID, map[string]any{
			"from_slug": current.Slug, "to_slug": candidate.Slug, "from_custom_path": current.CustomPath, "to_custom_path": candidate.CustomPath,
		}); err != nil {
			return err
		}
		if err := appendPublicationIntent(ctx, tx, candidate.ID, candidate.Revision, "product.url_updated", now); err != nil {
			return err
		}
		updated, err = scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, candidate.ID))
		return err
	})
	return updated, err
}

func (s *Store) ArchiveProduct(ctx context.Context, actorID, id string, expectedRevision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogPublish); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE products SET record_state='archived',status='hidden',revision=revision+1,
			updated_by=?,updated_at=? WHERE id=? AND revision=? AND record_state='current'`,
			actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			var recordState catalog.RecordState
			var revision int64
			if scanErr := tx.QueryRowContext(ctx, `SELECT record_state,revision FROM products WHERE id=?`, id).Scan(&recordState, &revision); scanErr != nil {
				return scanErr
			}
			if recordState == catalog.RecordArchived {
				return catalog.ErrArchivedProduct
			}
			return catalog.ErrRevisionConflict
		}
		newRevision := expectedRevision + 1
		if err := appendAudit(ctx, tx, actorID, "product.archived", "product", id, map[string]any{
			"from_revision": expectedRevision,
			"to_revision":   newRevision,
		}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, id, newRevision, "product.archived", now)
	})
}

func (s *Store) CloneProduct(ctx context.Context, actorID, sourceID string, expectedSourceRevision int64, clone catalog.Product) (catalog.Product, error) {
	var created catalog.Product
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		source, err := scanProduct(tx.QueryRowContext(ctx, `SELECT `+productColumns+` FROM products WHERE id=?`, sourceID))
		if err != nil {
			return err
		}
		if source.Revision != expectedSourceRevision {
			return catalog.ErrRevisionConflict
		}
		if clone.ID == "" {
			clone.ID, err = randomID("prd")
			if err != nil {
				return err
			}
		}
		if clone.PartNumber == "" {
			clone.PartNumber = source.PartNumber
		}
		if clone.Name == "" {
			clone.Name = source.Name
		}
		if clone.ManufacturerID == "" && clone.Manufacturer == "" {
			clone.ManufacturerID = source.ManufacturerID
			clone.Manufacturer = source.Manufacturer
		}
		if clone.BrandID == "" && clone.Brand == "" {
			clone.BrandID = source.BrandID
			clone.Brand = source.Brand
		}
		if clone.LifecycleID == "" {
			clone.LifecycleID = source.LifecycleID
		}
		if len(clone.ApplicationIDs) == 0 {
			clone.ApplicationIDs = append([]string(nil), source.ApplicationIDs...)
		}
		if clone.CategoryID == "" {
			clone.CategoryID = source.CategoryID
		}
		if clone.PackageFormFactor == "" {
			clone.PackageFormFactor = source.PackageFormFactor
		}
		if clone.Description == "" {
			clone.Description = source.Description
		}
		if clone.Features == "" {
			clone.Features = source.Features
		}
		if clone.Specification == "" {
			clone.Specification = source.Specification
		}
		if clone.DocumentURL == "" {
			clone.DocumentURL = source.DocumentURL
		}
		clone.RecordState = catalog.RecordCurrent
		clone.Status = catalog.Hidden
		clone.Revision = 1
		clone.ClonedFromID = source.ID
		clone.CreatedBy = actorID
		clone.UpdatedBy = actorID
		if err := clone.Prepare(); err != nil {
			return err
		}
		if err := validateProductReferences(ctx, tx, &clone); err != nil {
			return err
		}
		if err := clone.Prepare(); err != nil {
			return err
		}
		if err := ensureCurrentIdentityAvailable(ctx, tx, clone, ""); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := insertProductTx(ctx, tx, clone, now); err != nil {
			return err
		}
		if err := cloneSpecValues(ctx, tx, source.ID, clone.ID, clone.CategoryID, actorID, now); err != nil {
			return err
		}
		if err := cloneDocuments(ctx, tx, source.ID, clone.ID, actorID, now); err != nil {
			return err
		}
		if err := cloneProductImages(ctx, tx, source.ID, clone.ID, actorID, now); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "product.cloned", "product", clone.ID, map[string]any{
			"cloned_from_id": source.ID, "source_revision": source.Revision,
		}); err != nil {
			return err
		}
		if err := appendPublicationIntent(ctx, tx, clone.ID, clone.Revision, "product.cloned", now); err != nil {
			return err
		}
		created = clone
		return nil
	})
	return created, err
}

func cloneSpecValues(ctx context.Context, tx *sql.Tx, sourceID, targetID, categoryID, actorID, now string) error {
	rows, err := tx.QueryContext(ctx, `SELECT spec_id,raw_value,source_locale FROM product_spec_values WHERE product_id=?`, sourceID)
	if err != nil {
		return err
	}
	type sourceValue struct{ specID, rawValue, sourceLocale string }
	var values []sourceValue
	for rows.Next() {
		var value sourceValue
		if err := rows.Scan(&value.specID, &value.rawValue, &value.sourceLocale); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range values {
		var applicable int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM category_spec_sets cs JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id
			WHERE cs.category_id=? AND sm.spec_id=?)`, categoryID, value.specID).Scan(&applicable); err != nil {
			return err
		}
		id, err := randomID("spv")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_spec_values(
			id,product_id,spec_id,raw_value,source_locale,source_revision,active,created_by,updated_by,created_at,updated_at
		) VALUES(?,?,?,?,?,1,?,?,?,?,?)`, id, targetID, value.specID, value.rawValue, value.sourceLocale,
			applicable != 0, actorID, actorID, now, now); err != nil {
			return err
		}
	}
	return nil
}

func cloneDocuments(ctx context.Context, tx *sql.Tx, sourceID, targetID, actorID, now string) error {
	rows, err := tx.QueryContext(ctx, `SELECT label,document_type_id,asset_id,external_url,language,sort_order
		FROM product_documents WHERE product_id=? ORDER BY sort_order,id`, sourceID)
	if err != nil {
		return err
	}
	type sourceDocument struct {
		label, documentTypeID, language string
		assetID, externalURL            sql.NullString
		sortOrder                       int
	}
	var documents []sourceDocument
	for rows.Next() {
		var document sourceDocument
		if err := rows.Scan(&document.label, &document.documentTypeID, &document.assetID, &document.externalURL,
			&document.language, &document.sortOrder); err != nil {
			rows.Close()
			return err
		}
		documents = append(documents, document)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, document := range documents {
		id, err := randomID("doc")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_documents(
			id,product_id,label,document_type_id,asset_id,external_url,language,sort_order,created_by,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, targetID, document.label, document.documentTypeID,
			document.assetID, document.externalURL, document.language, document.sortOrder, actorID, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,COALESCE(actor_id,''),action,target_type,target_id,result,details_json,created_at
		FROM admin_log ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		var createdAt, details string
		if err := rows.Scan(&entry.ID, &entry.ActorID, &entry.Action, &entry.TargetType, &entry.TargetID,
			&entry.Result, &details, &createdAt); err != nil {
			return nil, err
		}
		entry.Details = json.RawMessage(details)
		entry.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func requireActorCapability(ctx context.Context, tx *sql.Tx, actorID string, capability identity.Capability) error {
	var allowed int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM users u
		JOIN roles r ON r.id=u.role
		JOIN role_capabilities rc ON rc.role_id=r.id
		WHERE u.id=? AND u.status='active' AND r.status='active' AND rc.capability=?`, actorID, capability).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPermissionDenied
	}
	return err
}

func validateProductReferences(ctx context.Context, tx *sql.Tx, product *catalog.Product) error {
	var systemKey sql.NullString
	var categoryStatus catalog.EntryStatus
	if err := tx.QueryRowContext(ctx, `SELECT system_key,status FROM categories WHERE id=?`, product.CategoryID).Scan(&systemKey, &categoryStatus); err != nil {
		return catalog.ErrInvalidProduct
	}
	if categoryStatus != catalog.EntryActive || (systemKey.Valid && systemKey.String == "root") {
		return catalog.ErrInvalidProduct
	}
	if product.ManufacturerID != "" {
		name, err := activeDictionaryName(ctx, tx, product.ManufacturerID, catalog.DictionaryManufacturer)
		if err != nil {
			return err
		}
		product.Manufacturer = name
	}
	if product.BrandID != "" {
		name, err := activeDictionaryName(ctx, tx, product.BrandID, catalog.DictionaryBrand)
		if err != nil {
			return err
		}
		product.Brand = name
	}
	if product.LifecycleID != "" {
		name, err := activeDictionaryName(ctx, tx, product.LifecycleID, catalog.DictionaryLifecycle)
		if err != nil {
			return err
		}
		product.Lifecycle = name
	} else {
		product.Lifecycle = ""
	}
	product.Applications = product.Applications[:0]
	for _, applicationID := range product.ApplicationIDs {
		var application catalog.Application
		if err := tx.QueryRowContext(ctx, `SELECT id,name,slug FROM dictionary_entries WHERE id=? AND kind='application' AND status='active'`, applicationID).
			Scan(&application.ID, &application.Name, &application.Slug); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return catalog.ErrDisabledReference
			}
			return err
		}
		product.Applications = append(product.Applications, application)
	}
	return nil
}

func replaceProductApplications(ctx context.Context, tx *sql.Tx, productID string, applicationIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_applications WHERE product_id=?`, productID); err != nil {
		return err
	}
	for index, applicationID := range applicationIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_applications(product_id,application_id,sort_order) VALUES(?,?,?)`, productID, applicationID, index); err != nil {
			return err
		}
	}
	return nil
}

func activeDictionaryName(ctx context.Context, tx *sql.Tx, id string, kind catalog.DictionaryKind) (string, error) {
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM dictionary_entries WHERE id=? AND kind=? AND status='active'`, id, kind).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", catalog.ErrDisabledReference
	}
	return name, err
}

func insertProductTx(ctx context.Context, tx *sql.Tx, product catalog.Product, now string) error {
	if err := ensureActiveRouteAvailable(ctx, tx, product, product.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO products (
		id, category_id, part_number, identity_part_number, name,
		manufacturer_id, manufacturer, identity_manufacturer, brand_id, brand, lifecycle_id,
		package_form_factor, description, features, specification, document_url,
		status, record_state, revision, cloned_from_id, created_by, updated_by,
		search_folded, search_projection_version, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		product.ID, product.CategoryID, product.PartNumber, product.IdentityPart, product.Name,
		nullable(product.ManufacturerID), product.Manufacturer, product.IdentityMaker, nullable(product.BrandID), product.Brand, nullable(product.LifecycleID),
		product.PackageFormFactor, product.Description, product.Features, product.Specification, product.DocumentURL,
		product.Status, product.RecordState, product.Revision, nullable(product.ClonedFromID), nullable(product.CreatedBy), nullable(product.UpdatedBy),
		product.SearchFolded, product.ProjectionVer, now, now)
	if err != nil {
		return fmt.Errorf("insert product: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO product_url_names(product_id,slug,custom_path,revision,updated_at) VALUES(?,?,?,1,?)`, product.ID, product.Slug, product.CustomPath, now); err != nil {
		return fmt.Errorf("insert product URL name: %w", err)
	}
	return replaceProductApplications(ctx, tx, product.ID, product.ApplicationIDs)
}

func ensureActiveRouteAvailable(ctx context.Context, tx *sql.Tx, product catalog.Product, excludeProductID string) error {
	if product.Status != catalog.Published {
		return nil
	}
	var routeConfig publishing.SiteRouteConfig
	if err := tx.QueryRowContext(ctx, `SELECT product_prefix,url_pattern FROM public_site_state WHERE singleton=1`).Scan(&routeConfig.ProductPrefix, &routeConfig.Pattern); err != nil {
		return err
	}
	source := publishing.Source{Product: product}
	var err error
	source.Category, source.CategoryPath, err = publicCategoryIdentityQuery(ctx, tx, product.CategoryID)
	if err != nil {
		return err
	}
	if product.ManufacturerID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.ManufacturerID).Scan(&source.ManufacturerSlug); err != nil {
			return err
		}
	}
	if product.BrandID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT slug FROM dictionary_entries WHERE id=?`, product.BrandID).Scan(&source.BrandSlug); err != nil {
			return err
		}
	}
	candidateRoute, reason := resolvedProductRoute(source, routeConfig)
	if reason != "" {
		return publishing.ErrSiteRouteInvalid
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM public_activations WHERE route=? AND product_id<>?`, candidateRoute, excludeProductID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return catalog.ErrRouteConflict
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_url_names n JOIN products p ON p.id=n.product_id WHERE p.record_state='current' AND p.status='published' AND n.custom_path=? AND n.product_id<>?`, candidateRoute, excludeProductID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return catalog.ErrRouteConflict
	}
	if product.CustomPath != "" {
		return nil
	}
	switch routeConfig.Pattern {
	case publishing.RouteCompact:
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_url_names n JOIN products p ON p.id=n.product_id WHERE p.record_state='current' AND p.status='published' AND n.slug=? AND n.product_id<>?`, product.Slug, excludeProductID).Scan(&count)
	case publishing.RouteManufacturer:
		if product.ManufacturerID == "" {
			return publishing.ErrSiteRouteInvalid
		}
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_url_names n JOIN products p ON p.id=n.product_id WHERE p.record_state='current' AND p.status='published' AND p.manufacturer_id=? AND n.slug=? AND n.product_id<>?`, product.ManufacturerID, product.Slug, excludeProductID).Scan(&count)
	case publishing.RouteBrand:
		if product.BrandID == "" {
			return publishing.ErrSiteRouteInvalid
		}
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_url_names n JOIN products p ON p.id=n.product_id WHERE p.record_state='current' AND p.status='published' AND p.brand_id=? AND n.slug=? AND n.product_id<>?`, product.BrandID, product.Slug, excludeProductID).Scan(&count)
	case publishing.RouteCategory:
		if product.CategoryID == "" {
			return publishing.ErrSiteRouteInvalid
		}
		err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_url_names n JOIN products p ON p.id=n.product_id WHERE p.record_state='current' AND p.status='published' AND p.category_id=? AND n.slug=? AND n.product_id<>?`, product.CategoryID, product.Slug, excludeProductID).Scan(&count)
	default:
		return publishing.ErrSiteRouteInvalid
	}
	if err != nil {
		return err
	}
	if count != 0 {
		return catalog.ErrRouteConflict
	}
	return nil
}

func appendAudit(ctx context.Context, tx *sql.Tx, actorID, action, targetType, targetID string, details any) error {
	id, err := randomID("log")
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admin_log(
		id,actor_id,action,target_type,target_id,result,details_json,created_at
	) VALUES(?,?,?,?,?,'success',?,?)`, id, nullable(actorID), action, targetType, targetID,
		string(encoded), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func appendPublicationIntent(ctx context.Context, tx *sql.Tx, productID string, revision int64, cause, now string) error {
	return appendPublicChange(ctx, tx, "product", productID, revision, cause, now)
}

func appendPublicChange(ctx context.Context, tx *sql.Tx, entityType, entityID string, revision int64, cause, now string) error {
	id, err := randomID("out")
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO publication_intents(
		id,entity_type,entity_id,desired_revision,cause,status,created_at,updated_at
	) VALUES(?,?,?,?,?,'pending',?,?)`, id, entityType, entityID, revision, cause, now, now)
	return err
}
