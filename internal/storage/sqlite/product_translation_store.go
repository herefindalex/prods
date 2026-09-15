package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/localization"
)

func (s *Store) SaveProductSourceLocales(ctx context.Context, actorID, productID string, expectedProductRevision int64, sourceLocale string, sourceLocales map[string]string) (catalog.ProductContent, error) {
	if actorID == "" || productID == "" || expectedProductRevision < 1 {
		return catalog.ProductContent{}, catalog.ErrInvalidProduct
	}
	normalizedLocale, ok := localization.NormalizeBuiltinLocale(sourceLocale)
	if !ok {
		return catalog.ProductContent{}, catalog.ErrInvalidContentLocale
	}
	normalizedFields, err := catalog.NormalizeFieldSourceLocales(sourceLocales, normalizedLocale, catalog.ProductTranslatableFields)
	if err != nil {
		return catalog.ProductContent{}, err
	}
	encoded, err := json.Marshal(normalizedFields)
	if err != nil {
		return catalog.ProductContent{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var revision int64
		var state catalog.RecordState
		if err := tx.QueryRowContext(ctx, `SELECT revision,record_state FROM products WHERE id=?`, productID).Scan(&revision, &state); err != nil {
			return err
		}
		if state != catalog.RecordCurrent {
			return catalog.ErrArchivedProduct
		}
		if revision != expectedProductRevision {
			return catalog.ErrRevisionConflict
		}
		var enabled bool
		if err := tx.QueryRowContext(ctx, `SELECT content_editing_enabled FROM website_working WHERE singleton=1`).Scan(&enabled); err != nil {
			return err
		}
		if !enabled {
			return ErrContentLocaleUnavailable
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE product_content_metadata SET source_locale=?,source_locales_json=? WHERE product_id=?`, normalizedLocale, string(encoded), productID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE products SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`, actorID, now, productID, expectedProductRevision)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if err := appendAudit(ctx, tx, actorID, "product.source_locales_saved", "product", productID, map[string]any{"source_locale": normalizedLocale, "source_locales": normalizedFields, "revision": expectedProductRevision + 1}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, productID, expectedProductRevision+1, "product.source_locales_saved", now)
	})
	if err != nil {
		return catalog.ProductContent{}, err
	}
	return s.ProductContent(ctx, productID)
}

var ErrContentLocaleUnavailable = errors.New("content locale unavailable")

func (s *Store) ProductContent(ctx context.Context, productID string) (catalog.ProductContent, error) {
	content := catalog.ProductContent{ProductID: productID}
	var sourceLocalesJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT m.source_locale,m.source_locales_json,p.revision
		FROM product_content_metadata m JOIN products p ON p.id=m.product_id WHERE m.product_id=?`, productID).Scan(
		&content.SourceLocale, &sourceLocalesJSON, &content.ProductRevision,
	); err != nil {
		return content, err
	}
	if err := json.Unmarshal([]byte(sourceLocalesJSON), &content.SourceLocales); err != nil {
		return content, err
	}
	if len(content.SourceLocales) == 0 {
		content.SourceLocales, _ = catalog.NormalizeFieldSourceLocales(nil, content.SourceLocale, catalog.ProductTranslatableFields)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT locale,name,description,features,specification,revision,COALESCE(updated_by,''),updated_at
		FROM product_translations WHERE product_id=? ORDER BY locale`, productID)
	if err != nil {
		return content, err
	}
	defer rows.Close()
	for rows.Next() {
		var item catalog.ProductTranslation
		if err := rows.Scan(&item.Locale, &item.Name, &item.Description, &item.Features, &item.Specification, &item.Revision, &item.UpdatedBy, &item.UpdatedAt); err != nil {
			return content, err
		}
		content.Translations = append(content.Translations, item)
	}
	return content, rows.Err()
}

func (s *Store) SaveProductTranslation(ctx context.Context, actorID, productID string, expectedProductRevision int64, item catalog.ProductTranslation) (catalog.ProductContent, error) {
	item.Locale = strings.TrimSpace(item.Locale)
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.Features = strings.TrimSpace(item.Features)
	item.Specification = strings.TrimSpace(item.Specification)
	if actorID == "" || productID == "" || expectedProductRevision < 1 || item.Locale == "" {
		return catalog.ProductContent{}, catalog.ErrInvalidProduct
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var revision int64
		var state catalog.RecordState
		if err := tx.QueryRowContext(ctx, `SELECT revision,record_state FROM products WHERE id=?`, productID).Scan(&revision, &state); err != nil {
			return err
		}
		if state != catalog.RecordCurrent {
			return catalog.ErrArchivedProduct
		}
		if revision != expectedProductRevision {
			return catalog.ErrRevisionConflict
		}
		var enabled bool
		if err := tx.QueryRowContext(ctx, `SELECT w.content_editing_enabled
			FROM product_content_metadata m CROSS JOIN website_working w WHERE m.product_id=? AND w.singleton=1`, productID).Scan(&enabled); err != nil {
			return err
		}
		if _, supported := localization.NormalizeBuiltinLocale(item.Locale); !enabled || !supported {
			return ErrContentLocaleUnavailable
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO product_translations(product_id,locale,name,description,features,specification,revision,updated_by,updated_at)
			VALUES(?,?,?,?,?,?,1,?,?) ON CONFLICT(product_id,locale) DO UPDATE SET name=excluded.name,description=excluded.description,
			features=excluded.features,specification=excluded.specification,revision=product_translations.revision+1,updated_by=excluded.updated_by,updated_at=excluded.updated_at`,
			productID, item.Locale, item.Name, item.Description, item.Features, item.Specification, actorID, now)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE products SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`, actorID, now, productID, expectedProductRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if err := appendAudit(ctx, tx, actorID, "product.translation_saved", "product", productID, map[string]any{"locale": item.Locale, "revision": expectedProductRevision + 1}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, productID, expectedProductRevision+1, "product.translation_saved", now)
	})
	if err != nil {
		return catalog.ProductContent{}, err
	}
	return s.ProductContent(ctx, productID)
}

func localeInJSON(encoded, locale string) bool {
	var locales []string
	if json.Unmarshal([]byte(encoded), &locales) != nil {
		return false
	}
	for _, candidate := range locales {
		if candidate == locale {
			return true
		}
	}
	return false
}
