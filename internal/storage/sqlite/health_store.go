package sqlite

import (
	"context"

	"prods/internal/catalog"
)

// OperationalHealth contains durable workload and publication state used by
// the privileged System Health view. Historical completed jobs are excluded:
// the counters describe work that can affect the current runtime state.
type OperationalHealth struct {
	PublicationPending    int64 `json:"publication_pending"`
	PublicationProcessing int64 `json:"publication_processing"`
	PublicationFailed     int64 `json:"publication_failed"`
	PublicationDirty      int64 `json:"publication_dirty"`
	ActiveImports         int64 `json:"active_imports"`
	RunningBackups        int64 `json:"running_backups"`
	ActivePublicProducts  int64 `json:"active_public_products"`
	SearchProjectionDrift int64 `json:"search_projection_drift"`
	AssetOrphans          int64 `json:"asset_orphans"`
	AssetDeletionPending  int64 `json:"asset_deletion_pending"`
	AssetDeletionFailed   int64 `json:"asset_deletion_failed"`
}

func (s *Store) OperationalHealth(ctx context.Context) (OperationalHealth, error) {
	var health OperationalHealth
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM publication_intents WHERE status='pending'),
			(SELECT COUNT(*) FROM publication_intents WHERE status='processing'),
			(SELECT COUNT(*) FROM publication_intents WHERE status='failed'),
			(SELECT COUNT(*) FROM publication_dirty),
			(SELECT COUNT(*) FROM import_jobs WHERE status IN ('queued','parsing','committing')),
			(SELECT COUNT(*) FROM backup_runs WHERE status='running'),
			(SELECT COUNT(*) FROM public_activations),
			(SELECT COUNT(*)
			 FROM public_activations a
			 LEFT JOIN public_search_projection p ON p.product_id=a.product_id
			 WHERE p.product_id IS NULL
			    OR p.source_revision<>a.source_revision
			    OR p.site_epoch<>a.site_epoch
				    OR p.projection_version<>?),
			(SELECT COUNT(*) FROM asset_gc_orphans),
			(SELECT COUNT(*) FROM asset_gc_deletions WHERE state='planned'),
			(SELECT COUNT(*) FROM asset_gc_deletions WHERE state='failed')
	`, catalog.SearchProjectionVersion).Scan(
		&health.PublicationPending,
		&health.PublicationProcessing,
		&health.PublicationFailed,
		&health.PublicationDirty,
		&health.ActiveImports,
		&health.RunningBackups,
		&health.ActivePublicProducts,
		&health.SearchProjectionDrift,
		&health.AssetOrphans,
		&health.AssetDeletionPending,
		&health.AssetDeletionFailed,
	)
	return health, err
}
