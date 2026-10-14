package sqlite

import (
	"context"
	"database/sql"
	"slices"

	"prods/internal/publishing"
)

// Both marker bindings and their actual field rows are read in one SQLite
// snapshot. Working Product revisions deliberately do not join this projection:
// a published activation stays authoritative while a newer edit is pending.
func (s *Store) SpecSearchCandidates(ctx context.Context, specIDs []string, projectionVersion string) (publishing.SpecCandidateSnapshot, error) {
	snapshot := publishing.SpecCandidateSnapshot{ProjectionVersion: projectionVersion}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return snapshot, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT sp.product_id,sp.public_revision,sp.source_revision,sp.site_epoch,sp.visibility_generation,sp.projection_version,f.spec_id
 FROM public_spec_projection_products sp
 JOIN public_activations a ON a.product_id=sp.product_id AND a.public_revision=sp.public_revision AND a.source_revision=sp.source_revision AND a.site_epoch=sp.site_epoch AND a.visibility_generation=sp.visibility_generation
 JOIN public_visibility v ON v.product_id=a.product_id AND v.visible=1 AND v.generation=a.visibility_generation
 JOIN products p ON p.id=a.product_id AND p.record_state='current' AND p.status='published'
 LEFT JOIN public_spec_projection_fields f ON f.product_id=sp.product_id
 ORDER BY sp.product_id,f.spec_id`)
	if err != nil {
		return snapshot, err
	}
	for rows.Next() {
		var binding publishing.SpecCandidateBinding
		var specID sql.NullString
		if err = rows.Scan(&binding.ProductID, &binding.PublicRevision, &binding.SourceRevision, &binding.SiteEpoch, &binding.VisibilityGeneration, &binding.ProjectionVersion, &specID); err != nil {
			rows.Close()
			return snapshot, err
		}
		if len(snapshot.Inventory) == 0 || snapshot.Inventory[len(snapshot.Inventory)-1].ProductID != binding.ProductID {
			snapshot.Inventory = append(snapshot.Inventory, binding)
		}
		if specID.Valid {
			last := &snapshot.Inventory[len(snapshot.Inventory)-1]
			last.SpecIDs = append(last.SpecIDs, specID.String)
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return snapshot, err
	}
	if err = rows.Close(); err != nil {
		return snapshot, err
	}
	// Intersect requested Spec IDs using only the actual rows just read. This
	// imposes no SQL-variable or keyword candidate cap and keeps numeric truth
	// entirely in the gate-protected oracle.
	for _, binding := range snapshot.Inventory {
		matches := binding.ProjectionVersion == projectionVersion
		for _, id := range specIDs {
			matches = matches && slices.Contains(binding.SpecIDs, id)
		}
		if matches {
			snapshot.Candidates = append(snapshot.Candidates, binding)
		}
	}
	if err = tx.Commit(); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// Rebuild accepts views already validated by the publisher. Each binding is
// checked again inside writer admission; delayed work cannot replace a newer
// activation or delete rows installed after the supplied snapshot was taken.
func (s *Store) RebuildSpecSearchProjection(ctx context.Context, activations []publishing.ActivePublication) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		for _, activation := range activations {
			var current bool
			err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM public_activations a
 JOIN public_visibility v ON v.product_id=a.product_id AND v.visible=1 AND v.generation=a.visibility_generation
 JOIN products p ON p.id=a.product_id AND p.record_state='current' AND p.status='published'
 WHERE a.product_id=? AND a.public_revision=? AND a.source_revision=? AND a.site_epoch=? AND a.visibility_generation=?)`, activation.ProductID, activation.PublicRevision, activation.SourceRevision, activation.SiteEpoch, activation.VisibilityGeneration).Scan(&current)
			if err != nil {
				return err
			}
			if !current {
				continue
			}
			if err = upsertSpecSearchProjection(ctx, tx, activation); err != nil {
				return err
			}
		}
		return nil
	})
}

func upsertSpecSearchProjection(ctx context.Context, tx *sql.Tx, activation publishing.ActivePublication) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO public_spec_projection_products(product_id,public_revision,source_revision,site_epoch,visibility_generation,projection_version)
 VALUES(?,?,?,?,?,?) ON CONFLICT(product_id) DO UPDATE SET public_revision=excluded.public_revision,source_revision=excluded.source_revision,site_epoch=excluded.site_epoch,visibility_generation=excluded.visibility_generation,projection_version=excluded.projection_version`, activation.ProductID, activation.PublicRevision, activation.SourceRevision, activation.SiteEpoch, activation.VisibilityGeneration, publishing.SpecProjectionVersion)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM public_spec_projection_fields WHERE product_id=?`, activation.ProductID); err != nil {
		return err
	}
	for _, id := range publishing.SpecProjectionIDs(activation.View) {
		if _, err = tx.ExecContext(ctx, `INSERT INTO public_spec_projection_fields(product_id,spec_id) VALUES(?,?)`, activation.ProductID, id); err != nil {
			return err
		}
	}
	return nil
}
