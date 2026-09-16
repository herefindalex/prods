package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func (s *Store) CreateSpecDefinition(ctx context.Context, actorID string, spec catalog.SpecDefinition) (catalog.SpecDefinition, error) {
	if spec.ID == "" {
		id, err := randomID("spc")
		if err != nil {
			return catalog.SpecDefinition{}, err
		}
		spec.ID = id
	}
	if spec.Name == "" {
		return catalog.SpecDefinition{}, catalog.ErrInvalidSpec
	}
	if spec.Status == "" {
		spec.Status = catalog.EntryActive
	}
	if spec.Status != catalog.EntryActive && spec.Status != catalog.EntryDisabled {
		return catalog.SpecDefinition{}, catalog.ErrInvalidSpec
	}
	spec.Revision = 1
	if spec.SemanticVer < 1 {
		spec.SemanticVer = 1
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO spec_definitions(
			id,name,preferred_unit,filterable,semantic_version,status,revision,created_by,updated_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, spec.ID, spec.Name, spec.PreferredUnit, spec.Filterable, spec.SemanticVer,
			spec.Status, spec.Revision, actorID, actorID, now, now)
		if err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "spec.created", "spec", spec.ID, map[string]any{"name": spec.Name})
	})
	return spec, err
}

func (s *Store) ListSpecDefinitions(ctx context.Context) ([]catalog.SpecDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,preferred_unit,filterable,semantic_version,status,revision
		FROM spec_definitions ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var specs []catalog.SpecDefinition
	for rows.Next() {
		var spec catalog.SpecDefinition
		if err := rows.Scan(&spec.ID, &spec.Name, &spec.PreferredUnit, &spec.Filterable, &spec.SemanticVer, &spec.Status, &spec.Revision); err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, rows.Err()
}

func (s *Store) CreateSpecSet(ctx context.Context, actorID string, set catalog.SpecSet) (catalog.SpecSet, error) {
	if set.ID == "" {
		id, err := randomID("sps")
		if err != nil {
			return catalog.SpecSet{}, err
		}
		set.ID = id
	}
	if set.Name == "" {
		return catalog.SpecSet{}, catalog.ErrInvalidSpec
	}
	if set.Status == "" {
		set.Status = catalog.EntryActive
	}
	set.Revision = 1
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(set.SpecIDs))
		for _, specID := range set.SpecIDs {
			if specID == "" {
				return catalog.ErrInvalidSpec
			}
			if _, duplicate := seen[specID]; duplicate {
				return catalog.ErrInvalidSpec
			}
			seen[specID] = struct{}{}
			var active int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM spec_definitions WHERE id=? AND status='active'`, specID).Scan(&active); err != nil {
				return catalog.ErrInvalidSpec
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO spec_sets(
			id,name,status,revision,created_by,updated_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?)`, set.ID, set.Name, set.Status, set.Revision, actorID, actorID, now, now); err != nil {
			return err
		}
		for index, specID := range set.SpecIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO spec_set_members(spec_set_id,spec_id,sort_order) VALUES(?,?,?)`, set.ID, specID, index); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, actorID, "spec_set.created", "spec_set", set.ID, map[string]any{"spec_ids": set.SpecIDs})
	})
	return set, err
}

func (s *Store) ListSpecSets(ctx context.Context) ([]catalog.SpecSet, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,status,revision FROM spec_sets ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	var sets []catalog.SpecSet
	for rows.Next() {
		var set catalog.SpecSet
		if err := rows.Scan(&set.ID, &set.Name, &set.Status, &set.Revision); err != nil {
			rows.Close()
			return nil, err
		}
		sets = append(sets, set)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range sets {
		memberRows, err := s.db.QueryContext(ctx, `SELECT spec_id FROM spec_set_members WHERE spec_set_id=? ORDER BY sort_order,spec_id`, sets[index].ID)
		if err != nil {
			return nil, err
		}
		for memberRows.Next() {
			var specID string
			if err := memberRows.Scan(&specID); err != nil {
				memberRows.Close()
				return nil, err
			}
			sets[index].SpecIDs = append(sets[index].SpecIDs, specID)
		}
		if err := memberRows.Close(); err != nil {
			return nil, err
		}
		if err := memberRows.Err(); err != nil {
			return nil, err
		}
	}
	return sets, nil
}

func (s *Store) CategorySpecSet(ctx context.Context, categoryID string) (catalog.SpecSet, error) {
	var set catalog.SpecSet
	err := s.db.QueryRowContext(ctx, `SELECT ss.id,ss.name,ss.status,ss.revision
		FROM category_spec_sets cs JOIN spec_sets ss ON ss.id=cs.spec_set_id
		WHERE cs.category_id=?`, categoryID).Scan(&set.ID, &set.Name, &set.Status, &set.Revision)
	if err != nil {
		return catalog.SpecSet{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT spec_id FROM spec_set_members WHERE spec_set_id=? ORDER BY sort_order,spec_id`, set.ID)
	if err != nil {
		return catalog.SpecSet{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var specID string
		if err := rows.Scan(&specID); err != nil {
			return catalog.SpecSet{}, err
		}
		set.SpecIDs = append(set.SpecIDs, specID)
	}
	return set, rows.Err()
}

func (s *Store) SetCategorySpecSet(ctx context.Context, actorID, categoryID, specSetID string, expectedCategoryRevision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var categoryRevision int64
		var systemKey sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT revision,system_key FROM categories WHERE id=? AND status='active'`, categoryID).Scan(&categoryRevision, &systemKey); err != nil {
			return catalog.ErrInvalidCategory
		}
		if systemKey.Valid && systemKey.String == "uncategorized" {
			return catalog.ErrInvalidSpec
		}
		if categoryRevision != expectedCategoryRevision {
			return catalog.ErrRevisionConflict
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM spec_sets WHERE id=? AND status='active'`, specSetID).Scan(&active); err != nil {
			return catalog.ErrInvalidSpec
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO category_spec_sets(category_id,spec_set_id) VALUES(?,?)
			ON CONFLICT(category_id) DO UPDATE SET spec_set_id=excluded.spec_set_id`, categoryID, specSetID); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE categories SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`,
			actorID, now, categoryID, expectedCategoryRevision); err != nil {
			return err
		}
		if err := reconcileCategorySpecValues(ctx, tx, categoryID); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "category.spec_set_changed", "category", categoryID, map[string]any{
			"spec_set_id": specSetID, "revision": expectedCategoryRevision + 1,
		}); err != nil {
			return err
		}
		_, err := requeueAffectedProductsTx(ctx, tx, actorID, "category", categoryID, "category.spec_set_changed", now)
		return err
	})
}

func reconcileCategorySpecValues(ctx context.Context, tx *sql.Tx, categoryID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE product_spec_values AS pv SET active=EXISTS(
		SELECT 1 FROM products p
		JOIN category_spec_sets cs ON cs.category_id=p.category_id
		JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id AND sm.spec_id=pv.spec_id
		WHERE p.id=pv.product_id AND p.category_id=?
	) WHERE product_id IN (SELECT id FROM products WHERE category_id=?)`, categoryID, categoryID)
	return err
}

func reconcileProductSpecValues(ctx context.Context, tx *sql.Tx, productID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE product_spec_values AS pv SET active=EXISTS(
		SELECT 1 FROM products p
		JOIN category_spec_sets cs ON cs.category_id=p.category_id
		JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id AND sm.spec_id=pv.spec_id
		WHERE p.id=pv.product_id
	) WHERE product_id=?`, productID)
	return err
}

func (s *Store) SaveSpecValue(ctx context.Context, actorID, productID, specID, rawValue, sourceLocale string, expectedProductRevision int64) (catalog.SpecValue, error) {
	var value catalog.SpecValue
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var categoryID string
		var recordState catalog.RecordState
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT category_id,record_state,revision FROM products WHERE id=?`, productID).Scan(&categoryID, &recordState, &revision); err != nil {
			return err
		}
		if recordState == catalog.RecordArchived {
			return catalog.ErrArchivedProduct
		}
		if revision != expectedProductRevision {
			return catalog.ErrRevisionConflict
		}
		var specExists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM spec_definitions WHERE id=?`, specID).Scan(&specExists); err != nil {
			return catalog.ErrInvalidSpec
		}
		active := false
		var applicable int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM category_spec_sets cs JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id
			WHERE cs.category_id=? AND sm.spec_id=?)`, categoryID, specID).Scan(&applicable); err != nil {
			return err
		}
		active = applicable != 0
		var existingID string
		var sourceRevision int64
		err := tx.QueryRowContext(ctx, `SELECT id,source_revision FROM product_spec_values
			WHERE product_id=? AND spec_id=? AND source_locale=?`, productID, specID, sourceLocale).Scan(&existingID, &sourceRevision)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if errors.Is(err, sql.ErrNoRows) {
			existingID, err = randomID("spv")
			if err != nil {
				return err
			}
			sourceRevision = 1
			_, err = tx.ExecContext(ctx, `INSERT INTO product_spec_values(
				id,product_id,spec_id,raw_value,source_locale,source_revision,active,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, existingID, productID, specID, rawValue, sourceLocale, sourceRevision, active,
				actorID, actorID, now, now)
		} else if err == nil {
			sourceRevision++
			_, err = tx.ExecContext(ctx, `UPDATE product_spec_values SET raw_value=?,source_revision=?,active=?,updated_by=?,updated_at=?
				WHERE id=?`, rawValue, sourceRevision, active, actorID, now, existingID)
			if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE normalized_values SET status='stale'
					WHERE spec_value_id=? AND status='current'`, existingID)
			}
		}
		if err != nil {
			return err
		}
		newProductRevision := expectedProductRevision + 1
		result, err := tx.ExecContext(ctx, `UPDATE products SET revision=?,updated_by=?,updated_at=? WHERE id=? AND revision=?`,
			newProductRevision, actorID, now, productID, expectedProductRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		value = catalog.SpecValue{ID: existingID, ProductID: productID, SpecID: specID, RawValue: rawValue,
			SourceLocale: sourceLocale, SourceRevision: sourceRevision, Active: active}
		if err := appendAudit(ctx, tx, actorID, "product.spec_saved", "product", productID, map[string]any{
			"spec_id": specID, "source_revision": sourceRevision, "active": active,
		}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, productID, newProductRevision, "product.spec_saved", now)
	})
	return value, err
}

func (s *Store) ProductSpecValues(ctx context.Context, productID string) ([]catalog.SpecValueDetail, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM products WHERE id=?`, productID).Scan(&exists); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,product_id,spec_id,raw_value,source_locale,source_revision,active
		FROM product_spec_values WHERE product_id=? ORDER BY spec_id,source_locale,id`, productID)
	if err != nil {
		return nil, err
	}
	var details []catalog.SpecValueDetail
	for rows.Next() {
		var detail catalog.SpecValueDetail
		if err := rows.Scan(&detail.Value.ID, &detail.Value.ProductID, &detail.Value.SpecID, &detail.Value.RawValue,
			&detail.Value.SourceLocale, &detail.Value.SourceRevision, &detail.Value.Active); err != nil {
			rows.Close()
			return nil, err
		}
		details = append(details, detail)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range details {
		normalizedRows, err := s.db.QueryContext(ctx, `SELECT id,spec_value_id,value_json,source_revision,spec_semantic_version,
			normalizer_version,source,status,failure_reason
			FROM normalized_values WHERE spec_value_id=? ORDER BY created_at DESC,id DESC`, details[index].Value.ID)
		if err != nil {
			return nil, err
		}
		for normalizedRows.Next() {
			var normalized catalog.NormalizedValue
			if err := normalizedRows.Scan(&normalized.ID, &normalized.SpecValueID, &normalized.ValueJSON,
				&normalized.SourceRevision, &normalized.SpecSemanticVersion, &normalized.NormalizerVersion,
				&normalized.Source, &normalized.Status, &normalized.FailureReason); err != nil {
				normalizedRows.Close()
				return nil, err
			}
			details[index].Normalized = append(details[index].Normalized, normalized)
		}
		if err := normalizedRows.Close(); err != nil {
			return nil, err
		}
		if err := normalizedRows.Err(); err != nil {
			return nil, err
		}
	}
	return details, nil
}

func (s *Store) SaveNormalizedValue(ctx context.Context, actorID string, normalized catalog.NormalizedValue) (catalog.NormalizedValue, error) {
	if normalized.SpecValueID == "" || (normalized.Source != catalog.NormalizedAutomatic && normalized.Source != catalog.NormalizedManual) ||
		(normalized.Status != catalog.NormalizedCurrent && normalized.Status != catalog.NormalizedFailed) || normalized.NormalizerVersion == "" {
		return catalog.NormalizedValue{}, catalog.ErrInvalidSpec
	}
	if normalized.ID == "" {
		id, err := randomID("nrm")
		if err != nil {
			return catalog.NormalizedValue{}, err
		}
		normalized.ID = id
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var sourceRevision, semanticVersion int64
		if err := tx.QueryRowContext(ctx, `SELECT pv.source_revision,sd.semantic_version
			FROM product_spec_values pv JOIN spec_definitions sd ON sd.id=pv.spec_id WHERE pv.id=?`,
			normalized.SpecValueID).Scan(&sourceRevision, &semanticVersion); err != nil {
			return catalog.ErrInvalidSpec
		}
		if normalized.SourceRevision != sourceRevision || normalized.SpecSemanticVersion != semanticVersion {
			return catalog.ErrRevisionConflict
		}
		if normalized.Source == catalog.NormalizedAutomatic {
			var manual int
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM normalized_values
				WHERE spec_value_id=? AND source='manual' AND status='current')`, normalized.SpecValueID).Scan(&manual); err != nil {
				return err
			}
			if manual != 0 {
				return catalog.ErrManualNormalization
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE normalized_values SET status='stale'
			WHERE spec_value_id=? AND status='current'`, normalized.SpecValueID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO normalized_values(
			id,spec_value_id,value_json,source_revision,spec_semantic_version,normalizer_version,source,status,failure_reason,created_by,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, normalized.ID, normalized.SpecValueID, normalized.ValueJSON,
			normalized.SourceRevision, normalized.SpecSemanticVersion, normalized.NormalizerVersion,
			normalized.Source, normalized.Status, normalized.FailureReason, actorID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	return normalized, err
}

func (s *Store) AddProductDocument(ctx context.Context, actorID string, expectedProductRevision int64, document catalog.ProductDocument) (catalog.ProductDocument, error) {
	if document.ID == "" {
		id, err := randomID("doc")
		if err != nil {
			return catalog.ProductDocument{}, err
		}
		document.ID = id
	}
	if err := document.Prepare(); err != nil {
		return catalog.ProductDocument{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var recordState catalog.RecordState
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT record_state,revision FROM products WHERE id=?`, document.ProductID).Scan(&recordState, &revision); err != nil {
			return err
		}
		if recordState == catalog.RecordArchived {
			return catalog.ErrArchivedProduct
		}
		if revision != expectedProductRevision {
			return catalog.ErrRevisionConflict
		}
		if _, err := activeDictionaryName(ctx, tx, document.DocumentTypeID, catalog.DictionaryDocumentType); err != nil {
			return err
		}
		if document.AssetID != "" {
			var mime string
			if err := tx.QueryRowContext(ctx, `SELECT mime_type FROM assets WHERE id=?`, document.AssetID).Scan(&mime); err != nil || mime != "application/pdf" {
				return catalog.ErrInvalidDocument
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO product_documents(
			id,product_id,label,document_type_id,asset_id,external_url,language,sort_order,created_by,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, document.ID, document.ProductID, document.Label, document.DocumentTypeID,
			nullable(document.AssetID), nullable(document.ExternalURL), document.Language, document.SortOrder, actorID, now)
		if err != nil {
			return err
		}
		newRevision := expectedProductRevision + 1
		result, err := tx.ExecContext(ctx, `UPDATE products SET revision=?,updated_by=?,updated_at=? WHERE id=? AND revision=?`,
			newRevision, actorID, now, document.ProductID, expectedProductRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if err := appendAudit(ctx, tx, actorID, "product.document_added", "product", document.ProductID, map[string]any{
			"document_id": document.ID, "document_type_id": document.DocumentTypeID,
		}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, document.ProductID, newRevision, "product.document_added", now)
	})
	return document, err
}

func (s *Store) ProductDocuments(ctx context.Context, productID string) ([]catalog.ProductDocument, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,product_id,label,document_type_id,COALESCE(asset_id,''),COALESCE(external_url,''),language,sort_order
		FROM product_documents WHERE product_id=? ORDER BY sort_order,id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var documents []catalog.ProductDocument
	for rows.Next() {
		var document catalog.ProductDocument
		if err := rows.Scan(&document.ID, &document.ProductID, &document.Label, &document.DocumentTypeID,
			&document.AssetID, &document.ExternalURL, &document.Language, &document.SortOrder); err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}
