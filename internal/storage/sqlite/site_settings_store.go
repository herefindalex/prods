package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"prods/internal/identity"
	"prods/internal/site"
)

func (s *Store) SiteSettings(ctx context.Context) (site.Settings, error) {
	var settings site.Settings
	var supportedJSON string
	if err := s.db.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json,time_zone,revision,updated_at
		FROM site_settings WHERE singleton=1`).Scan(
		&settings.DefaultLocale, &supportedJSON, &settings.TimeZone, &settings.Revision, &settings.UpdatedAt,
	); err != nil {
		return site.Settings{}, err
	}
	if err := json.Unmarshal([]byte(supportedJSON), &settings.SupportedLocales); err != nil {
		return site.Settings{}, err
	}
	return settings, nil
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
		if err := tx.QueryRowContext(ctx, `SELECT default_locale,supported_locales_json,time_zone,revision,updated_at
			FROM site_settings WHERE singleton=1`).Scan(
			&settings.DefaultLocale, &supportedJSON, &settings.TimeZone, &settings.Revision, &settings.UpdatedAt,
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
