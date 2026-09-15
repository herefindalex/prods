package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"prods/internal/localization"
)

func (s *Store) InstallOfficialPublicCopy(ctx context.Context, catalog localization.PublicCopyCatalog) error {
	catalog.OfficialBundle = strings.TrimSpace(catalog.OfficialBundle)
	catalog.ReviewStatus = strings.TrimSpace(catalog.ReviewStatus)
	if catalog.OfficialBundle == "" || catalog.ReviewStatus == "" || len(catalog.Definitions) == 0 {
		return localization.ErrInvalidPublicCopy
	}
	definitions := make(map[string]localization.PublicCopyDefinition, len(catalog.Definitions))
	for index := range catalog.Definitions {
		definition := catalog.Definitions[index]
		if err := definition.Prepare(); err != nil || definition.OfficialBundle != catalog.OfficialBundle {
			return localization.ErrInvalidPublicCopy
		}
		if _, exists := definitions[definition.Key]; exists {
			return localization.ErrInvalidPublicCopy
		}
		definitions[definition.Key] = definition
	}
	localeCodes := localization.BuiltinLocaleCodes()
	defaults := make(map[string]map[string]localization.PublicCopyDefault, len(definitions))
	for _, item := range catalog.Defaults {
		item.Key = strings.TrimSpace(item.Key)
		item.Value = strings.TrimSpace(item.Value)
		item.OfficialBundle = strings.TrimSpace(item.OfficialBundle)
		item.Locale, _ = localization.NormalizeBuiltinLocale(item.Locale)
		definition, exists := definitions[item.Key]
		if !exists || item.Locale == "" || item.Value == "" || item.DefinitionVersion != definition.DefinitionVersion || item.OfficialBundle != catalog.OfficialBundle {
			return localization.ErrInvalidPublicCopy
		}
		if err := localization.ValidatePublicCopyValue(definition, item.Value); err != nil {
			return localization.ErrInvalidPublicCopy
		}
		if defaults[item.Key] == nil {
			defaults[item.Key] = make(map[string]localization.PublicCopyDefault)
		}
		if _, duplicate := defaults[item.Key][item.Locale]; duplicate {
			return localization.ErrInvalidPublicCopy
		}
		defaults[item.Key][item.Locale] = item
	}
	for key := range definitions {
		for _, locale := range localeCodes {
			if _, exists := defaults[key][locale]; !exists {
				return fmt.Errorf("%w: missing %s default for %s", localization.ErrInvalidPublicCopy, locale, key)
			}
		}
	}

	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var active string
		err := tx.QueryRowContext(ctx, `SELECT official_bundle_version FROM public_copy_active WHERE singleton=1`).Scan(&active)
		if err == nil && active == catalog.OfficialBundle {
			return nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO public_copy_bundles(version,source,review_status,installed_at) VALUES(?,'official',?,?)`,
			catalog.OfficialBundle, catalog.ReviewStatus, now); err != nil {
			return err
		}
		keys := make([]string, 0, len(definitions))
		for key := range definitions {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			definition := definitions[key]
			requiredJSON, err := json.Marshal(definition.RequiredPlaceholders)
			if err != nil {
				return err
			}
			allowedJSON, err := json.Marshal(definition.AllowedPlaceholders)
			if err != nil {
				return err
			}
			sampleJSON, err := json.Marshal(definition.Sample)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO public_copy_definitions(
				copy_key,definition_version,description,value_kind,required_placeholders_json,allowed_placeholders_json,sample_json,official_bundle_version
			) VALUES(?,?,?,?,?,?,?,?)`, definition.Key, definition.DefinitionVersion, definition.Description, definition.ValueKind,
				string(requiredJSON), string(allowedJSON), string(sampleJSON), catalog.OfficialBundle); err != nil {
				return err
			}
			for _, locale := range localeCodes {
				item := defaults[key][locale]
				if _, err := tx.ExecContext(ctx, `INSERT INTO public_copy_defaults(copy_key,locale,value,definition_version,official_bundle_version)
					VALUES(?,?,?,?,?)`, item.Key, item.Locale, item.Value, item.DefinitionVersion, item.OfficialBundle); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO public_copy_active(singleton,official_bundle_version,activated_at) VALUES(1,?,?)
			ON CONFLICT(singleton) DO UPDATE SET official_bundle_version=excluded.official_bundle_version,activated_at=excluded.activated_at`, catalog.OfficialBundle, now)
		return err
	})
}

func (s *Store) PublicCopyCatalog(ctx context.Context) (localization.PublicCopyCatalog, error) {
	var catalog localization.PublicCopyCatalog
	if err := s.db.QueryRowContext(ctx, `SELECT a.official_bundle_version,b.review_status
		FROM public_copy_active a JOIN public_copy_bundles b ON b.version=a.official_bundle_version WHERE a.singleton=1`).Scan(
		&catalog.OfficialBundle, &catalog.ReviewStatus,
	); err != nil {
		return catalog, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT copy_key,definition_version,description,value_kind,required_placeholders_json,allowed_placeholders_json,sample_json
		FROM public_copy_definitions WHERE official_bundle_version=? ORDER BY copy_key`, catalog.OfficialBundle)
	if err != nil {
		return catalog, err
	}
	for rows.Next() {
		var definition localization.PublicCopyDefinition
		var requiredJSON, allowedJSON, sampleJSON string
		definition.OfficialBundle = catalog.OfficialBundle
		if err := rows.Scan(&definition.Key, &definition.DefinitionVersion, &definition.Description, &definition.ValueKind,
			&requiredJSON, &allowedJSON, &sampleJSON); err != nil {
			rows.Close()
			return catalog, err
		}
		if err := json.Unmarshal([]byte(requiredJSON), &definition.RequiredPlaceholders); err != nil {
			rows.Close()
			return catalog, err
		}
		if err := json.Unmarshal([]byte(allowedJSON), &definition.AllowedPlaceholders); err != nil {
			rows.Close()
			return catalog, err
		}
		if err := json.Unmarshal([]byte(sampleJSON), &definition.Sample); err != nil {
			rows.Close()
			return catalog, err
		}
		catalog.Definitions = append(catalog.Definitions, definition)
	}
	if err := rows.Close(); err != nil {
		return catalog, err
	}
	if err := rows.Err(); err != nil {
		return catalog, err
	}
	defaultRows, err := s.db.QueryContext(ctx, `SELECT copy_key,locale,value,definition_version
		FROM public_copy_defaults WHERE official_bundle_version=? ORDER BY copy_key,locale`, catalog.OfficialBundle)
	if err != nil {
		return catalog, err
	}
	defer defaultRows.Close()
	for defaultRows.Next() {
		var item localization.PublicCopyDefault
		item.OfficialBundle = catalog.OfficialBundle
		if err := defaultRows.Scan(&item.Key, &item.Locale, &item.Value, &item.DefinitionVersion); err != nil {
			return catalog, err
		}
		catalog.Defaults = append(catalog.Defaults, item)
	}
	return catalog, defaultRows.Err()
}
