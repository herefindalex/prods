package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"prods/internal/identity"
)

var (
	ErrInvitationMailInFlight = errors.New("an invitation email send is already in flight for this grant")
	ErrInvitationMailUnknown  = errors.New("the prior invitation email outcome is unknown; issue a new grant before sending again")
)

func (s *Store) CreateUserInvitationMailAttempt(ctx context.Context, actorID, userID, rawToken string) (identity.InvitationMailAttempt, error) {
	if actorID == "" || userID == "" || rawToken == "" {
		return identity.InvitationMailAttempt{}, ErrInvalidGrant
	}
	attemptID, err := randomID("uml")
	if err != nil {
		return identity.InvitationMailAttempt{}, err
	}
	now := time.Now().UTC()
	attempt := identity.InvitationMailAttempt{
		ID:             attemptID,
		UserID:         userID,
		ContentVersion: identity.InvitationMailContentVersion,
		Status:         identity.InvitationMailPending,
		CreatedAt:      now.Format(time.RFC3339Nano),
	}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityUsersManage); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT u.email,u.auth_revision
			FROM users u JOIN set_password_tokens t ON t.user_id=u.id
			WHERE u.id=? AND u.status='active' AND t.token_digest=? AND t.used_at IS NULL
			AND t.expires_at>? AND t.auth_revision=u.auth_revision`, userID, identity.TokenDigest(rawToken), attempt.CreatedAt).Scan(
			&attempt.RecipientEmail, &attempt.AuthRevision,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidGrant
			}
			return err
		}
		var priorStatus identity.InvitationMailStatus
		err := tx.QueryRowContext(ctx, `SELECT status FROM user_invitation_mail_attempts
			WHERE token_digest=? AND status IN ('pending','unknown') ORDER BY created_at DESC,id DESC LIMIT 1`,
			identity.TokenDigest(rawToken)).Scan(&priorStatus)
		switch {
		case err == nil && priorStatus == identity.InvitationMailPending:
			return ErrInvitationMailInFlight
		case err == nil && priorStatus == identity.InvitationMailUnknown:
			return ErrInvitationMailUnknown
		case err != nil && !errors.Is(err, sql.ErrNoRows):
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_invitation_mail_attempts(
			id,user_id,auth_revision,token_digest,recipient_email,content_version,status,error_class,error_message,created_by,created_at,completed_at
		) VALUES(?,?,?,?,?,?,'pending','','',?,?,NULL)`, attempt.ID, attempt.UserID, attempt.AuthRevision,
			identity.TokenDigest(rawToken), attempt.RecipientEmail, attempt.ContentVersion, actorID, attempt.CreatedAt); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "user.invitation_mail_requested", "user_invitation_mail", attempt.ID, map[string]any{
			"user_id":         attempt.UserID,
			"auth_revision":   attempt.AuthRevision,
			"recipient_email": attempt.RecipientEmail,
			"content_version": attempt.ContentVersion,
		})
	})
	return attempt, err
}

func (s *Store) CompleteUserInvitationMailAttempt(ctx context.Context, attemptID string, status identity.InvitationMailStatus, errorClass, errorMessage string) (identity.InvitationMailAttempt, error) {
	if attemptID == "" || (status != identity.InvitationMailAccepted && status != identity.InvitationMailFailed && status != identity.InvitationMailUnknown) {
		return identity.InvitationMailAttempt{}, ErrInvalidGrant
	}
	errorClass = boundedText(errorClass, 160)
	errorMessage = boundedText(errorMessage, 1000)
	var attempt identity.InvitationMailAttempt
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var createdBy string
		if err := tx.QueryRowContext(ctx, `SELECT id,user_id,auth_revision,recipient_email,content_version,status,created_at,created_by
			FROM user_invitation_mail_attempts WHERE id=?`, attemptID).Scan(
			&attempt.ID, &attempt.UserID, &attempt.AuthRevision, &attempt.RecipientEmail, &attempt.ContentVersion,
			&attempt.Status, &attempt.CreatedAt, &createdBy,
		); err != nil {
			return err
		}
		if attempt.Status != identity.InvitationMailPending {
			return ErrInvalidGrant
		}
		completedAt := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `UPDATE user_invitation_mail_attempts
			SET status=?,error_class=?,error_message=?,completed_at=? WHERE id=? AND status='pending'`,
			status, errorClass, errorMessage, completedAt, attempt.ID)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return ErrInvalidGrant
		}
		attempt.Status = status
		attempt.ErrorClass = errorClass
		attempt.ErrorMessage = errorMessage
		attempt.CompletedAt = completedAt
		return appendAudit(ctx, tx, createdBy, "user.invitation_mail_completed", "user_invitation_mail", attempt.ID, map[string]any{
			"user_id": attempt.UserID,
			"status":  status,
		})
	})
	return attempt, err
}

func (s *Store) ReconcileUserInvitationMailAttempts(ctx context.Context) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id,user_id FROM user_invitation_mail_attempts WHERE status='pending' ORDER BY created_at,id`)
		if err != nil {
			return err
		}
		var pending [][2]string
		for rows.Next() {
			var item [2]string
			if err := rows.Scan(&item[0], &item[1]); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, item)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, item := range pending {
			if _, err := tx.ExecContext(ctx, `UPDATE user_invitation_mail_attempts
				SET status='unknown',error_class='restart_ambiguous',error_message='process restarted before durable SMTP completion',completed_at=?
				WHERE id=? AND status='pending'`, now, item[0]); err != nil {
				return err
			}
			if err := appendAudit(ctx, tx, "", "user.invitation_mail_reconciled_unknown", "user_invitation_mail", item[0], map[string]any{
				"user_id": item[1],
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListUserInvitationMailAttempts(ctx context.Context, userID string, limit int) ([]identity.InvitationMailAttempt, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,auth_revision,recipient_email,content_version,status,error_class,error_message,created_at,COALESCE(completed_at,'')
		FROM user_invitation_mail_attempts WHERE user_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var attempts []identity.InvitationMailAttempt
	for rows.Next() {
		var attempt identity.InvitationMailAttempt
		if err := rows.Scan(&attempt.ID, &attempt.UserID, &attempt.AuthRevision, &attempt.RecipientEmail, &attempt.ContentVersion,
			&attempt.Status, &attempt.ErrorClass, &attempt.ErrorMessage, &attempt.CreatedAt, &attempt.CompletedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func boundedText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
