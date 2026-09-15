package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func (s *Store) AddProductImage(ctx context.Context, actorID string, expectedRevision int64, image catalog.ProductImage) (catalog.ProductImage, error) {
	if image.ID == "" {
		id, err := randomID("img")
		if err != nil {
			return catalog.ProductImage{}, err
		}
		image.ID = id
	}
	if err := image.Prepare(); err != nil {
		return catalog.ProductImage{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if err := requireCurrentProductRevision(ctx, tx, image.ProductID, expectedRevision); err != nil {
			return err
		}
		if image.AssetID != "" {
			if err := requireProductImageAsset(ctx, tx, image.AssetID); err != nil {
				return err
			}
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM product_images WHERE product_id=?`, image.ProductID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			image.Primary = true
		}
		if image.Primary {
			if _, err := tx.ExecContext(ctx, `UPDATE product_images SET is_primary=0 WHERE product_id=?`, image.ProductID); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_images(id,product_id,asset_id,external_url,alt_text,sort_order,is_primary,created_by,created_at)
			VALUES(?,?,?,?,?,?,?,?,?)`, image.ID, image.ProductID, nullable(image.AssetID), nullable(image.ExternalURL), image.AltText, image.SortOrder, image.Primary, actorID, now); err != nil {
			return err
		}
		return bumpProductForImage(ctx, tx, actorID, image.ProductID, expectedRevision, "product.image_added", image.ID, now)
	})
	return image, err
}

func (s *Store) UpdateProductImage(ctx context.Context, actorID string, expectedRevision int64, image catalog.ProductImage) (catalog.ProductImage, error) {
	if err := image.Prepare(); err != nil {
		return catalog.ProductImage{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if err := requireCurrentProductRevision(ctx, tx, image.ProductID, expectedRevision); err != nil {
			return err
		}
		var assetID, externalURL string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(asset_id,''),COALESCE(external_url,'') FROM product_images WHERE id=? AND product_id=?`, image.ID, image.ProductID).
			Scan(&assetID, &externalURL); err != nil {
			return err
		}
		if assetID != image.AssetID || externalURL != image.ExternalURL {
			return catalog.ErrInvalidAsset
		}
		if image.Primary {
			if _, err := tx.ExecContext(ctx, `UPDATE product_images SET is_primary=0 WHERE product_id=?`, image.ProductID); err != nil {
				return err
			}
		} else {
			var currentPrimary int
			if err := tx.QueryRowContext(ctx, `SELECT is_primary FROM product_images WHERE id=? AND product_id=?`, image.ID, image.ProductID).Scan(&currentPrimary); err != nil {
				return err
			}
			if currentPrimary != 0 {
				return catalog.ErrInvalidAsset
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE product_images SET alt_text=?,sort_order=?,is_primary=? WHERE id=? AND product_id=?`, image.AltText, image.SortOrder, image.Primary, image.ID, image.ProductID)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sql.ErrNoRows
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		return bumpProductForImage(ctx, tx, actorID, image.ProductID, expectedRevision, "product.image_updated", image.ID, now)
	})
	return image, err
}

func (s *Store) DeleteProductImage(ctx context.Context, actorID, productID, imageID string, expectedRevision int64) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityCatalogEdit); err != nil {
			return err
		}
		if err := requireCurrentProductRevision(ctx, tx, productID, expectedRevision); err != nil {
			return err
		}
		var primary bool
		if err := tx.QueryRowContext(ctx, `SELECT is_primary FROM product_images WHERE id=? AND product_id=?`, imageID, productID).Scan(&primary); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM product_images WHERE id=? AND product_id=?`, imageID, productID); err != nil {
			return err
		}
		if primary {
			if _, err := tx.ExecContext(ctx, `UPDATE product_images SET is_primary=1 WHERE id=(SELECT id FROM product_images WHERE product_id=? ORDER BY sort_order,id LIMIT 1)`, productID); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		return bumpProductForImage(ctx, tx, actorID, productID, expectedRevision, "product.image_deleted", imageID, now)
	})
}

func (s *Store) ProductImages(ctx context.Context, productID string) ([]catalog.ProductImage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,product_id,COALESCE(asset_id,''),COALESCE(external_url,''),alt_text,sort_order,is_primary
		FROM product_images WHERE product_id=? ORDER BY is_primary DESC,sort_order,id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	images := make([]catalog.ProductImage, 0)
	for rows.Next() {
		var image catalog.ProductImage
		if err := rows.Scan(&image.ID, &image.ProductID, &image.AssetID, &image.ExternalURL, &image.AltText, &image.SortOrder, &image.Primary); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func requireCurrentProductRevision(ctx context.Context, tx *sql.Tx, productID string, expectedRevision int64) error {
	var state catalog.RecordState
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT record_state,revision FROM products WHERE id=?`, productID).Scan(&state, &revision); err != nil {
		return err
	}
	if state != catalog.RecordCurrent {
		return catalog.ErrArchivedProduct
	}
	if revision != expectedRevision {
		return catalog.ErrRevisionConflict
	}
	return nil
}

func requireProductImageAsset(ctx context.Context, tx *sql.Tx, assetID string) error {
	var mime string
	err := tx.QueryRowContext(ctx, `SELECT mime_type FROM assets WHERE id=?`, assetID).Scan(&mime)
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.ErrInvalidAsset
	}
	if err != nil {
		return err
	}
	if mime != "image/jpeg" && mime != "image/png" && mime != "image/webp" {
		return catalog.ErrInvalidAsset
	}
	return nil
}

func bumpProductForImage(ctx context.Context, tx *sql.Tx, actorID, productID string, expectedRevision int64, action, imageID, now string) error {
	result, err := tx.ExecContext(ctx, `UPDATE products SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=? AND record_state='current'`, actorID, now, productID, expectedRevision)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return catalog.ErrRevisionConflict
	}
	if err := appendAudit(ctx, tx, actorID, action, "product", productID, map[string]any{"image_id": imageID}); err != nil {
		return err
	}
	return appendPublicationIntent(ctx, tx, productID, expectedRevision+1, action, now)
}

func cloneProductImages(ctx context.Context, tx *sql.Tx, sourceProductID, targetProductID, actorID, now string) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,COALESCE(asset_id,''),COALESCE(external_url,''),alt_text,sort_order,is_primary FROM product_images WHERE product_id=? ORDER BY sort_order,id`, sourceProductID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sourceID, assetID, externalURL, altText string
		var sortOrder int
		var primary bool
		if err := rows.Scan(&sourceID, &assetID, &externalURL, &altText, &sortOrder, &primary); err != nil {
			return err
		}
		id, err := randomID("img")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO product_images(id,product_id,asset_id,external_url,alt_text,sort_order,is_primary,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
			id, targetProductID, nullable(assetID), nullable(externalURL), altText, sortOrder, primary, actorID, now); err != nil {
			return err
		}
	}
	return rows.Err()
}
