package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/wqedggc/shoreos/pkg/identity"
)

var (
	ErrBindingCodeInvalid = errors.New("identity binding code is invalid")
	ErrBindingCodeExpired = errors.New("identity binding code is expired")
	ErrIdentityBound      = errors.New("identity is already bound")
)

func (s *Store) UserByIdentity(ctx context.Context, provider, subject string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.username, u.display_name, u.avatar
FROM shoreos_user_identities i
JOIN shoreos_users u ON u.id = i.user_id
WHERE i.provider = ? AND i.provider_subject = ? AND u.status = 'ACTIVE'
`, provider, subject).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Avatar)
	return user, err
}

func (s *Store) CreateIdentityBindingCode(ctx context.Context, userID int64, provider string, ttl time.Duration) (string, time.Time, error) {
	code, err := identity.NewToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(ttl)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO shoreos_identity_binding_codes (user_id, provider, code_hash, expires_at, created_at)
VALUES (?, ?, ?, ?, NOW(3))
`, userID, provider, identity.HashToken(code), expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return code, expiresAt, nil
}

func (s *Store) ConsumeIdentityBindingCode(ctx context.Context, code, provider, subject, unionID string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var userID int64
	var expiresAt time.Time
	var consumedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT user_id, expires_at, consumed_at
FROM shoreos_identity_binding_codes
WHERE provider = ? AND code_hash = ?
FOR UPDATE
`, provider, identity.HashToken(code)).Scan(&userID, &expiresAt, &consumedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrBindingCodeInvalid
	}
	if err != nil {
		return User{}, err
	}
	if consumedAt.Valid {
		return User{}, ErrBindingCodeInvalid
	}
	if !expiresAt.After(time.Now()) {
		return User{}, ErrBindingCodeExpired
	}

	var boundUserID int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM shoreos_user_identities WHERE provider = ? AND provider_subject = ?`, provider, subject).Scan(&boundUserID)
	if err == nil {
		return User{}, ErrIdentityBound
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO shoreos_user_identities (user_id, provider, provider_subject, provider_union_id, created_at, updated_at)
VALUES (?, ?, ?, NULLIF(?, ''), NOW(3), NOW(3))
`, userID, provider, subject, unionID); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE shoreos_identity_binding_codes SET consumed_at = NOW(3) WHERE provider = ? AND code_hash = ?`, provider, identity.HashToken(code)); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, userID)
}

func (s *Store) CreateSession(ctx context.Context, userID int64) (string, error) {
	return s.createSession(ctx, userID)
}
