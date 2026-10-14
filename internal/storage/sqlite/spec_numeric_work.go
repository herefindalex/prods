package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ProcessSpecNumericProfileWork advances one durable, bounded profile batch.
// Admission, target-version validation, stale marking, audit, publication intent,
// and the cursor commit together. Public activation snapshots are never edited.
func (s *Store) ProcessSpecNumericProfileWork(ctx context.Context, limit int) (bool, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	worked := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var specID, cursor string
		var revision, semantic int64
		err := tx.QueryRowContext(ctx, `SELECT spec_id,profile_revision,spec_semantic_version,cursor_product_id
   FROM spec_numeric_profile_work WHERE status='pending' ORDER BY created_at,spec_id,profile_revision LIMIT 1`).Scan(&specID, &revision, &semantic, &cursor)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		worked = true
		now := time.Now().UTC().Format(time.RFC3339Nano)
		var current bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM spec_numeric_profiles np JOIN spec_definitions sd ON sd.id=np.spec_id
   WHERE np.spec_id=? AND np.revision=? AND np.spec_semantic_version=? AND sd.semantic_version=?)`, specID, revision, semantic, semantic).Scan(&current); err != nil {
			return err
		}
		if !current {
			// Superseded work may complete only its own durable record.
			_, err = tx.ExecContext(ctx, `UPDATE spec_numeric_profile_work SET status='completed',updated_at=? WHERE spec_id=? AND profile_revision=? AND spec_semantic_version=?`, now, specID, revision, semantic)
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT p.id,p.revision FROM products p WHERE p.record_state='current' AND p.id>?
   AND (EXISTS(SELECT 1 FROM product_spec_values pv WHERE pv.product_id=p.id AND pv.spec_id=?)
    OR EXISTS(SELECT 1 FROM category_spec_sets cs JOIN spec_set_members sm ON sm.spec_set_id=cs.spec_set_id WHERE cs.category_id=p.category_id AND sm.spec_id=?))
   ORDER BY p.id LIMIT ?`, cursor, specID, specID, limit)
		if err != nil {
			return err
		}
		type affectedProduct struct {
			id       string
			revision int64
		}
		products := make([]affectedProduct, 0, limit)
		for rows.Next() {
			var p affectedProduct
			if err = rows.Scan(&p.id, &p.revision); err != nil {
				rows.Close()
				return err
			}
			products = append(products, p)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		for _, p := range products {
			if _, err = tx.ExecContext(ctx, `UPDATE normalized_values SET status='stale' WHERE status='current'
    AND spec_value_id IN (SELECT id FROM product_spec_values WHERE product_id=? AND spec_id=?)
    AND (profile_revision<>? OR spec_semantic_version<>?)`, p.id, specID, revision, semantic); err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE products SET revision=revision+1,updated_by=NULL,updated_at=? WHERE id=? AND revision=?`, now, p.id, p.revision); err != nil {
				return err
			}
			if err = appendAudit(ctx, tx, "", "product.spec_profile_changed", "product", p.id, map[string]any{"spec_id": specID, "profile_revision": revision, "spec_semantic_version": semantic, "revision": p.revision + 1}); err != nil {
				return err
			}
			if err = appendPublicationIntent(ctx, tx, p.id, p.revision+1, "product.spec_profile_changed", now); err != nil {
				return err
			}
			cursor = p.id
		}
		status := "pending"
		if len(products) < limit {
			status = "completed"
		}
		_, err = tx.ExecContext(ctx, `UPDATE spec_numeric_profile_work SET cursor_product_id=?,status=?,updated_at=? WHERE spec_id=? AND profile_revision=? AND spec_semantic_version=? AND status='pending'`, cursor, status, now, specID, revision, semantic)
		return err
	})
	return worked, err
}
