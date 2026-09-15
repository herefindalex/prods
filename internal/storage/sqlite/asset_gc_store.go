package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"prods/internal/publishing"
	"prods/internal/site"
)

// AssetFileDeletion is durable work left after an orphaned asset's metadata
// has been removed. The physical file remains private and is retried until the
// deletion is recorded complete.
type AssetFileDeletion struct {
	AssetID     string
	StoragePath string
	State       string
	LastError   string
}

// SnapshotWithAssetPins creates one consistent database snapshot and pins the
// exact asset set visible in that snapshot before asset GC may proceed.
func (s *Store) SnapshotWithAssetPins(ctx context.Context, destination, operationID string) error {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return errors.New("asset snapshot operation ID is required")
	}

	s.assetLifecycle.Lock()
	defer s.assetLifecycle.Unlock()

	available, err := s.assetLifecycleTablesAvailable(ctx)
	if err != nil {
		return err
	}
	if !available {
		// Pre-v9 databases are backed up while the process is offline and no GC
		// worker can run. Their pre-upgrade snapshot therefore needs no pin row.
		return s.Snapshot(ctx, destination)
	}

	if err := s.Snapshot(ctx, destination); err != nil {
		return err
	}

	snapshot, err := sql.Open("sqlite", "file:"+filepath.ToSlash(destination)+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return fmt.Errorf("open asset snapshot: %w", err)
	}
	defer snapshot.Close()

	rows, err := snapshot.QueryContext(ctx, `SELECT id FROM assets ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read snapshot assets: %w", err)
	}
	var assetIDs []string
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			rows.Close()
			return err
		}
		assetIDs = append(assetIDs, assetID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		for _, assetID := range assetIDs {
			result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO asset_backup_pins(operation_id,asset_id,created_at)
VALUES(?,?,?)`, operationID, assetID, now)
			if err != nil {
				return err
			}
			if changed, err := result.RowsAffected(); err != nil || changed != 1 {
				if err != nil {
					return err
				}
				return fmt.Errorf("asset %s disappeared before backup pin", assetID)
			}
		}
		return nil
	})
}

func (s *Store) ReleaseAssetPins(ctx context.Context, operationID string) error {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return errors.New("asset snapshot operation ID is required")
	}
	available, err := s.assetLifecycleTablesAvailable(ctx)
	if err != nil || !available {
		return err
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM asset_backup_pins WHERE operation_id=?`, operationID)
		return err
	})
}

// ClearAssetPins is safe only during single-instance startup, before any new
// backup can begin. Pins left by a terminated process cannot protect resumable
// work because incomplete backups are never published.
func (s *Store) ClearAssetPins(ctx context.Context) error {
	available, err := s.assetLifecycleTablesAvailable(ctx)
	if err != nil || !available {
		return err
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM asset_backup_pins`)
		return err
	})
}

func (s *Store) assetLifecycleTablesAvailable(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
WHERE type='table' AND name IN ('asset_gc_orphans','asset_backup_pins','asset_gc_deletions')`).Scan(&count)
	return count == 3, err
}

// PlanAssetGarbageCollection marks newly orphaned assets, clears recovered
// markers, and atomically converts expired orphans into durable file-deletion
// work. Active references and backup pins are revalidated in the same write
// transaction that removes asset metadata.
func (s *Store) PlanAssetGarbageCollection(ctx context.Context, now time.Time, grace time.Duration, limit int) (int, int, error) {
	if grace < 0 || limit <= 0 {
		return 0, 0, errors.New("invalid asset GC limits")
	}
	now = now.UTC()
	cutoff := now.Add(-grace)

	s.assetLifecycle.Lock()
	defer s.assetLifecycle.Unlock()

	marked := 0
	planned := 0
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		protected, err := loadProtectedAssetIDs(ctx, tx)
		if err != nil {
			return err
		}

		rows, err := tx.QueryContext(ctx, `SELECT id,storage_path FROM assets ORDER BY id`)
		if err != nil {
			return err
		}
		type candidate struct {
			id   string
			path string
		}
		var assets []candidate
		for rows.Next() {
			var item candidate
			if err := rows.Scan(&item.id, &item.path); err != nil {
				rows.Close()
				return err
			}
			assets = append(assets, item)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}

		for _, asset := range assets {
			if _, ok := protected[asset.id]; ok {
				if _, err := tx.ExecContext(ctx, `DELETE FROM asset_gc_orphans WHERE asset_id=?`, asset.id); err != nil {
					return err
				}
				continue
			}

			var orphanedAtText string
			err := tx.QueryRowContext(ctx, `SELECT orphaned_at FROM asset_gc_orphans WHERE asset_id=?`, asset.id).Scan(&orphanedAtText)
			if errors.Is(err, sql.ErrNoRows) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO asset_gc_orphans(asset_id,orphaned_at) VALUES(?,?)`, asset.id, now.Format(time.RFC3339Nano)); err != nil {
					return err
				}
				marked++
				continue
			}
			if err != nil {
				return err
			}
			orphanedAt, err := time.Parse(time.RFC3339Nano, orphanedAtText)
			if err != nil {
				return fmt.Errorf("parse orphan time for %s: %w", asset.id, err)
			}
			if orphanedAt.After(cutoff) || planned >= limit {
				continue
			}

			stamp := now.Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx, `INSERT INTO asset_gc_deletions(
asset_id,storage_path,state,last_error,planned_at,updated_at,completed_at
) VALUES(?,?,'planned','',?,?,NULL)
ON CONFLICT(asset_id) DO UPDATE SET
 storage_path=excluded.storage_path,state='planned',last_error='',updated_at=excluded.updated_at,completed_at=NULL`,
				asset.id, asset.path, stamp, stamp); err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE id=?`, asset.id)
			if err != nil {
				return err
			}
			if changed, err := result.RowsAffected(); err != nil || changed != 1 {
				if err != nil {
					return err
				}
				return fmt.Errorf("asset %s was not deleted", asset.id)
			}
			planned++
		}
		return nil
	})
	return marked, planned, err
}

func (s *Store) PendingAssetFileDeletions(ctx context.Context, limit int) ([]AssetFileDeletion, error) {
	if limit <= 0 {
		return nil, errors.New("invalid asset deletion limit")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT asset_id,storage_path,state,last_error
FROM asset_gc_deletions WHERE state IN ('planned','failed') ORDER BY planned_at,asset_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AssetFileDeletion, 0)
	for rows.Next() {
		var item AssetFileDeletion
		if err := rows.Scan(&item.AssetID, &item.StoragePath, &item.State, &item.LastError); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) RecordAssetFileDeletion(ctx context.Context, assetID string, deletionErr error) error {
	assetID = strings.TrimSpace(assetID)
	if assetID == "" {
		return errors.New("asset ID is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if deletionErr == nil {
			result, err := tx.ExecContext(ctx, `UPDATE asset_gc_deletions
SET state='completed',last_error='',updated_at=?,completed_at=? WHERE asset_id=? AND state IN ('planned','failed')`, now, now, assetID)
			if err != nil {
				return err
			}
			if changed, _ := result.RowsAffected(); changed != 1 {
				return sql.ErrNoRows
			}
			return nil
		}
		message := deletionErr.Error()
		if len(message) > 1024 {
			message = message[:1024]
		}
		result, err := tx.ExecContext(ctx, `UPDATE asset_gc_deletions
SET state='failed',last_error=?,updated_at=?,completed_at=NULL WHERE asset_id=? AND state IN ('planned','failed')`, message, now, assetID)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func (s *Store) PurgeAssetDeletionHistory(ctx context.Context, before time.Time) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM asset_gc_deletions
WHERE state='completed' AND completed_at IS NOT NULL AND completed_at<?`, before.UTC().Format(time.RFC3339Nano))
		return err
	})
}

func loadProtectedAssetIDs(ctx context.Context, tx *sql.Tx) (map[string]struct{}, error) {
	protected := make(map[string]struct{})
	rows, err := tx.QueryContext(ctx, `SELECT asset_id FROM product_images WHERE asset_id IS NOT NULL
UNION SELECT asset_id FROM product_documents WHERE asset_id IS NOT NULL
UNION SELECT asset_id FROM asset_backup_pins`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			rows.Close()
			return nil, err
		}
		protected[assetID] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	configRows, err := tx.QueryContext(ctx, `SELECT config_json FROM website_working
UNION ALL SELECT config_json FROM website_versions`)
	if err != nil {
		return nil, err
	}
	for configRows.Next() {
		var encoded string
		if err := configRows.Scan(&encoded); err != nil {
			configRows.Close()
			return nil, err
		}
		var configuration site.Configuration
		if err := json.Unmarshal([]byte(encoded), &configuration); err != nil {
			configRows.Close()
			return nil, fmt.Errorf("decode website asset references: %w", err)
		}
		for _, assetID := range configuration.AssetIDs() {
			protected[assetID] = struct{}{}
		}
	}
	if err := configRows.Close(); err != nil {
		return nil, err
	}
	if err := configRows.Err(); err != nil {
		return nil, err
	}

	viewRows, err := tx.QueryContext(ctx, `SELECT view_json FROM public_activations`)
	if err != nil {
		return nil, err
	}
	for viewRows.Next() {
		var encoded string
		if err := viewRows.Scan(&encoded); err != nil {
			viewRows.Close()
			return nil, err
		}
		var view publishing.PublicView
		if err := json.Unmarshal([]byte(encoded), &view); err != nil {
			viewRows.Close()
			return nil, fmt.Errorf("decode active public asset references: %w", err)
		}
		for _, image := range view.Images {
			if image.AssetID != "" {
				protected[image.AssetID] = struct{}{}
			}
		}
		for _, document := range view.Documents {
			if document.AssetID != "" {
				protected[document.AssetID] = struct{}{}
			}
		}
	}
	if err := viewRows.Close(); err != nil {
		return nil, err
	}
	return protected, viewRows.Err()
}
