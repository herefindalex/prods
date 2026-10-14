package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"prods/internal/catalog"
	"prods/internal/catalogfilter"
	"prods/internal/identity"
	"prods/internal/specnumeric"
)

func (s *Store) ConfigureSpecNumericProfile(ctx context.Context, actorID, specID string, expectedRevision int64, profile specnumeric.Profile) (catalog.SpecNumericProfile, error) {
	if err := specnumeric.ValidateProfile(profile); err != nil {
		return catalog.SpecNumericProfile{}, fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
	}
	var result catalog.SpecNumericProfile
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		return configureSpecNumericProfileTx(ctx, tx, actorID, specID, expectedRevision, profile, &result)
	})
	if err != nil {
		return catalog.SpecNumericProfile{}, err
	}
	return result, nil
}

func configureSpecNumericProfileTx(ctx context.Context, tx *sql.Tx, actorID, specID string, expectedRevision int64, profile specnumeric.Profile, result *catalog.SpecNumericProfile) error {
	encoded, err := json.Marshal(profile)
	if err != nil {
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
	var previousJSON string
	var profileRevision int64
	err = tx.QueryRowContext(ctx, `SELECT profile_json,revision FROM spec_numeric_profiles WHERE spec_id=?`, specID).Scan(&previousJSON, &profileRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		var previous specnumeric.Profile
		if err := json.Unmarshal([]byte(previousJSON), &previous); err != nil {
			return err
		}
		if previous.Quantity != profile.Quantity || previous.SemanticRole != profile.SemanticRole {
			return catalog.ErrInvalidSpec
		}
	}
	semantic++
	profileRevision++
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO spec_numeric_profiles(spec_id,spec_semantic_version,revision,profile_json,updated_by,updated_at)
	 VALUES(?,?,?,?,?,?) ON CONFLICT(spec_id) DO UPDATE SET spec_semantic_version=excluded.spec_semantic_version,
	 revision=excluded.revision,profile_json=excluded.profile_json,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, specID, semantic, profileRevision, string(encoded), actorID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE spec_definitions SET semantic_version=?,revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`, semantic, actorID, now, specID, expectedRevision); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO spec_numeric_profile_work(spec_id,profile_revision,spec_semantic_version,status,created_at,updated_at) VALUES(?,?,?,'pending',?,?)`, specID, profileRevision, semantic, now, now); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, actorID, "spec.numeric_profile_configured", "spec", specID, map[string]any{"profile_revision": profileRevision, "spec_semantic_version": semantic}); err != nil {
		return err
	}
	if err := saveSpecSearchMetadataTx(ctx, tx, actorID, specID, expectedRevision+1, catalogfilter.NumericProfile(profile), now); err != nil {
		return err
	}
	*result = catalog.SpecNumericProfile{SpecID: specID, SpecSemanticVersion: semantic, Revision: profileRevision, Profile: profile}
	return nil
}

// One statement freezes Raw, profile and all expectations in one read snapshot.
// Neither legacy normalized history nor display units infer a selection/profile.
func (s *Store) SpecNormalizationContext(ctx context.Context, specValueID string) (catalog.SpecNormalizationContext, error) {
	var result catalog.SpecNormalizationContext
	var encoded string
	var profileSemantic int64
	err := s.db.QueryRowContext(ctx, `SELECT pv.id,pv.product_id,pv.spec_id,pv.raw_value,pv.source_origin_locale,
	 p.revision,pv.source_revision,sd.semantic_version,np.revision,np.profile_json,
	 COALESCE(ns.selected_normalized_value_id,''),COALESCE(ns.preference,''),COALESCE(ns.revision,0),
	 COALESCE(ps.spec_value_id,''),COALESCE(ps.revision,0),np.spec_semantic_version
	 FROM product_spec_values pv JOIN products p ON p.id=pv.product_id JOIN spec_definitions sd ON sd.id=pv.spec_id
	 JOIN spec_numeric_profiles np ON np.spec_id=pv.spec_id
	 LEFT JOIN spec_normalization_selections ns ON ns.spec_value_id=pv.id
	 LEFT JOIN product_spec_sources ps ON ps.product_id=pv.product_id AND ps.spec_id=pv.spec_id
	 WHERE pv.id=?`, specValueID).Scan(&result.SpecValueID, &result.ProductID, &result.SpecID, &result.Raw, &result.SourceLocale,
		&result.Expected.ProductRevision, &result.Expected.SourceRevision, &result.Expected.SpecSemanticVersion,
		&result.Expected.ProfileRevision, &encoded, &result.Selection.SelectedNormalizedValueID, &result.Selection.Preference,
		&result.Expected.SelectionRevision, &result.SourceSelection.SpecValueID, &result.SourceSelection.Revision, &profileSemantic)
	if err != nil {
		return catalog.SpecNormalizationContext{}, err
	}
	if profileSemantic != result.Expected.SpecSemanticVersion {
		return catalog.SpecNormalizationContext{}, catalog.ErrRevisionConflict
	}
	if err := json.Unmarshal([]byte(encoded), &result.Profile); err != nil {
		return catalog.SpecNormalizationContext{}, err
	}
	if err := specnumeric.ValidateProfile(result.Profile); err != nil {
		return catalog.SpecNormalizationContext{}, err
	}
	result.Selection.SpecValueID = result.SpecValueID
	result.Selection.Revision = result.Expected.SelectionRevision
	result.SourceSelection.ProductID, result.SourceSelection.SpecID = result.ProductID, result.SpecID
	return result, nil
}

// Parsing is deliberately completed before acquiring writer admission. The
// frozen source and every expectation are checked again in the write transaction.
func (s *Store) RecordSpecNormalization(ctx context.Context, actorID string, command catalog.NormalizeSpecCommand, result specnumeric.Result) (catalog.NormalizationOutcome, error) {
	c, err := s.SpecNormalizationContext(ctx, command.SpecValueID)
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	if c.Expected != command.Expected {
		return catalog.NormalizationOutcome{}, catalog.ErrRevisionConflict
	}
	encoded := ""
	// Result is produced by the server's named parser on the frozen Context;
	// it is not part of the HTTP command DTO. Validate its controlled schema.
	if result.Candidate != nil {
		if err := specnumeric.ValidateValue(*result.Candidate, c.Profile); err != nil {
			return catalog.NormalizationOutcome{}, fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
		}
		encoded, err = specnumeric.MarshalValue(*result.Candidate)
		if err != nil {
			return catalog.NormalizationOutcome{}, err
		}
	}
	return s.recordControlledNormalization(ctx, actorID, c, command.Expected, encoded, string(result.Reason), catalog.NormalizedAutomatic, false)
}

func (s *Store) AdoptManualSpecValue(ctx context.Context, actorID string, command catalog.AdoptSpecCommand) (catalog.NormalizationOutcome, error) {
	c, err := s.SpecNormalizationContext(ctx, command.SpecValueID)
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	if c.Expected != command.Expected {
		return catalog.NormalizationOutcome{}, catalog.ErrRevisionConflict
	}
	value, err := specnumeric.UnmarshalValue(command.ValueJSON, c.Profile)
	if err != nil {
		return catalog.NormalizationOutcome{}, fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
	}
	encoded, err := specnumeric.MarshalValue(value)
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	return s.recordControlledNormalization(ctx, actorID, c, command.Expected, encoded, "", catalog.NormalizedManual, true)
}

func checkNormalizationContextTx(ctx context.Context, tx *sql.Tx, actorID string, frozen catalog.SpecNormalizationContext, expected catalog.NormalizationExpectation) error {
	if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
		return err
	}
	var actual catalog.NormalizationExpectation
	var raw, origin, shared string
	var state catalog.RecordState
	var applicable bool
	err := tx.QueryRowContext(ctx, `SELECT p.revision,pv.source_revision,sd.semantic_version,np.revision,
	 COALESCE(ns.revision,0),pv.raw_value,pv.source_origin_locale,COALESCE(ps.spec_value_id,''),p.record_state,
	 pv.active AND sd.status='active' AND EXISTS(SELECT 1 FROM category_spec_sets cs
	 JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id WHERE cs.category_id=p.category_id AND sm.spec_id=pv.spec_id)
	 FROM product_spec_values pv JOIN products p ON p.id=pv.product_id
	 JOIN spec_definitions sd ON sd.id=pv.spec_id JOIN spec_numeric_profiles np ON np.spec_id=pv.spec_id
	 LEFT JOIN spec_normalization_selections ns ON ns.spec_value_id=pv.id
	 LEFT JOIN product_spec_sources ps ON ps.product_id=pv.product_id AND ps.spec_id=pv.spec_id
	 WHERE pv.id=? AND pv.product_id=? AND pv.spec_id=?`, frozen.SpecValueID, frozen.ProductID, frozen.SpecID).Scan(
		&actual.ProductRevision, &actual.SourceRevision, &actual.SpecSemanticVersion, &actual.ProfileRevision, &actual.SelectionRevision,
		&raw, &origin, &shared, &state, &applicable)
	if err != nil {
		return err
	}
	if state != catalog.RecordCurrent {
		return catalog.ErrArchivedProduct
	}
	if actual != expected || actual != frozen.Expected || raw != frozen.Raw || origin != frozen.SourceLocale {
		return catalog.ErrRevisionConflict
	}
	if !applicable {
		return catalog.ErrInvalidSpec
	}
	if shared != frozen.SpecValueID {
		return fmt.Errorf("%w: select the shared Raw source before normalization", catalog.ErrRevisionConflict)
	}
	return nil
}

func (s *Store) recordControlledNormalization(ctx context.Context, actorID string, frozen catalog.SpecNormalizationContext, expected catalog.NormalizationExpectation, encoded, reason string, source catalog.NormalizedSource, explicit bool) (catalog.NormalizationOutcome, error) {
	var outcome catalog.NormalizationOutcome
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		return recordControlledNormalizationTx(ctx, tx, actorID, frozen, expected, encoded, reason, source, explicit, &outcome)
	})
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	return outcome, nil
}

func recordControlledNormalizationTx(ctx context.Context, tx *sql.Tx, actorID string, frozen catalog.SpecNormalizationContext, expected catalog.NormalizationExpectation, encoded, reason string, source catalog.NormalizedSource, explicit bool, outcome *catalog.NormalizationOutcome) error {
	id, err := randomID("nrm")
	if err != nil {
		return err
	}
	if err := checkNormalizationContextTx(ctx, tx, actorID, frozen, expected); err != nil {
		return err
	}
	status := catalog.NormalizedCurrent
	state := "candidate"
	if reason != "" || encoded == "" {
		status = catalog.NormalizedFailed
		state = "failed"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if source == catalog.NormalizedAutomatic {
		var previous string
		err := tx.QueryRowContext(ctx, `SELECT id FROM normalized_values WHERE spec_value_id=? AND value_json=? AND source_revision=?
		 AND spec_semantic_version=? AND profile_revision=? AND source_origin_locale=? AND numeric_schema_version=?
		 AND normalizer_version=? AND source='automatic' AND status=? AND failure_reason=? ORDER BY created_at DESC,id DESC LIMIT 1`,
			frozen.SpecValueID, encoded, expected.SourceRevision, expected.SpecSemanticVersion, expected.ProfileRevision, frozen.SourceLocale, specnumeric.SchemaVersion, frozen.Profile.ParserVersion, status, reason).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			*outcome = catalog.NormalizationOutcome{State: state, ReasonCode: reason, NormalizedValueID: previous, ProductRevision: expected.ProductRevision, SelectionRevision: expected.SelectionRevision}
			if status == catalog.NormalizedCurrent && frozen.Selection.Preference != catalog.NormalizedManual {
				if frozen.Selection.SelectedNormalizedValueID == previous {
					outcome.State = "recorded"
					return nil
				}
				return adoptNormalizationTx(ctx, tx, actorID, frozen, previous, source, "product.spec_normalized", now, outcome)
			}
			return nil
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO normalized_values(id,spec_value_id,value_json,source_revision,spec_semantic_version,
	 normalizer_version,source,status,failure_reason,created_by,created_at,profile_revision,source_origin_locale,numeric_schema_version)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, frozen.SpecValueID, encoded, expected.SourceRevision, expected.SpecSemanticVersion,
		frozen.Profile.ParserVersion, source, status, reason, actorID, now, expected.ProfileRevision, frozen.SourceLocale, specnumeric.SchemaVersion); err != nil {
		return err
	}
	*outcome = catalog.NormalizationOutcome{State: state, ReasonCode: reason, NormalizedValueID: id, ProductRevision: expected.ProductRevision, SelectionRevision: expected.SelectionRevision}
	if status == catalog.NormalizedCurrent && (explicit || frozen.Selection.Preference != catalog.NormalizedManual) {
		action := "product.spec_normalized"
		if source == catalog.NormalizedManual {
			action = "product.spec_manual_adopted"
		}
		return adoptNormalizationTx(ctx, tx, actorID, frozen, id, source, action, now, outcome)
	}
	return appendAudit(ctx, tx, actorID, "product.spec_normalization_recorded", "product", frozen.ProductID, map[string]any{"spec_value_id": frozen.SpecValueID, "normalized_value_id": id, "state": state, "reason": reason})
}

func adoptNormalizationTx(ctx context.Context, tx *sql.Tx, actorID string, frozen catalog.SpecNormalizationContext, id string, preference catalog.NormalizedSource, action, now string, outcome *catalog.NormalizationOutcome) error {
	if _, err := tx.ExecContext(ctx, `UPDATE normalized_values SET status='stale' WHERE spec_value_id=? AND id<>? AND status='current'`, frozen.SpecValueID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO spec_normalization_selections(spec_value_id,selected_normalized_value_id,preference,revision,updated_by,updated_at)
	 VALUES(?,?,?,?,?,?) ON CONFLICT(spec_value_id) DO UPDATE SET selected_normalized_value_id=excluded.selected_normalized_value_id,
	 preference=excluded.preference,revision=excluded.revision,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, frozen.SpecValueID, id, preference, frozen.Expected.SelectionRevision+1, actorID, now); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE products SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`, actorID, now, frozen.ProductID, frozen.Expected.ProductRevision)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed != 1 {
		return catalog.ErrRevisionConflict
	}
	outcome.State = "adopted"
	outcome.ProductRevision = frozen.Expected.ProductRevision + 1
	outcome.SelectionRevision = frozen.Expected.SelectionRevision + 1
	outcome.PublicationPending = true
	if err := appendAudit(ctx, tx, actorID, action, "product", frozen.ProductID, map[string]any{"spec_value_id": frozen.SpecValueID, "normalized_value_id": id, "selection_revision": outcome.SelectionRevision, "revision": outcome.ProductRevision}); err != nil {
		return err
	}
	return appendPublicationIntent(ctx, tx, frozen.ProductID, outcome.ProductRevision, action, now)
}

func (s *Store) SelectAutomaticSpecValue(ctx context.Context, actorID string, command catalog.SelectAutomaticSpecCommand) (catalog.NormalizationOutcome, error) {
	c, err := s.SpecNormalizationContext(ctx, command.SpecValueID)
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	var encoded, origin, parser, status, source string
	var sourceRevision, semantic, profileRevision int64
	var schema int
	err = s.db.QueryRowContext(ctx, `SELECT value_json,source_revision,spec_semantic_version,profile_revision,source_origin_locale,numeric_schema_version,normalizer_version,status,source
	 FROM normalized_values WHERE id=? AND spec_value_id=?`, command.CandidateID, command.SpecValueID).Scan(&encoded, &sourceRevision, &semantic, &profileRevision, &origin, &schema, &parser, &status, &source)
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	if sourceRevision != c.Expected.SourceRevision || semantic != c.Expected.SpecSemanticVersion || profileRevision != c.Expected.ProfileRevision || origin != c.SourceLocale || schema != specnumeric.SchemaVersion || parser != c.Profile.ParserVersion || status != string(catalog.NormalizedCurrent) || source != string(catalog.NormalizedAutomatic) {
		return catalog.NormalizationOutcome{}, catalog.ErrRevisionConflict
	}
	if _, err := specnumeric.UnmarshalValue(encoded, c.Profile); err != nil {
		return catalog.NormalizationOutcome{}, fmt.Errorf("%w: %v", catalog.ErrInvalidSpec, err)
	}
	var outcome catalog.NormalizationOutcome
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := checkNormalizationContextTx(ctx, tx, actorID, c, command.Expected); err != nil {
			return err
		}
		var current int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM normalized_values WHERE id=? AND spec_value_id=? AND value_json=? AND source_revision=? AND spec_semantic_version=? AND profile_revision=? AND source_origin_locale=? AND numeric_schema_version=? AND normalizer_version=? AND status='current' AND source='automatic')`, command.CandidateID, c.SpecValueID, encoded, sourceRevision, semantic, profileRevision, origin, schema, parser).Scan(&current); err != nil {
			return err
		}
		if current != 1 {
			return catalog.ErrRevisionConflict
		}
		outcome.NormalizedValueID = command.CandidateID
		return adoptNormalizationTx(ctx, tx, actorID, c, command.CandidateID, catalog.NormalizedAutomatic, "product.spec_automatic_selected", time.Now().UTC().Format(time.RFC3339Nano), &outcome)
	})
	if err != nil {
		return catalog.NormalizationOutcome{}, err
	}
	return outcome, nil
}

// Source identity is independent of numeric profiles and source locale.
func (s *Store) SelectSpecSource(ctx context.Context, actorID string, command catalog.SelectSpecSourceCommand) (catalog.SpecSourceSelection, error) {
	var selected catalog.SpecSourceSelection
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		var productRevision, sourceRevision, selectionRevision int64
		var state catalog.RecordState
		var applicable bool
		if err := tx.QueryRowContext(ctx, `SELECT p.revision,p.record_state,pv.source_revision,COALESCE(ps.revision,0),
		 pv.active AND sd.status='active' AND EXISTS(SELECT 1 FROM category_spec_sets cs JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id WHERE cs.category_id=p.category_id AND sm.spec_id=pv.spec_id)
		 FROM product_spec_values pv JOIN products p ON p.id=pv.product_id JOIN spec_definitions sd ON sd.id=pv.spec_id
		 LEFT JOIN product_spec_sources ps ON ps.product_id=pv.product_id AND ps.spec_id=pv.spec_id
		 WHERE pv.id=? AND pv.product_id=? AND pv.spec_id=?`, command.SpecValueID, command.ProductID, command.SpecID).Scan(&productRevision, &state, &sourceRevision, &selectionRevision, &applicable); err != nil {
			return err
		}
		if state != catalog.RecordCurrent {
			return catalog.ErrArchivedProduct
		}
		if productRevision != command.ExpectedProductRevision || sourceRevision != command.ExpectedSourceRevision || selectionRevision != command.ExpectedSourceSelectionRevision {
			return catalog.ErrRevisionConflict
		}
		if !applicable {
			return catalog.ErrInvalidSpec
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		selected = catalog.SpecSourceSelection{ProductID: command.ProductID, SpecID: command.SpecID, SpecValueID: command.SpecValueID, Revision: selectionRevision + 1}
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_spec_sources(product_id,spec_id,spec_value_id,revision,updated_by,updated_at) VALUES(?,?,?,?,?,?)
		 ON CONFLICT(product_id,spec_id) DO UPDATE SET spec_value_id=excluded.spec_value_id,revision=excluded.revision,updated_by=excluded.updated_by,updated_at=excluded.updated_at`, selected.ProductID, selected.SpecID, selected.SpecValueID, selected.Revision, actorID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE products SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`, actorID, now, selected.ProductID, productRevision); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, actorID, "product.spec_source_selected", "product", selected.ProductID, map[string]any{"spec_id": selected.SpecID, "spec_value_id": selected.SpecValueID, "source_selection_revision": selected.Revision, "revision": productRevision + 1}); err != nil {
			return err
		}
		return appendPublicationIntent(ctx, tx, selected.ProductID, productRevision+1, "product.spec_source_selected", now)
	})
	if err != nil {
		return catalog.SpecSourceSelection{}, err
	}
	return selected, nil
}
