package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

func (s *Store) CreateAsset(ctx context.Context, actorID string, asset catalog.Asset) (catalog.Asset, error) {
	if err := asset.Prepare(); err != nil {
		return catalog.Asset{}, err
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		capability := identity.CapabilityCatalogEdit
		if asset.OwnerType == "website" {
			capability = identity.CapabilitySystemManage
		}
		if err := requireActorCapability(ctx, tx, actorID, capability); err != nil {
			return err
		}
		if asset.OwnerType == "product" {
			var state catalog.RecordState
			if err := tx.QueryRowContext(ctx, `SELECT record_state FROM products WHERE id=?`, asset.OwnerID).Scan(&state); err != nil {
				return err
			}
			if state != catalog.RecordCurrent {
				return catalog.ErrArchivedProduct
			}
		}
		asset.CreatedAt = time.Now().UTC()
		if _, err := tx.ExecContext(ctx, `INSERT INTO assets(id,owner_type,owner_id,original_filename,storage_path,mime_type,size_bytes,checksum,created_by,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			asset.ID, asset.OwnerType, asset.OwnerID, asset.OriginalFilename, asset.StoragePath, asset.MIMEType, asset.SizeBytes, asset.Checksum, actorID, asset.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "asset.created", "asset", asset.ID, map[string]any{
			"owner_type": asset.OwnerType,
			"owner_id":   asset.OwnerID,
			"mime_type":  asset.MIMEType,
			"size_bytes": asset.SizeBytes,
			"checksum":   asset.Checksum,
		})
	})
	return asset, err
}

func (s *Store) WebsiteAsset(ctx context.Context, id string) (catalog.Asset, error) {
	var asset catalog.Asset
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT id,owner_type,owner_id,original_filename,storage_path,mime_type,size_bytes,checksum,created_at
		FROM assets WHERE id=? AND owner_type='website' AND owner_id='working'`, id).Scan(
		&asset.ID, &asset.OwnerType, &asset.OwnerID, &asset.OriginalFilename, &asset.StoragePath,
		&asset.MIMEType, &asset.SizeBytes, &asset.Checksum, &createdAt,
	)
	if err != nil {
		return catalog.Asset{}, err
	}
	asset.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return catalog.Asset{}, err
	}
	if err := asset.Prepare(); err != nil {
		return catalog.Asset{}, errors.New("invalid stored website asset")
	}
	return asset, nil
}
