package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/localization"
)

func taxonomySource(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, subjectType, subjectID string) (string, string, map[string]string, error) {
	var locale, description, sourceLocalesJSON string
	err := queryer.QueryRowContext(ctx, `SELECT source_locale,description,source_locales_json FROM taxonomy_content WHERE subject_type=? AND subject_id=?`, subjectType, subjectID).Scan(&locale, &description, &sourceLocalesJSON)
	if err != nil {
		return "", "", nil, err
	}
	var sourceLocales map[string]string
	if err := json.Unmarshal([]byte(sourceLocalesJSON), &sourceLocales); err != nil {
		return "", "", nil, err
	}
	if len(sourceLocales) == 0 {
		sourceLocales, _ = catalog.NormalizeFieldSourceLocales(nil, locale, catalog.TaxonomyTranslatableFields)
	}
	return locale, description, sourceLocales, nil
}

func insertTaxonomySource(ctx context.Context, tx *sql.Tx, subjectType, subjectID, requestedLocale, description string, requestedLocales map[string]string) error {
	var defaultLocale string
	if err := tx.QueryRowContext(ctx, `SELECT v.default_locale FROM public_site_state p
		JOIN website_versions v ON v.site_epoch=p.active_epoch WHERE p.singleton=1`).Scan(&defaultLocale); err != nil {
		return err
	}
	requestedLocale = strings.TrimSpace(requestedLocale)
	if requestedLocale == "" {
		requestedLocale = defaultLocale
	}
	if normalized, ok := localization.NormalizeBuiltinLocale(requestedLocale); ok {
		requestedLocale = normalized
	} else {
		return catalog.ErrInvalidDictionary
	}
	sourceLocales, err := catalog.NormalizeFieldSourceLocales(requestedLocales, requestedLocale, catalog.TaxonomyTranslatableFields)
	if err != nil {
		return catalog.ErrInvalidDictionary
	}
	encoded, err := json.Marshal(sourceLocales)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO taxonomy_content(subject_type,subject_id,source_locale,source_locales_json,description) VALUES(?,?,?,?,?)`,
		subjectType, subjectID, requestedLocale, string(encoded), strings.TrimSpace(description))
	return err
}

func (s *Store) TaxonomyContent(ctx context.Context, subjectType, subjectID string) (catalog.TaxonomyContent, error) {
	content := catalog.TaxonomyContent{SubjectType: subjectType, SubjectID: subjectID}
	var table string
	if subjectType == "category" {
		table = "categories"
	} else if subjectType == "dictionary" {
		table = "dictionary_entries"
	} else {
		return content, catalog.ErrInvalidDictionary
	}
	if err := s.db.QueryRowContext(ctx, `SELECT revision FROM `+table+` WHERE id=?`, subjectID).Scan(&content.SubjectRevision); err != nil {
		return content, err
	}
	var err error
	content.SourceLocale, content.Description, content.SourceLocales, err = taxonomySource(ctx, s.db, subjectType, subjectID)
	if err != nil {
		return content, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT locale,name,description,revision,COALESCE(updated_by,''),updated_at FROM taxonomy_translations WHERE subject_type=? AND subject_id=? ORDER BY locale`, subjectType, subjectID)
	if err != nil {
		return content, err
	}
	defer rows.Close()
	for rows.Next() {
		var item catalog.TaxonomyTranslation
		if err := rows.Scan(&item.Locale, &item.Name, &item.Description, &item.Revision, &item.UpdatedBy, &item.UpdatedAt); err != nil {
			return content, err
		}
		content.Translations = append(content.Translations, item)
	}
	return content, rows.Err()
}

func (s *Store) SaveTaxonomyTranslation(ctx context.Context, actorID, subjectType, subjectID string, expectedRevision int64, item catalog.TaxonomyTranslation) (catalog.TaxonomyContent, error) {
	item.Locale = strings.TrimSpace(item.Locale)
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	if actorID == "" || subjectID == "" || expectedRevision < 1 || item.Locale == "" || (subjectType != "category" && subjectType != "dictionary") {
		return catalog.TaxonomyContent{}, catalog.ErrInvalidDictionary
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var revision int64
		var entityType string
		if subjectType == "category" {
			var status catalog.EntryStatus
			if err := tx.QueryRowContext(ctx, `SELECT revision,status FROM categories WHERE id=?`, subjectID).Scan(&revision, &status); err != nil {
				return err
			}
			entityType = "category"
		} else {
			var kind catalog.DictionaryKind
			var status catalog.EntryStatus
			if err := tx.QueryRowContext(ctx, `SELECT revision,kind,status FROM dictionary_entries WHERE id=?`, subjectID).Scan(&revision, &kind, &status); err != nil {
				return err
			}
			entityType = string(kind)
		}
		if revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		var enabled bool
		if err := tx.QueryRowContext(ctx, `SELECT w.content_editing_enabled FROM taxonomy_content tc CROSS JOIN website_working w WHERE tc.subject_type=? AND tc.subject_id=? AND w.singleton=1`, subjectType, subjectID).Scan(&enabled); err != nil {
			return err
		}
		if _, supported := localization.NormalizeBuiltinLocale(item.Locale); !enabled || !supported {
			return ErrContentLocaleUnavailable
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO taxonomy_translations(subject_type,subject_id,locale,name,description,revision,updated_by,updated_at) VALUES(?,?,?,?,?,1,?,?) ON CONFLICT(subject_type,subject_id,locale) DO UPDATE SET name=excluded.name,description=excluded.description,revision=taxonomy_translations.revision+1,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, subjectType, subjectID, item.Locale, item.Name, item.Description, actorID, now)
		if err != nil {
			return err
		}
		table := "dictionary_entries"
		if subjectType == "category" {
			table = "categories"
		}
		result, err := tx.ExecContext(ctx, `UPDATE `+table+` SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`, actorID, now, subjectID, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return catalog.ErrRevisionConflict
		}
		if _, err := requeueAffectedProductsTx(ctx, tx, actorID, entityType, subjectID, "taxonomy.translation_saved", now); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "taxonomy.translation_saved", subjectType, subjectID, map[string]any{"locale": item.Locale, "revision": expectedRevision + 1})
	})
	if err != nil {
		return catalog.TaxonomyContent{}, err
	}
	return s.TaxonomyContent(ctx, subjectType, subjectID)
}
