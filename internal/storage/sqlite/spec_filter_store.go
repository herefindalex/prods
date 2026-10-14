package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/identity"
	"prods/internal/specnumeric"
)

type specProfileQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readSpecSearchProfile(ctx context.Context, q specProfileQuerier, id string) (catalog.SpecSearchProfile, error) {
	var result catalog.SpecSearchProfile
	var metadata, numeric string
	var numericSemantic int64
	err := q.QueryRowContext(ctx, `SELECT sd.id,sd.semantic_version,sd.revision,sd.filterable,COALESCE(sf.profile_json,''),COALESCE(np.profile_json,''),COALESCE(np.spec_semantic_version,0)
 FROM spec_definitions sd LEFT JOIN spec_filter_profiles sf ON sf.spec_id=sd.id LEFT JOIN spec_numeric_profiles np ON np.spec_id=sd.id WHERE sd.id=?`, id).Scan(&result.SpecID, &result.SpecSemanticVersion, &result.Revision, &result.Filterable, &metadata, &numeric, &numericSemantic)
	if err != nil {
		return result, err
	}
	if metadata == "" {
		result.Profile = catalogfilter.DefaultTextProfile()
	} else if err = json.Unmarshal([]byte(metadata), &result.Profile); err != nil {
		return result, err
	}
	if result.Profile.Kind == "numeric" {
		if numeric == "" || numericSemantic != result.SpecSemanticVersion {
			return result, catalog.ErrInvalidSpec
		}
		var p specnumeric.Profile
		if err = json.Unmarshal([]byte(numeric), &p); err != nil {
			return result, err
		}
		result.Profile.Numeric = &p
	}
	if err = catalogfilter.ValidateProfile(result.Profile); err != nil {
		return result, fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
	}
	return result, nil
}

func (s *Store) SpecSearchProfile(ctx context.Context, id string) (catalog.SpecSearchProfile, error) {
	return readSpecSearchProfile(ctx, s.db, id)
}

func saveSpecSearchMetadataTx(ctx context.Context, tx *sql.Tx, actorID, id string, revision int64, p catalogfilter.Profile, now string) error {
	// Numeric meaning lives solely in spec_numeric_profiles. Metadata records
	// only presentation kind/operators; readers attach that authoritative row.
	p.Numeric = nil
	encoded, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spec_filter_profiles(spec_id,revision,profile_json,updated_by,updated_at) VALUES(?,?,?,?,?)
 ON CONFLICT(spec_id) DO UPDATE SET revision=excluded.revision,profile_json=excluded.profile_json,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, id, revision, string(encoded), actorID, now)
	return err
}

func (s *Store) ConfigureSpecSearchProfile(ctx context.Context, actorID, specID string, expectedRevision int64, desired catalog.SpecSearchProfile) (catalog.SpecSearchProfile, error) {
	if err := catalogfilter.ValidateProfile(desired.Profile); err != nil {
		return catalog.SpecSearchProfile{}, fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
	}
	var result catalog.SpecSearchProfile
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if desired.Profile.Kind == "numeric" {
			var numeric catalog.SpecNumericProfile
			if err := configureSpecNumericProfileTx(ctx, tx, actorID, specID, expectedRevision, *desired.Profile.Numeric, &numeric); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE spec_definitions SET filterable=? WHERE id=?`, desired.Filterable, specID); err != nil {
				return err
			}
			var err error
			result, err = readSpecSearchProfile(ctx, tx, specID)
			return err
		}
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var revision, semantic int64
		if err := tx.QueryRowContext(ctx, `SELECT revision,semantic_version FROM spec_definitions WHERE id=?`, specID).Scan(&revision, &semantic); err != nil {
			return err
		}
		if revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		revision++
		semantic++
		if _, err := tx.ExecContext(ctx, `UPDATE spec_definitions SET filterable=?,revision=?,semantic_version=?,updated_by=?,updated_at=? WHERE id=? AND revision=?`, desired.Filterable, revision, semantic, actorID, now, specID, expectedRevision); err != nil {
			return err
		}
		if err := saveSpecSearchMetadataTx(ctx, tx, actorID, specID, revision, desired.Profile, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO spec_filter_profile_work(spec_id,profile_revision,spec_semantic_version,status,created_at,updated_at) VALUES(?,?,?,'pending',?,?)`, specID, revision, semantic, now, now); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "spec.search_profile_configured", "spec", specID, map[string]any{"revision": revision, "spec_semantic_version": semantic, "kind": desired.Profile.Kind, "filterable": desired.Filterable}); err != nil {
			return err
		}
		var err error
		result, err = readSpecSearchProfile(ctx, tx, specID)
		return err
	})
	if err != nil {
		return catalog.SpecSearchProfile{}, err
	}
	return result, nil
}

// Initial seed definitions are installed once, before any Product exists.
// Adoption is deferred to bounded durable jobs after Installer commits Ready.
func installSpecSearchProfileTx(ctx context.Context, tx *sql.Tx, actorID string, spec catalog.SpecDefinition, profile catalogfilter.Profile, now string) error {
	if err := catalogfilter.ValidateProfile(profile); err != nil {
		return fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
	}
	if profile.Numeric != nil {
		encoded, err := json.Marshal(profile.Numeric)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO spec_numeric_profiles(spec_id,spec_semantic_version,revision,profile_json,updated_by,updated_at) VALUES(?,?,1,?,?,?)`, spec.ID, spec.SemanticVer, string(encoded), actorID, now); err != nil {
			return err
		}
	}
	return saveSpecSearchMetadataTx(ctx, tx, actorID, spec.ID, 1, profile, now)
}
