package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"prods/internal/identity"
	"prods/internal/localization"
	"prods/internal/site"
)

func (s *Store) SiteSettings(ctx context.Context) (site.Settings, error) {
	var settings site.Settings
	var supportedJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json,content_multilingual_enabled,time_zone,revision,updated_at
		FROM site_settings WHERE singleton=1`).Scan(
		&settings.DefaultLocale, &supportedJSON, &settings.ContentMultilingualEnabled, &settings.TimeZone, &settings.Revision, &settings.UpdatedAt,
	); err != nil {
		return site.Settings{}, err
	}
	if err := json.Unmarshal([]byte(supportedJSON), &settings.SupportedLocales); err != nil {
		return site.Settings{}, err
	}
	return settings, nil
}

func (s *Store) UpdateContentLocalization(ctx context.Context, actorID string, expectedRevision int64, enabled bool, supported []string) (site.Settings, error) {
	if actorID == "" || expectedRevision < 1 {
		return site.Settings{}, site.ErrInvalidSettings
	}
	var settings site.Settings
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		var encoded string
		if err := tx.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json,content_multilingual_enabled,time_zone,revision,updated_at FROM site_settings WHERE singleton=1`).Scan(
			&settings.DefaultLocale, &encoded, &settings.ContentMultilingualEnabled, &settings.TimeZone, &settings.Revision, &settings.UpdatedAt); err != nil {
			return err
		}
		if settings.Revision != expectedRevision {
			return site.ErrSettingsConflict
		}
		_, normalized, err := localization.NormalizeSupported(settings.DefaultLocale, supported)
		if err != nil {
			return site.ErrInvalidSettings
		}
		encodedBytes, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE site_settings SET supported_locales_json=?,content_multilingual_enabled=?,revision=revision+1,updated_by=?,updated_at=? WHERE singleton=1 AND revision=?`, string(encodedBytes), enabled, actorID, now, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return site.ErrSettingsConflict
		}
		settings.SupportedLocales = normalized
		settings.ContentMultilingualEnabled = enabled
		settings.Revision++
		settings.UpdatedAt = now
		rows, err := tx.QueryContext(ctx, `SELECT id,revision FROM products WHERE record_state='current' AND status='published'`)
		if err != nil {
			return err
		}
		type pending struct {
			id       string
			revision int64
		}
		var products []pending
		for rows.Next() {
			var p pending
			if err := rows.Scan(&p.id, &p.revision); err != nil {
				rows.Close()
				return err
			}
			products = append(products, p)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, p := range products {
			if err := appendPublicationIntent(ctx, tx, p.id, p.revision, "site.content_localization_updated", now); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, actorID, "site.content_localization_updated", "site_settings", "site", map[string]any{"supported_locales": normalized, "enabled": enabled, "revision": settings.Revision})
	})
	return settings, err
}

func (s *Store) UpdateSiteTimeZone(ctx context.Context, actorID string, expectedRevision int64, value string) (site.Settings, error) {
	timeZone, err := site.PrepareTimeZone(value)
	if err != nil || actorID == "" || expectedRevision < 1 {
		return site.Settings{}, site.ErrInvalidSettings
	}

	var settings site.Settings
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		var supportedJSON string
		if err := tx.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json,content_multilingual_enabled,time_zone,revision,updated_at
			FROM site_settings WHERE singleton=1`).Scan(
			&settings.DefaultLocale, &supportedJSON, &settings.ContentMultilingualEnabled, &settings.TimeZone, &settings.Revision, &settings.UpdatedAt,
		); err != nil {
			return err
		}
		if settings.Revision != expectedRevision {
			return site.ErrSettingsConflict
		}
		if err := json.Unmarshal([]byte(supportedJSON), &settings.SupportedLocales); err != nil {
			return err
		}
		if settings.TimeZone == timeZone {
			return nil
		}

		previous := settings.TimeZone
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE site_settings
			SET time_zone=?,revision=revision+1,updated_by=?,updated_at=?
			WHERE singleton=1 AND revision=?`, timeZone, actorID, now, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return site.ErrSettingsConflict
		}
		settings.TimeZone = timeZone
		settings.Revision++
		settings.UpdatedAt = now
		return appendAudit(ctx, tx, actorID, "site.time_zone_updated", "site_settings", "site", map[string]any{
			"previous_time_zone": previous,
			"time_zone":          timeZone,
			"revision":           settings.Revision,
		})
	})
	if errors.Is(err, sql.ErrNoRows) {
		return site.Settings{}, site.ErrInvalidSettings
	}
	return settings, err
}
