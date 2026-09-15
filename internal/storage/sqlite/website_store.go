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
	"prods/internal/site"
)

func (s *Store) initializeWebsiteConfiguration(ctx context.Context, configuration site.Configuration) error {
	if err := configuration.Prepare(); err != nil {
		return err
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO website_working(singleton,revision,config_json,updated_at)
		VALUES(1,1,?,?) ON CONFLICT(singleton) DO NOTHING`, string(encoded), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO website_versions(source_working_revision,site_epoch,config_json,created_at)
		SELECT w.revision,p.active_epoch,w.config_json,? FROM website_working w CROSS JOIN public_site_state p
		WHERE w.singleton=1 AND p.singleton=1
		AND NOT EXISTS(SELECT 1 FROM website_versions v WHERE v.site_epoch=p.active_epoch)`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) WebsiteState(ctx context.Context) (site.State, error) {
	var state site.State
	var workingJSON, activeJSON, workingLocalesJSON, activeLocalesJSON string
	var workingOverridesJSON, activeOverridesJSON string
	err := s.db.QueryRowContext(ctx, `SELECT w.revision,v.version,p.active_epoch,r.custom_css_disabled,r.generation,w.config_json,v.config_json,
		w.default_locale,w.enabled_locales_json,w.content_editing_enabled,w.public_copy_overrides_json,
		v.default_locale,v.enabled_locales_json,v.content_editing_enabled,v.public_copy_overrides_json
		FROM website_working w CROSS JOIN public_site_state p CROSS JOIN website_runtime_state r
		JOIN website_versions v ON v.site_epoch=p.active_epoch
		WHERE w.singleton=1 AND p.singleton=1 AND r.singleton=1`).Scan(
		&state.WorkingRevision, &state.ActiveVersion, &state.ActiveEpoch, &state.CustomCSSDisabled, &state.RuntimeGeneration, &workingJSON, &activeJSON,
		&state.WorkingLocalization.DefaultLocale, &workingLocalesJSON, &state.WorkingLocalization.ContentEditingEnabled, &workingOverridesJSON,
		&state.ActiveLocalization.DefaultLocale, &activeLocalesJSON, &state.ActiveLocalization.ContentEditingEnabled, &activeOverridesJSON,
	)
	if err != nil {
		return site.State{}, err
	}
	if err := json.Unmarshal([]byte(workingJSON), &state.Working); err != nil {
		return site.State{}, fmt.Errorf("decode working website configuration: %w", err)
	}
	if err := json.Unmarshal([]byte(activeJSON), &state.Active); err != nil {
		return site.State{}, fmt.Errorf("decode active website configuration: %w", err)
	}
	if err := json.Unmarshal([]byte(workingLocalesJSON), &state.WorkingLocalization.EnabledLocales); err != nil {
		return site.State{}, fmt.Errorf("decode working website locales: %w", err)
	}
	if err := json.Unmarshal([]byte(activeLocalesJSON), &state.ActiveLocalization.EnabledLocales); err != nil {
		return site.State{}, fmt.Errorf("decode active website locales: %w", err)
	}
	if err := json.Unmarshal([]byte(workingOverridesJSON), &state.WorkingLocalization.PublicCopyOverrides); err != nil {
		return site.State{}, fmt.Errorf("decode working public copy overrides: %w", err)
	}
	if err := json.Unmarshal([]byte(activeOverridesJSON), &state.ActiveLocalization.PublicCopyOverrides); err != nil {
		return site.State{}, fmt.Errorf("decode active public copy overrides: %w", err)
	}
	if err := state.Working.Prepare(); err != nil {
		return site.State{}, fmt.Errorf("invalid working website configuration: %w", err)
	}
	if err := state.Active.Prepare(); err != nil {
		return site.State{}, fmt.Errorf("invalid active website configuration: %w", err)
	}
	if err := state.WorkingLocalization.Prepare(); err != nil {
		return site.State{}, fmt.Errorf("invalid working website localization: %w", err)
	}
	if err := state.ActiveLocalization.Prepare(); err != nil {
		return site.State{}, fmt.Errorf("invalid active website localization: %w", err)
	}
	return state, nil
}

func (s *Store) WebsiteCustomCSSState(ctx context.Context) (bool, int64, error) {
	var disabled bool
	var generation int64
	err := s.db.QueryRowContext(ctx, `SELECT custom_css_disabled,generation FROM website_runtime_state WHERE singleton=1`).Scan(&disabled, &generation)
	return disabled, generation, err
}

func (s *Store) SetWebsiteCustomCSSDisabled(ctx context.Context, actorID string, disabled bool, coordinator publishing.VisibilityCoordinator) error {
	return s.withVisibilityTransaction(ctx, coordinator, func(tx *sql.Tx) error {
		var role string
		if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id=? AND status='active'`, actorID).Scan(&role); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrPermissionDenied
			}
			return err
		}
		if role != identity.RoleOwner {
			return ErrPermissionDenied
		}
		var current bool
		if err := tx.QueryRowContext(ctx, `SELECT custom_css_disabled FROM website_runtime_state WHERE singleton=1`).Scan(&current); err != nil {
			return err
		}
		if current == disabled {
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE website_runtime_state
			SET custom_css_disabled=?,generation=generation+1,updated_by=?,updated_at=? WHERE singleton=1`, disabled, actorID, now); err != nil {
			return err
		}
		action := "website.custom_css_enabled"
		if disabled {
			action = "website.custom_css_disabled"
		}
		return appendAudit(ctx, tx, actorID, action, "website", "public", map[string]any{"disabled": disabled})
	})
}

func (s *Store) ActiveWebsiteConfiguration(ctx context.Context) (site.Configuration, int64, int64, error) {
	state, err := s.WebsiteState(ctx)
	if err != nil {
		return site.Configuration{}, 0, 0, err
	}
	return state.Active, state.ActiveVersion, state.ActiveEpoch, nil
}

func (s *Store) WorkingWebsiteConfiguration(ctx context.Context) (site.Configuration, int64, error) {
	var configuration site.Configuration
	var revision int64
	var encoded string
	if err := s.db.QueryRowContext(ctx, `SELECT revision,config_json FROM website_working WHERE singleton=1`).Scan(&revision, &encoded); err != nil {
		return site.Configuration{}, 0, err
	}
	if err := json.Unmarshal([]byte(encoded), &configuration); err != nil {
		return site.Configuration{}, 0, err
	}
	if err := configuration.Prepare(); err != nil {
		return site.Configuration{}, 0, err
	}
	return configuration, revision, nil
}

func websiteConfigurationAtEpoch(ctx context.Context, tx *sql.Tx, epoch int64) (site.Configuration, int64, error) {
	var configuration site.Configuration
	var version int64
	var encoded string
	if err := tx.QueryRowContext(ctx, `SELECT version,config_json FROM website_versions WHERE site_epoch=?`, epoch).Scan(&version, &encoded); err != nil {
		return site.Configuration{}, 0, err
	}
	if err := json.Unmarshal([]byte(encoded), &configuration); err != nil {
		return site.Configuration{}, 0, err
	}
	if err := configuration.Prepare(); err != nil {
		return site.Configuration{}, 0, err
	}
	return configuration, version, nil
}

func (s *Store) SaveWebsiteWorking(ctx context.Context, actorID string, expectedRevision int64, configuration site.Configuration) (site.State, error) {
	if err := configuration.Prepare(); err != nil {
		return site.State{}, err
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return site.State{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		if err := requireWebsiteAssets(ctx, tx, configuration.AssetIDs()); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE website_working
			SET revision=revision+1,config_json=?,updated_by=?,updated_at=?
			WHERE singleton=1 AND revision=?`, string(encoded), actorID, time.Now().UTC().Format(time.RFC3339Nano), expectedRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return catalog.ErrRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "website.working_saved", "website", "working", map[string]any{
			"previous_revision": expectedRevision, "revision": expectedRevision + 1,
		})
	})
	if err != nil {
		return site.State{}, err
	}
	return s.WebsiteState(ctx)
}

func (s *Store) SaveWebsiteLocalization(ctx context.Context, actorID string, expectedRevision int64, settings site.WebsiteLocalization) (site.State, error) {
	if actorID == "" || expectedRevision < 1 {
		return site.State{}, site.ErrInvalidSettings
	}
	if err := settings.Prepare(); err != nil {
		return site.State{}, err
	}
	localesJSON, err := json.Marshal(settings.EnabledLocales)
	if err != nil {
		return site.State{}, err
	}
	overridesJSON, err := json.Marshal(settings.PublicCopyOverrides)
	if err != nil {
		return site.State{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE website_working SET
			revision=revision+1,default_locale=?,enabled_locales_json=?,content_editing_enabled=?,public_copy_overrides_json=?,updated_by=?,updated_at=?
			WHERE singleton=1 AND revision=?`, settings.DefaultLocale, string(localesJSON), settings.ContentEditingEnabled,
			string(overridesJSON), actorID, now, expectedRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return catalog.ErrRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "website.localization_working_saved", "website", "working", map[string]any{
			"previous_revision": expectedRevision,
			"revision":          expectedRevision + 1,
			"default_locale":    settings.DefaultLocale,
			"enabled_locales":   settings.EnabledLocales,
		})
	})
	if err != nil {
		return site.State{}, err
	}
	return s.WebsiteState(ctx)
}

func requireWebsiteAssets(ctx context.Context, tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets WHERE id=? AND owner_type='website' AND owner_id='working'`, id).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return catalog.ErrInvalidAsset
		}
	}
	return nil
}

func (s *Store) WebsiteVersions(ctx context.Context, limit int) ([]site.Version, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := s.db.QueryContext(ctx, `SELECT version,source_working_revision,site_epoch,config_json,
		default_locale,enabled_locales_json,content_editing_enabled,public_copy_overrides_json,created_at
		FROM website_versions ORDER BY version DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]site.Version, 0)
	for rows.Next() {
		var version site.Version
		var encoded, localesJSON, overridesJSON, createdAt string
		if err := rows.Scan(&version.Version, &version.SourceWorkingRevision, &version.SiteEpoch, &encoded,
			&version.Localization.DefaultLocale, &localesJSON, &version.Localization.ContentEditingEnabled, &overridesJSON, &createdAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(encoded), &version.Configuration); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(localesJSON), &version.Localization.EnabledLocales); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(overridesJSON), &version.Localization.PublicCopyOverrides); err != nil {
			return nil, err
		}
		if err := version.Localization.Prepare(); err != nil {
			return nil, err
		}
		version.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) RestoreWebsiteVersion(ctx context.Context, actorID string, expectedWorkingRevision, versionID int64) (site.State, error) {
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		var encoded, defaultLocale, localesJSON, overridesJSON string
		var contentEditingEnabled bool
		if err := tx.QueryRowContext(ctx, `SELECT config_json,default_locale,enabled_locales_json,content_editing_enabled,public_copy_overrides_json
			FROM website_versions WHERE version=?`, versionID).Scan(
			&encoded, &defaultLocale, &localesJSON, &contentEditingEnabled, &overridesJSON,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sql.ErrNoRows
			}
			return err
		}
		var configuration site.Configuration
		if err := json.Unmarshal([]byte(encoded), &configuration); err != nil {
			return err
		}
		if err := configuration.Prepare(); err != nil {
			return err
		}
		localizationSettings := site.WebsiteLocalization{
			DefaultLocale:         defaultLocale,
			ContentEditingEnabled: contentEditingEnabled,
		}
		if err := json.Unmarshal([]byte(localesJSON), &localizationSettings.EnabledLocales); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(overridesJSON), &localizationSettings.PublicCopyOverrides); err != nil {
			return err
		}
		if err := localizationSettings.Prepare(); err != nil {
			return err
		}
		if err := requireWebsiteAssets(ctx, tx, configuration.AssetIDs()); err != nil {
			return err
		}
		encodedBytes, err := json.Marshal(configuration)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE website_working
			SET revision=revision+1,config_json=?,default_locale=?,enabled_locales_json=?,content_editing_enabled=?,public_copy_overrides_json=?,updated_by=?,updated_at=?
			WHERE singleton=1 AND revision=?`, string(encodedBytes), defaultLocale, localesJSON, contentEditingEnabled, overridesJSON,
			actorID, time.Now().UTC().Format(time.RFC3339Nano), expectedWorkingRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return catalog.ErrRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "website.version_restored_to_working", "website_version", fmt.Sprint(versionID), map[string]any{
			"working_revision": expectedWorkingRevision + 1,
		})
	})
	if err != nil {
		return site.State{}, err
	}
	return s.WebsiteState(ctx)
}
