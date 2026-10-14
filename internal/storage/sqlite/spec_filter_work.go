package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/specnumeric"
)

func (s *Store) StartSpecNormalizationWork(ctx context.Context, actorID, specID string, expectedRevision int64) (catalog.SpecNormalizationWork, error) {
	id, err := randomID("snw")
	if err != nil {
		return catalog.SpecNormalizationWork{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		p, err := readSpecSearchProfile(ctx, tx, specID)
		if err != nil {
			return err
		}
		if p.Revision != expectedRevision {
			return catalog.ErrRevisionConflict
		}
		if p.Profile.Kind != "numeric" {
			return catalog.ErrInvalidSpec
		}
		var profileRevision int64
		if err = tx.QueryRowContext(ctx, `SELECT revision FROM spec_numeric_profiles WHERE spec_id=?`, specID).Scan(&profileRevision); err != nil {
			return err
		}
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT id FROM spec_normalization_work WHERE spec_id=? AND profile_revision=? AND spec_semantic_version=? AND status='pending' ORDER BY created_at,id LIMIT 1`, specID, profileRevision, p.SpecSemanticVersion).Scan(&existing)
		if err == nil {
			id = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err = tx.ExecContext(ctx, `INSERT INTO spec_normalization_work(id,spec_id,actor_id,profile_revision,spec_semantic_version,status,total,created_at,updated_at) VALUES(?,?,?,?,?,'pending',0,?,?)`, id, specID, actorID, profileRevision, p.SpecSemanticVersion, now, now); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO spec_normalization_work_items(work_id,spec_value_id,status)
 SELECT ?,pv.id,'pending' FROM product_spec_sources ps JOIN product_spec_values pv ON pv.id=ps.spec_value_id
 JOIN products p ON p.id=pv.product_id JOIN spec_definitions sd ON sd.id=pv.spec_id
 WHERE pv.spec_id=? AND ps.product_id=pv.product_id AND ps.spec_id=pv.spec_id AND pv.active=1 AND sd.status='active' AND p.record_state='current'
 AND EXISTS(SELECT 1 FROM category_spec_sets cs JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id WHERE cs.category_id=p.category_id AND sm.spec_id=pv.spec_id)`, id, specID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE spec_normalization_work SET total=(SELECT COUNT(*) FROM spec_normalization_work_items WHERE work_id=?) WHERE id=?`, id, id); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "spec.normalization_work_started", "spec", specID, map[string]any{"work_id": id, "profile_revision": profileRevision, "spec_semantic_version": p.SpecSemanticVersion})
	})
	if err != nil {
		return catalog.SpecNormalizationWork{}, err
	}
	return s.SpecNormalizationWork(ctx, id)
}

func (s *Store) SpecNormalizationWork(ctx context.Context, id string) (catalog.SpecNormalizationWork, error) {
	var w catalog.SpecNormalizationWork
	err := s.db.QueryRowContext(ctx, `SELECT id,spec_id,status,cursor_spec_value_id,profile_revision,spec_semantic_version,processed,failed,skipped,total FROM spec_normalization_work WHERE id=?`, id).Scan(&w.ID, &w.SpecID, &w.Status, &w.Cursor, &w.ProfileRevision, &w.SemanticVersion, &w.Processed, &w.Failed, &w.Skipped, &w.Total)
	return w, err
}

func (s *Store) SpecNormalizationWorkItems(ctx context.Context, id string, page, pageSize int) ([]catalog.SpecNormalizationWorkItem, int, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, 0, catalog.ErrInvalidSpec
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var total int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM spec_normalization_work_items WHERE work_id=?`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT spec_value_id,status,reason FROM spec_normalization_work_items WHERE work_id=? ORDER BY spec_value_id LIMIT ? OFFSET ?`, id, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []catalog.SpecNormalizationWorkItem{}
	for rows.Next() {
		var item catalog.SpecNormalizationWorkItem
		if err = rows.Scan(&item.SpecValueID, &item.State, &item.Reason); err != nil {
			return nil, 0, err
		}
		result = append(result, item)
	}
	return result, total, rows.Err()
}

// Each item uses a frozen parsing context outside admission, then commits the
// controlled result, publication intent, audit and durable progress together.
func (s *Store) ProcessSpecNormalizationWork(ctx context.Context, limit int) (bool, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	var id, actor string
	err := s.db.QueryRowContext(ctx, `SELECT id,actor_id FROM spec_normalization_work WHERE status='pending' ORDER BY created_at,id LIMIT 1`).Scan(&id, &actor)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	work, err := s.SpecNormalizationWork(ctx, id)
	if err != nil {
		return true, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT spec_value_id FROM spec_normalization_work_items WHERE work_id=? AND status='pending' ORDER BY spec_value_id LIMIT ?`, id, limit)
	if err != nil {
		return true, err
	}
	var values []string
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			rows.Close()
			return true, err
		}
		values = append(values, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return true, err
	}
	rows.Close()
	for _, valueID := range values {
		frozen, contextErr := s.SpecNormalizationContext(ctx, valueID)
		state, reason, encoded := "completed", "", ""
		if contextErr != nil {
			state, reason = "failed", "context_changed"
		} else if frozen.Selection.Preference == catalog.NormalizedManual {
			state, reason = "skipped", "manual_preference"
		} else {
			parsed := specnumeric.Normalize(frozen.Raw, frozen.Profile)
			reason = string(parsed.Reason)
			if parsed.Candidate != nil {
				encoded, err = specnumeric.MarshalValue(*parsed.Candidate)
				if err != nil {
					return true, err
				}
			}
			if reason != "" || encoded == "" {
				state = "failed"
			}
		}
		err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
			var current bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spec_normalization_work w JOIN spec_numeric_profiles np ON np.spec_id=w.spec_id JOIN spec_definitions sd ON sd.id=w.spec_id
 WHERE w.id=? AND w.status='pending' AND np.revision=w.profile_revision AND np.spec_semantic_version=w.spec_semantic_version AND sd.semantic_version=w.spec_semantic_version)`, id).Scan(&current); err != nil {
				return err
			}
			if !current {
				_, err := tx.ExecContext(ctx, `UPDATE spec_normalization_work SET status='superseded',updated_at=? WHERE id=? AND status='pending'`, time.Now().UTC().Format(time.RFC3339Nano), id)
				return err
			}
			if err := requireActorCapability(ctx, tx, actor, identity.CapabilityCatalogEdit); err != nil {
				state, reason = "failed", "permission_changed"
			} else if contextErr == nil {
				if frozen.Expected.ProfileRevision != work.ProfileRevision || frozen.Expected.SpecSemanticVersion != work.SemanticVersion {
					state, reason = "failed", "profile_changed"
				} else if err := checkNormalizationContextTx(ctx, tx, actor, frozen, frozen.Expected); err != nil {
					state, reason = "failed", "context_changed"
				} else if state != "skipped" {
					var outcome catalog.NormalizationOutcome
					if err := recordControlledNormalizationTx(ctx, tx, actor, frozen, frozen.Expected, encoded, reason, catalog.NormalizedAutomatic, false, &outcome); err != nil {
						return err
					}
				}
			}
			result, err := tx.ExecContext(ctx, `UPDATE spec_normalization_work_items SET status=?,reason=? WHERE work_id=? AND spec_value_id=? AND status='pending'`, state, reason, id, valueID)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n == 0 {
				return nil
			}
			failed, skipped := 0, 0
			if state == "failed" {
				failed = 1
			}
			if state == "skipped" {
				skipped = 1
			}
			_, err = tx.ExecContext(ctx, `UPDATE spec_normalization_work SET processed=processed+1,failed=failed+?,skipped=skipped+?,cursor_spec_value_id=?,updated_at=? WHERE id=? AND status='pending'`, failed, skipped, valueID, time.Now().UTC().Format(time.RFC3339Nano), id)
			return err
		})
		if err != nil {
			return true, err
		}
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE spec_normalization_work SET status='completed',updated_at=? WHERE id=? AND status='pending' AND processed=total`, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		return appendAudit(ctx, tx, actor, "spec.normalization_work_completed", "spec", work.SpecID, map[string]any{"work_id": id})
	})
	return true, err
}
