package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"prods/internal/identity"
	"prods/internal/inquiries"
)

func (s *Store) ListRFQRecipientUsers(ctx context.Context) ([]inquiries.RecipientUserOption, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,email,display_name FROM users WHERE status='active' ORDER BY display_name,email,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []inquiries.RecipientUserOption
	for rows.Next() {
		var user inquiries.RecipientUserOption
		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) RFQRecipientSettings(ctx context.Context) (inquiries.RecipientSettings, error) {
	var settings inquiries.RecipientSettings
	err := s.db.QueryRowContext(ctx, `SELECT revision,COALESCE(updated_by,''),updated_at
		FROM rfq_recipient_settings WHERE singleton=1`).Scan(&settings.Revision, &settings.UpdatedBy, &settings.UpdatedAt)
	if err != nil {
		return inquiries.RecipientSettings{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.kind,COALESCE(r.user_id,''),
		CASE WHEN r.kind='user' THEN COALESCE(u.email,'') ELSE r.email END,
		COALESCE(u.display_name,'')
		FROM rfq_default_recipients r LEFT JOIN users u ON u.id=r.user_id
		ORDER BY r.recipient_index`)
	if err != nil {
		return inquiries.RecipientSettings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var recipient inquiries.Recipient
		if err := rows.Scan(&recipient.Kind, &recipient.UserID, &recipient.Email, &recipient.DisplayName); err != nil {
			return inquiries.RecipientSettings{}, err
		}
		settings.Recipients = append(settings.Recipients, recipient)
	}
	return settings, rows.Err()
}

func (s *Store) UpdateRFQRecipientSettings(ctx context.Context, actorID string, expectedRevision int64, recipients []inquiries.Recipient) (inquiries.RecipientSettings, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" || expectedRevision < 1 {
		return inquiries.RecipientSettings{}, inquiries.ErrInvalidRecipient
	}
	normalized, err := inquiries.NormalizeRecipients(recipients)
	if err != nil {
		return inquiries.RecipientSettings{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityRFQManage); err != nil {
			return err
		}
		if err := validateRecipientUsers(ctx, tx, normalized); err != nil {
			return err
		}
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT revision FROM rfq_recipient_settings WHERE singleton=1`).Scan(&revision); err != nil {
			return err
		}
		if revision != expectedRevision {
			return inquiries.ErrRFQRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rfq_default_recipients`); err != nil {
			return err
		}
		for index, recipient := range normalized {
			if _, err := tx.ExecContext(ctx, `INSERT INTO rfq_default_recipients(
				recipient_index,kind,user_id,email
			) VALUES(?,?,?,?)`, index, recipient.Kind, nullable(recipient.UserID), recipient.Email); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE rfq_recipient_settings
			SET revision=revision+1,updated_by=?,updated_at=? WHERE singleton=1 AND revision=?`, actorID, now, expectedRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return inquiries.ErrRFQRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "rfq.default_recipients_updated", "rfq_settings", "default_recipients", map[string]any{
			"recipient_count": len(normalized), "revision": expectedRevision + 1,
			"delivery_created": false, "authorization_granted": false,
		})
	})
	if err != nil {
		return inquiries.RecipientSettings{}, err
	}
	return s.RFQRecipientSettings(ctx)
}

func (s *Store) RFQ(ctx context.Context, id string) (RFQSummary, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return RFQSummary{}, inquiries.ErrRFQNotFound
	}
	items, err := s.ListRFQs(ctx)
	if err != nil {
		return RFQSummary{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return RFQSummary{}, inquiries.ErrRFQNotFound
}

func (s *Store) UpdateRFQStatus(ctx context.Context, actorID, id string, expectedRevision int64, next inquiries.Status) (RFQSummary, error) {
	actorID = strings.TrimSpace(actorID)
	id = strings.TrimSpace(id)
	if actorID == "" || id == "" || expectedRevision < 1 || !inquiries.ValidStatus(next) {
		return RFQSummary{}, inquiries.ErrInvalidRFQStatus
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityRFQManage); err != nil {
			return err
		}
		var current inquiries.Status
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT status,revision FROM rfqs WHERE id=?`, id).Scan(&current, &revision); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return inquiries.ErrRFQNotFound
			}
			return err
		}
		if revision != expectedRevision {
			return inquiries.ErrRFQRevisionConflict
		}
		if !inquiries.CanTransition(current, next) {
			return inquiries.ErrInvalidTransition
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE rfqs
			SET status=?,revision=revision+1,updated_by=?,updated_at=?
			WHERE id=? AND revision=?`, next, actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return inquiries.ErrRFQRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "rfq.status_updated", "rfq", id, map[string]any{
			"from": current, "to": next, "revision": expectedRevision + 1,
		})
	})
	if err != nil {
		return RFQSummary{}, err
	}
	return s.RFQ(ctx, id)
}

func (s *Store) ReplaceRFQRecipients(ctx context.Context, actorID, id string, expectedRevision int64, recipients []inquiries.Recipient) (RFQSummary, error) {
	actorID = strings.TrimSpace(actorID)
	id = strings.TrimSpace(id)
	if actorID == "" || id == "" || expectedRevision < 1 {
		return RFQSummary{}, inquiries.ErrInvalidRecipient
	}
	normalized, err := inquiries.NormalizeRecipients(recipients)
	if err != nil {
		return RFQSummary{}, err
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityRFQManage); err != nil {
			return err
		}
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT revision FROM rfqs WHERE id=?`, id).Scan(&revision); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return inquiries.ErrRFQNotFound
			}
			return err
		}
		if revision != expectedRevision {
			return inquiries.ErrRFQRevisionConflict
		}
		if err := validateRecipientUsers(ctx, tx, normalized); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rfq_recipients WHERE rfq_id=?`, id); err != nil {
			return err
		}
		for index, recipient := range normalized {
			if _, err := tx.ExecContext(ctx, `INSERT INTO rfq_recipients(
				rfq_id,recipient_index,kind,user_id,email
			) VALUES(?,?,?,?,?)`, id, index, recipient.Kind, nullable(recipient.UserID), recipient.Email); err != nil {
				return err
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE rfqs
			SET revision=revision+1,updated_by=?,updated_at=? WHERE id=? AND revision=?`,
			actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return inquiries.ErrRFQRevisionConflict
		}
		userCount, emailCount := 0, 0
		for _, recipient := range normalized {
			if recipient.Kind == inquiries.RecipientUser {
				userCount++
			} else {
				emailCount++
			}
		}
		return appendAudit(ctx, tx, actorID, "rfq.recipients_replaced", "rfq", id, map[string]any{
			"user_count": userCount, "email_count": emailCount, "revision": expectedRevision + 1,
			"delivery_created": false, "authorization_granted": false,
		})
	})
	if err != nil {
		return RFQSummary{}, err
	}
	return s.RFQ(ctx, id)
}

// AnonymizeRFQ removes locally retained RFQ personal/free-form content while
// preserving the workflow identity, Product references, immutable Admin Log,
// and non-personal delivery outcomes. It deliberately does not claim to
// retract bytes already sent by SMTP or data present in older backups.
func (s *Store) AnonymizeRFQ(ctx context.Context, actorID, id string, expectedRevision int64) (RFQSummary, error) {
	actorID = strings.TrimSpace(actorID)
	id = strings.TrimSpace(id)
	if actorID == "" || id == "" || expectedRevision < 1 {
		return RFQSummary{}, inquiries.ErrRFQNotFound
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityRFQManage); err != nil {
			return err
		}
		var revision int64
		var privacyState string
		if err := tx.QueryRowContext(ctx, `SELECT revision,privacy_state FROM rfqs WHERE id=?`, id).
			Scan(&revision, &privacyState); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return inquiries.ErrRFQNotFound
			}
			return err
		}
		if revision != expectedRevision {
			return inquiries.ErrRFQRevisionConflict
		}
		if privacyState == "anonymized" {
			return nil
		}
		var sending int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)
			FROM smtp_delivery_recipients r JOIN smtp_delivery_attempts a ON a.id=r.attempt_id
			WHERE a.rfq_id=? AND r.status='sending'`, id).Scan(&sending); err != nil {
			return err
		}
		if sending != 0 {
			return inquiries.ErrRFQDeliveryInFlight
		}

		rows, err := tx.QueryContext(ctx, `SELECT id FROM smtp_delivery_attempts WHERE rfq_id=? ORDER BY id`, id)
		if err != nil {
			return err
		}
		var attemptIDs []string
		for rows.Next() {
			var attemptID string
			if err := rows.Scan(&attemptID); err != nil {
				rows.Close()
				return err
			}
			attemptIDs = append(attemptIDs, attemptID)
		}
		if err := rows.Close(); err != nil {
			return err
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_recipients SET
			email='',
			status=CASE WHEN status='pending' THEN 'failed' ELSE status END,
			error_class=CASE WHEN status='pending' THEN 'privacy_anonymized' ELSE error_class END,
			error_message='',
			completed_at=CASE WHEN status='pending' THEN ? ELSE completed_at END
			WHERE attempt_id IN (SELECT id FROM smtp_delivery_attempts WHERE rfq_id=?)`, now, id); err != nil {
			return err
		}
		placeholder := `{"anonymized":true}`
		digest := sha256.Sum256([]byte(placeholder))
		for _, attemptID := range attemptIDs {
			status, err := aggregateDeliveryStatus(ctx, tx, attemptID)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_attempts SET
				content_hash=?,content_json=?,subject=?,body=?,status=?,updated_at=? WHERE id=?`,
				hex.EncodeToString(digest[:]), placeholder, "Prods RFQ "+id+" (anonymized)",
				"RFQ personal data was anonymized.", status, now, attemptID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM rfq_recipients WHERE rfq_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE rfq_items
			SET requested='',raw_query='',quantity='',notes='' WHERE rfq_id=?`, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE idempotency_receipts
			SET request_hash='anonymized' WHERE rfq_id=?`, id); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE rfqs SET
			name='',email='',company='',phone='',country='',general_message='',
			privacy_state='anonymized',privacy_processed_at=?,privacy_processed_by=?,
			revision=revision+1,updated_by=?,updated_at=?
			WHERE id=? AND revision=? AND privacy_state='retained'`, now, actorID, actorID, now, id, expectedRevision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return inquiries.ErrRFQRevisionConflict
		}
		return appendAudit(ctx, tx, actorID, "rfq.personal_data_anonymized", "rfq", id, map[string]any{
			"revision": expectedRevision + 1, "delivery_attempts_scrubbed": len(attemptIDs),
			"external_copies_recalled": false,
		})
	})
	if err != nil {
		return RFQSummary{}, err
	}
	return s.RFQ(ctx, id)
}

func validateRecipientUsers(ctx context.Context, tx *sql.Tx, recipients []inquiries.Recipient) error {
	for _, recipient := range recipients {
		if recipient.Kind != inquiries.RecipientUser {
			continue
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id=? AND status='active'`, recipient.UserID).Scan(&active); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return inquiries.ErrInvalidRecipient
			}
			return err
		}
	}
	return nil
}
