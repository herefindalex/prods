package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"prods/internal/identity"
	"prods/internal/site"
)

func (s *Store) SiteMaintenance(ctx context.Context) (site.Maintenance, error) {
	var state site.Maintenance
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT active,message,revision,COALESCE(updated_by,''),updated_at
		FROM site_maintenance WHERE singleton=1`).Scan(&active, &state.Message, &state.Revision, &state.UpdatedBy, &state.UpdatedAt)
	state.Active = active != 0
	return state, err
}

func (s *Store) UpdateSiteMaintenance(ctx context.Context, actorID string, expectedRevision int64, active bool, message string) (site.Maintenance, error) {
	candidate := site.Maintenance{Active: active, Message: message, Revision: expectedRevision}
	if strings.TrimSpace(actorID) == "" || candidate.Prepare() != nil {
		return site.Maintenance{}, site.ErrInvalidMaintenance
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		var current site.Maintenance
		var currentActive int
		if err := tx.QueryRowContext(ctx, `SELECT active,message,revision,COALESCE(updated_by,''),updated_at
			FROM site_maintenance WHERE singleton=1`).Scan(&currentActive, &current.Message, &current.Revision,
			&current.UpdatedBy, &current.UpdatedAt); err != nil {
			return err
		}
		current.Active = currentActive != 0
		if current.Revision != expectedRevision {
			return site.ErrMaintenanceConflict
		}
		if current.Active == candidate.Active && current.Message == candidate.Message {
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE site_maintenance SET
			active=?,message=?,revision=revision+1,updated_by=?,updated_at=?
			WHERE singleton=1 AND revision=?`, candidate.Active, candidate.Message, actorID, now, expectedRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return site.ErrMaintenanceConflict
		}
		return appendAudit(ctx, tx, actorID, "site.maintenance_updated", "site", "public", map[string]any{
			"active": candidate.Active, "revision": expectedRevision + 1,
		})
	})
	if err != nil {
		return site.Maintenance{}, err
	}
	return s.SiteMaintenance(ctx)
}
