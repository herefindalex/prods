package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"prods/internal/catalog"
	"prods/internal/identity"
)

var ErrInvalidTrafficSettings = errors.New("invalid traffic settings")

type TrafficSettings struct {
	RFQLimit         int    `json:"rfq_limit"`
	RFQWindowSeconds int    `json:"rfq_window_seconds"`
	Version          int64  `json:"version"`
	UpdatedAt        string `json:"updated_at"`
}

func (s *Store) TrafficSettings(ctx context.Context) (TrafficSettings, error) {
	var settings TrafficSettings
	err := s.db.QueryRowContext(ctx, `SELECT rfq_limit,rfq_window_seconds,version,updated_at
		FROM traffic_settings WHERE singleton=1`).Scan(
		&settings.RFQLimit, &settings.RFQWindowSeconds, &settings.Version, &settings.UpdatedAt,
	)
	return settings, err
}

func (s *Store) UpdateTrafficSettings(ctx context.Context, actorID string, expectedVersion int64, next TrafficSettings) (TrafficSettings, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || expectedVersion < 1 || !validTrafficSettings(next) {
		return TrafficSettings{}, ErrInvalidTrafficSettings
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilitySystemManage); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE traffic_settings
			SET rfq_limit=?,rfq_window_seconds=?,version=version+1,updated_at=?
			WHERE singleton=1 AND version=?`,
			next.RFQLimit, next.RFQWindowSeconds, time.Now().UTC().Format(time.RFC3339Nano), expectedVersion)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return catalog.ErrRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "traffic.settings_updated", "traffic_settings", "singleton", map[string]any{
			"rfq_limit": next.RFQLimit, "rfq_window_seconds": next.RFQWindowSeconds,
		})
	})
	if err != nil {
		return TrafficSettings{}, err
	}
	return s.TrafficSettings(ctx)
}

func validTrafficSettings(settings TrafficSettings) bool {
	return settings.RFQLimit >= 1 && settings.RFQLimit <= 10000 &&
		settings.RFQWindowSeconds >= 1 && settings.RFQWindowSeconds <= 86400
}
