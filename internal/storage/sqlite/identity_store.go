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
	ErrLastActiveOwner = errors.New("at least one active Owner with a usable password must remain")
	ErrInvalidGrant    = errors.New("set-password grant is invalid, expired, or already used")
)

func (s *Store) CreateRole(ctx context.Context, actorID string, role identity.Role) (identity.Role, error) {
	role.Name = strings.TrimSpace(role.Name)
	if role.Name == "" || len(role.Capabilities) == 0 {
		return identity.Role{}, ErrPermissionDenied
	}
	if role.ID == "" {
		id, err := randomID("rol")
		if err != nil {
			return identity.Role{}, err
		}
		role.ID = id
	}
	role.Status = "active"
	role.Revision = 1
	seen := make(map[identity.Capability]bool)
	for _, capability := range role.Capabilities {
		if !identity.ValidCapability(capability) || seen[capability] {
			return identity.Role{}, ErrPermissionDenied
		}
		seen[capability] = true
	}
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityUsersManage); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO roles(id,name,system_key,status,revision,created_at,updated_at)
			VALUES(?,?,NULL,'active',1,?,?)`, role.ID, role.Name, now, now); err != nil {
			return err
		}
		for _, capability := range role.Capabilities {
			if _, err := tx.ExecContext(ctx, `INSERT INTO role_capabilities(role_id,capability) VALUES(?,?)`, role.ID, capability); err != nil {
				return err
			}
		}
		return appendAudit(ctx, tx, actorID, "role.created", "role", role.ID, map[string]any{"capabilities": role.Capabilities})
	})
	return role, err
}

func (s *Store) CreateUser(ctx context.Context, actorID, email, displayName, roleID string, ttl time.Duration) (identity.SetPasswordGrant, error) {
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if identity.NormalizeEmail(email) == "" || roleID == "" {
		return identity.SetPasswordGrant{}, ErrInvalidCredentials
	}
	if displayName == "" {
		displayName = email
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	userID, err := randomID("usr")
	if err != nil {
		return identity.SetPasswordGrant{}, err
	}
	rawToken, err := identity.NewToken(32)
	if err != nil {
		return identity.SetPasswordGrant{}, err
	}
	expiresAt := time.Now().UTC().Add(ttl)
	user := identity.User{ID: userID, Email: email, DisplayName: displayName, Role: roleID, Status: identity.UserActive, AuthRevision: 1}
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityUsersManage); err != nil {
			return err
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM roles WHERE id=? AND status='active'`, roleID).Scan(&active); err != nil {
			return ErrPermissionDenied
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(
			id,email,email_normalized,display_name,role,status,password_hash,auth_revision,created_at,updated_at
		) VALUES(?,?,?,?,?,'active',NULL,1,?,?)`, user.ID, user.Email, identity.NormalizeEmail(user.Email), user.DisplayName, user.Role, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO set_password_tokens(
			token_digest,user_id,auth_revision,expires_at,used_at,created_by,created_at
		) VALUES(?,?,?,?,NULL,?,?)`, identity.TokenDigest(rawToken), user.ID, user.AuthRevision,
			expiresAt.Format(time.RFC3339Nano), actorID, now); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "user.created", "user", user.ID, map[string]any{"role_id": roleID})
	})
	return identity.SetPasswordGrant{User: user, Token: rawToken, ExpiresAt: expiresAt}, err
}

// CreateOwnerRecoveryGrant is a host-console trust path. Callers must already
// hold exclusive database ownership; it never creates a user or grants an
// arbitrary Admin identity. The raw token is returned once and only its digest
// is persisted.
func (s *Store) CreateOwnerRecoveryGrant(ctx context.Context, email string, ttl time.Duration) (identity.SetPasswordGrant, error) {
	email = identity.NormalizeEmail(email)
	if email == "" {
		return identity.SetPasswordGrant{}, ErrInvalidCredentials
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	rawToken, err := identity.NewToken(32)
	if err != nil {
		return identity.SetPasswordGrant{}, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	var user identity.User
	err = s.withWriteTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT id,email,display_name,role,status,auth_revision
			FROM users WHERE email_normalized=? AND role=? AND status='active'`, email, identity.RoleOwner).
			Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.Status, &user.AuthRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredentials
		}
		if err != nil {
			return err
		}
		stamp := now.Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE set_password_tokens SET used_at=?
			WHERE user_id=? AND used_at IS NULL`, stamp, user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO set_password_tokens(
			token_digest,user_id,auth_revision,expires_at,used_at,created_by,created_at
		) VALUES(?,?,?,?,NULL,?,?)`, identity.TokenDigest(rawToken), user.ID, user.AuthRevision,
			expiresAt.Format(time.RFC3339Nano), nullable(""), stamp); err != nil {
			return err
		}
		return appendAudit(ctx, tx, "", "user.owner_recovery_grant_created", "user", user.ID, map[string]any{
			"source": "host_console", "expires_at": expiresAt.Format(time.RFC3339Nano),
		})
	})
	return identity.SetPasswordGrant{User: user, Token: rawToken, ExpiresAt: expiresAt}, err
}

func (s *Store) SetUserPassword(ctx context.Context, rawToken, passwordHash string, now time.Time) (identity.User, error) {
	if rawToken == "" || passwordHash == "" {
		return identity.User{}, ErrInvalidGrant
	}
	var user identity.User
	err := s.withWriteTx(ctx, func(tx *sql.Tx) error {
		var grantRevision int
		err := tx.QueryRowContext(ctx, `SELECT u.id,u.email,u.display_name,u.role,u.status,u.auth_revision,t.auth_revision
			FROM set_password_tokens t JOIN users u ON u.id=t.user_id
			WHERE t.token_digest=? AND t.used_at IS NULL AND t.expires_at>? AND u.status='active' AND t.auth_revision=u.auth_revision`,
			identity.TokenDigest(rawToken), now.UTC().Format(time.RFC3339Nano)).Scan(&user.ID, &user.Email, &user.DisplayName,
			&user.Role, &user.Status, &user.AuthRevision, &grantRevision)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidGrant
		}
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?,auth_revision=auth_revision+1,updated_at=?
			WHERE id=? AND auth_revision=? AND status='active'`, passwordHash, now.UTC().Format(time.RFC3339Nano), user.ID, grantRevision)
		if err != nil {
			return err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return ErrInvalidGrant
		}
		if _, err := tx.ExecContext(ctx, `UPDATE set_password_tokens SET used_at=? WHERE user_id=? AND used_at IS NULL`,
			now.UTC().Format(time.RFC3339Nano), user.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`,
			now.UTC().Format(time.RFC3339Nano), user.ID); err != nil {
			return err
		}
		user.AuthRevision++
		return appendAudit(ctx, tx, user.ID, "user.password_set", "user", user.ID, map[string]any{"auth_revision": user.AuthRevision})
	})
	return user, err
}

func (s *Store) DisableUser(ctx context.Context, actorID, userID string) error {
	return s.withWriteTx(ctx, func(tx *sql.Tx) error {
		if err := requireActorCapability(ctx, tx, actorID, identity.CapabilityUsersManage); err != nil {
			return err
		}
		var roleID, status string
		if err := tx.QueryRowContext(ctx, `SELECT role,status FROM users WHERE id=?`, userID).Scan(&roleID, &status); err != nil {
			return err
		}
		if status == identity.UserDisabled {
			return nil
		}
		if roleID == identity.RoleOwner {
			var otherOwners int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users
				WHERE id<>? AND role=? AND status='active' AND password_hash IS NOT NULL`, userID, identity.RoleOwner).Scan(&otherOwners); err != nil {
				return err
			}
			if otherOwners == 0 {
				return ErrLastActiveOwner
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `UPDATE users SET status='disabled',password_hash=NULL,auth_revision=auth_revision+1,updated_at=? WHERE id=?`, now, userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE admin_sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, now, userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE set_password_tokens SET used_at=? WHERE user_id=? AND used_at IS NULL`, now, userID); err != nil {
			return err
		}
		return appendAudit(ctx, tx, actorID, "user.disabled", "user", userID, map[string]any{"previous_role_id": roleID})
	})
}

func (s *Store) ListRoles(ctx context.Context) ([]identity.Role, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,status,revision FROM roles ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []identity.Role
	for rows.Next() {
		var role identity.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Status, &role.Revision); err != nil {
			return nil, err
		}
		role.Capabilities, err = roleCapabilities(ctx, s.db, role.ID)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func roleCapabilities(ctx context.Context, queryer contextQueryer, roleID string) ([]identity.Capability, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT capability FROM role_capabilities WHERE role_id=? ORDER BY capability`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var capabilities []identity.Capability
	for rows.Next() {
		var capability identity.Capability
		if err := rows.Scan(&capability); err != nil {
			return nil, err
		}
		capabilities = append(capabilities, capability)
	}
	return capabilities, rows.Err()
}

func (s *Store) ListUsers(ctx context.Context) ([]identity.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,email,display_name,role,status,COALESCE(password_hash,''),auth_revision
		FROM users ORDER BY email_normalized,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []identity.User
	for rows.Next() {
		var user identity.User
		if err := rows.Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.Status, &user.PasswordHash, &user.AuthRevision); err != nil {
			return nil, err
		}
		user.PasswordHash = ""
		users = append(users, user)
	}
	return users, rows.Err()
}
