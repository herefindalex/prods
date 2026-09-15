package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"prods/internal/identity"
	"prods/internal/inquiries"
)

func (s *Store) CreateSMTPDeliveryAttempt(ctx context.Context, actorID, rfqID string, expectedRevision int64, deliveryKey string) (inquiries.DeliveryAttempt, error) {
	actorID = strings.TrimSpace(actorID)
	rfqID = strings.TrimSpace(rfqID)
	deliveryKey = strings.TrimSpace(deliveryKey)
	if actorID == "" || rfqID == "" || expectedRevision < 1 || len(deliveryKey) < 24 {
		return inquiries.DeliveryAttempt{}, inquiries.ErrInvalidDelivery
	}
	var attemptID string
	replayed := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityRFQManage); err != nil {
			return err
		}
		var existingRFQID string
		var existingRevision int64
		err := tx.QueryRowContext(ctx, `SELECT id,rfq_id,rfq_revision FROM smtp_delivery_attempts WHERE delivery_key=?`, deliveryKey).
			Scan(&attemptID, &existingRFQID, &existingRevision)
		if err == nil {
			if existingRFQID != rfqID || existingRevision != expectedRevision {
				return inquiries.ErrDeliveryConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		content := inquiries.DeliveryContent{Version: inquiries.DeliveryContentVersion, RFQID: rfqID}
		var revision int64
		var privacyState string
		if err := tx.QueryRowContext(ctx, `SELECT name,email,company,phone,country,general_message,revision,privacy_state
			FROM rfqs WHERE id=?`, rfqID).Scan(&content.Name, &content.Email, &content.Company,
			&content.Phone, &content.Country, &content.GeneralMessage, &revision, &privacyState); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return inquiries.ErrRFQNotFound
			}
			return err
		}
		if revision != expectedRevision {
			return inquiries.ErrRFQRevisionConflict
		}
		if privacyState == "anonymized" {
			return inquiries.ErrRFQAnonymized
		}
		itemRows, err := tx.QueryContext(ctx, `SELECT kind,COALESCE(product_id,''),requested,raw_query,quantity,notes,
			COALESCE(public_snapshot_json,'') FROM rfq_items WHERE rfq_id=? ORDER BY item_index`, rfqID)
		if err != nil {
			return err
		}
		for itemRows.Next() {
			var item inquiries.DeliveryItem
			if err := itemRows.Scan(&item.Kind, &item.ProductID, &item.Requested, &item.RawQuery,
				&item.Quantity, &item.Notes, &item.PublicSnapshotJSON); err != nil {
				itemRows.Close()
				return err
			}
			content.Items = append(content.Items, item)
		}
		if err := itemRows.Close(); err != nil {
			return err
		}
		if len(content.Items) == 0 {
			return inquiries.ErrInvalidDelivery
		}

		recipientRows, err := tx.QueryContext(ctx, `SELECT r.recipient_index,r.kind,COALESCE(r.user_id,''),
			CASE WHEN r.kind='user' THEN COALESCE(u.email,'') ELSE r.email END
			FROM rfq_recipients r LEFT JOIN users u ON u.id=r.user_id
			WHERE r.rfq_id=? ORDER BY r.recipient_index`, rfqID)
		if err != nil {
			return err
		}
		var recipients []inquiries.DeliveryRecipient
		for recipientRows.Next() {
			var recipient inquiries.DeliveryRecipient
			if err := recipientRows.Scan(&recipient.Index, &recipient.Kind, &recipient.SourceUserID, &recipient.Email); err != nil {
				recipientRows.Close()
				return err
			}
			if strings.TrimSpace(recipient.Email) == "" {
				recipientRows.Close()
				return inquiries.ErrInvalidRecipient
			}
			recipient.Status = inquiries.DeliveryPending
			recipients = append(recipients, recipient)
		}
		if err := recipientRows.Close(); err != nil {
			return err
		}
		if len(recipients) == 0 {
			return inquiries.ErrNoDeliveryRecipient
		}

		encoded, err := json.Marshal(content)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(encoded)
		subject, body := inquiries.RenderDelivery(content)
		attemptID, err = randomID("mail")
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO smtp_delivery_attempts(
			id,delivery_key,rfq_id,rfq_revision,content_version,content_hash,content_json,subject,body,status,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,'pending',?,?,?)`, attemptID, deliveryKey, rfqID, revision,
			inquiries.DeliveryContentVersion, hex.EncodeToString(digest[:]), string(encoded), subject, body, actorID, now, now); err != nil {
			return err
		}
		for _, recipient := range recipients {
			if _, err := tx.ExecContext(ctx, `INSERT INTO smtp_delivery_recipients(
				attempt_id,recipient_index,kind,source_user_id,email,status
			) VALUES(?,?,?,?,?,'pending')`, attemptID, recipient.Index, recipient.Kind,
				nullable(recipient.SourceUserID), recipient.Email); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, actorID, "rfq.delivery_requested", "rfq", rfqID, map[string]any{
			"attempt_id": attemptID, "rfq_revision": revision, "content_hash": hex.EncodeToString(digest[:]),
			"recipient_count": len(recipients), "automatic": false,
		})
	})
	if err != nil {
		return inquiries.DeliveryAttempt{}, err
	}
	attempt, err := s.SMTPDeliveryAttempt(ctx, attemptID)
	if err != nil {
		return inquiries.DeliveryAttempt{}, err
	}
	attempt.Replay = replayed
	return attempt, nil
}

func (s *Store) ListSMTPDeliveryAttempts(ctx context.Context, rfqID string) ([]inquiries.DeliveryAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM smtp_delivery_attempts WHERE rfq_id=? ORDER BY created_at DESC,id DESC`, strings.TrimSpace(rfqID))
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	attempts := make([]inquiries.DeliveryAttempt, 0, len(ids))
	for _, id := range ids {
		attempt, err := s.SMTPDeliveryAttempt(ctx, id)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, nil
}

func (s *Store) SMTPDeliveryAttempt(ctx context.Context, id string) (inquiries.DeliveryAttempt, error) {
	var attempt inquiries.DeliveryAttempt
	err := s.db.QueryRowContext(ctx, `SELECT id,rfq_id,rfq_revision,content_version,content_hash,status,created_by,created_at,updated_at
		FROM smtp_delivery_attempts WHERE id=?`, id).Scan(&attempt.ID, &attempt.RFQID, &attempt.RFQRevision,
		&attempt.ContentVersion, &attempt.ContentHash, &attempt.Status, &attempt.CreatedBy, &attempt.CreatedAt, &attempt.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return inquiries.DeliveryAttempt{}, inquiries.ErrInvalidDelivery
	}
	if err != nil {
		return inquiries.DeliveryAttempt{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT recipient_index,kind,COALESCE(source_user_id,''),email,status,error_class,error_message,
		COALESCE(started_at,''),COALESCE(completed_at,'') FROM smtp_delivery_recipients
		WHERE attempt_id=? ORDER BY recipient_index`, id)
	if err != nil {
		return inquiries.DeliveryAttempt{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var recipient inquiries.DeliveryRecipient
		if err := rows.Scan(&recipient.Index, &recipient.Kind, &recipient.SourceUserID, &recipient.Email,
			&recipient.Status, &recipient.ErrorClass, &recipient.ErrorMessage, &recipient.StartedAt, &recipient.CompletedAt); err != nil {
			return inquiries.DeliveryAttempt{}, err
		}
		attempt.Recipients = append(attempt.Recipients, recipient)
	}
	return attempt, rows.Err()
}

func (s *Store) ClaimSMTPDeliveryRecipient(ctx context.Context) (inquiries.DeliveryWork, bool, error) {
	var work inquiries.DeliveryWork
	found := false
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT r.attempt_id,r.recipient_index,r.email,a.subject,a.body
			FROM smtp_delivery_recipients r JOIN smtp_delivery_attempts a ON a.id=r.attempt_id
			WHERE r.status='pending' ORDER BY a.created_at,a.id,r.recipient_index LIMIT 1`).
			Scan(&work.AttemptID, &work.RecipientIndex, &work.To, &work.Subject, &work.Body)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_recipients
			SET status='sending',started_at=?,error_class='',error_message=''
			WHERE attempt_id=? AND recipient_index=? AND status='pending'`, now, work.AttemptID, work.RecipientIndex)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return inquiries.ErrDeliveryConflict
		}
		if _, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_attempts SET status=CASE
			WHEN EXISTS(SELECT 1 FROM smtp_delivery_recipients WHERE attempt_id=? AND status='unknown') THEN 'unknown'
			ELSE 'sending' END,updated_at=? WHERE id=?`, work.AttemptID, now, work.AttemptID); err != nil {
			return err
		}
		found = true
		return nil
	})
	return work, found, err
}

func (s *Store) CompleteSMTPDeliveryRecipient(ctx context.Context, attemptID string, recipientIndex int, outcome inquiries.DeliveryStatus, errorClass, errorMessage string) error {
	if outcome != inquiries.DeliveryAccepted && outcome != inquiries.DeliveryFailed && outcome != inquiries.DeliveryUnknown {
		return inquiries.ErrInvalidDelivery
	}
	if len(errorClass) > 100 {
		errorClass = errorClass[:100]
	}
	if len(errorMessage) > 500 {
		errorMessage = errorMessage[:500]
	}
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_recipients
			SET status=?,error_class=?,error_message=?,completed_at=?
			WHERE attempt_id=? AND recipient_index=? AND status='sending'`, outcome,
			strings.TrimSpace(errorClass), strings.TrimSpace(errorMessage), now, attemptID, recipientIndex)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return inquiries.ErrDeliveryConflict
		}
		status, err := aggregateDeliveryStatus(ctx, tx, attemptID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE smtp_delivery_attempts SET status=?,updated_at=? WHERE id=?`, status, now, attemptID)
		return err
	})
}

func (s *Store) ReconcileSMTPDeliveries(ctx context.Context) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_recipients
			SET status='unknown',error_class='process_restart',error_message='delivery outcome unknown after process restart',completed_at=?
			WHERE status='sending'`, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE smtp_delivery_attempts SET status='unknown',updated_at=?
			WHERE EXISTS(SELECT 1 FROM smtp_delivery_recipients r WHERE r.attempt_id=smtp_delivery_attempts.id AND r.status='unknown')`, now)
		return err
	})
}

func aggregateDeliveryStatus(ctx context.Context, tx *sql.Tx, attemptID string) (inquiries.DeliveryStatus, error) {
	counts := map[inquiries.DeliveryStatus]int{}
	rows, err := tx.QueryContext(ctx, `SELECT status,COUNT(*) FROM smtp_delivery_recipients WHERE attempt_id=? GROUP BY status`, attemptID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var status inquiries.DeliveryStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return "", err
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if counts[inquiries.DeliveryUnknown] > 0 {
		return inquiries.DeliveryUnknown, nil
	}
	if counts[inquiries.DeliveryPending]+counts[inquiries.DeliverySending] > 0 {
		return inquiries.DeliverySending, nil
	}
	if counts[inquiries.DeliveryFailed] > 0 && counts[inquiries.DeliveryAccepted] > 0 {
		return inquiries.DeliveryPartial, nil
	}
	if counts[inquiries.DeliveryFailed] > 0 {
		return inquiries.DeliveryFailed, nil
	}
	return inquiries.DeliveryAccepted, nil
}
