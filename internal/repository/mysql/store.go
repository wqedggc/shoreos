package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/wqedggc/shoreos/internal/config"
	"github.com/wqedggc/shoreos/pkg/identity"

	drivermysql "github.com/go-sql-driver/mysql"
)

type Store struct {
	db  *sql.DB
	cfg config.Config
}

var (
	ErrAlreadyInitialized = errors.New("shoreos already has a user")
	ErrUsernameTaken      = errors.New("username already exists")
)

type User struct {
	ID          int64  `json:"userId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
}

func Open(cfg config.Config) (*Store, error) {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, cfg: cfg}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Bootstrap(ctx context.Context) (User, string, error) {
	if strings.TrimSpace(s.cfg.AdminPassword) == "" {
		return User{}, "", errors.New("local bootstrap password is not configured")
	}
	passHash, err := identity.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		return User{}, "", err
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO shoreos_users (username, password_hash, display_name, avatar, status, created_at, updated_at)
SELECT ?, ?, ?, '🌸', 'ACTIVE', NOW(3), NOW(3)
WHERE NOT EXISTS (SELECT 1 FROM shoreos_users)
`, s.cfg.AdminUsername, passHash, s.cfg.AdminDisplay)
	if err != nil {
		return User{}, "", err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return User{}, "", err
	} else if affected != 1 {
		return User{}, "", ErrAlreadyInitialized
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return User{}, "", err
	}
	user, err := s.UserByID(ctx, userID)
	if err != nil {
		return User{}, "", err
	}
	token, err := s.createSession(ctx, user.ID)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Store) Login(ctx context.Context, username, password string) (User, string, error) {
	var user User
	var storedHash string
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, display_name, avatar, password_hash
FROM shoreos_users
WHERE username = ? AND status = 'ACTIVE'
`, username).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Avatar, &storedHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, "", sql.ErrNoRows
		}
		return User{}, "", err
	}
	matched, needsUpgrade := identity.CheckPassword(password, storedHash)
	if !matched {
		return User{}, "", sql.ErrNoRows
	}
	if needsUpgrade {
		upgradedHash, err := identity.HashPassword(password)
		if err != nil {
			return User{}, "", err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE shoreos_users SET password_hash = ?, updated_at = NOW(3) WHERE id = ?`, upgradedHash, user.ID); err != nil {
			return User{}, "", err
		}
	}
	token, err := s.createSession(ctx, user.ID)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Store) Register(ctx context.Context, username, password string) (User, string, error) {
	passHash, err := identity.HashPassword(password)
	if err != nil {
		return User{}, "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
INSERT INTO shoreos_users (username, password_hash, display_name, avatar, status, created_at, updated_at)
VALUES (?, ?, ?, '🌸', 'ACTIVE', NOW(3), NOW(3))
`, username, passHash, username)
	if err != nil {
		var mysqlErr *drivermysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return User{}, "", ErrUsernameTaken
		}
		return User{}, "", err
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, "", err
	}
	token, err := s.createSessionWithExecutor(ctx, tx, userID)
	if err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", err
	}
	return User{ID: userID, Username: username, DisplayName: username, Avatar: "🌸"}, token, nil
}

func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM shoreos_sessions WHERE token_hash = ?`, identity.HashToken(token))
	return err
}

func (s *Store) UserByToken(ctx context.Context, token string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.username, u.display_name, u.avatar
FROM shoreos_sessions s
JOIN shoreos_users u ON u.id = s.user_id
WHERE s.token_hash = ? AND s.expires_at > NOW(3) AND u.status = 'ACTIVE'
`, identity.HashToken(token)).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Avatar)
	return user, err
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, display_name, avatar
FROM shoreos_users
WHERE id = ? AND status = 'ACTIVE'
`, id).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Avatar)
	return user, err
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, display_name, avatar
FROM shoreos_users
WHERE username = ? AND status = 'ACTIVE'
`, username).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Avatar)
	return user, err
}

func (s *Store) UpdateUser(ctx context.Context, userID int64, displayName, avatar string) (User, error) {
	if displayName == "" {
		displayName = s.cfg.AdminDisplay
	}
	if avatar == "" {
		avatar = "🌸"
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE shoreos_users
SET display_name = ?, avatar = ?, updated_at = NOW(3)
WHERE id = ?
`, displayName, avatar, userID)
	if err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, userID)
}

func (s *Store) Profiles(ctx context.Context, userID int64) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT scenario_json
FROM fire_scenarios
WHERE user_id = ?
ORDER BY id ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []map[string]any
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var profile map[string]any
		if err := json.Unmarshal([]byte(raw), &profile); err != nil {
			return nil, fmt.Errorf("decode profile: %w", err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *Store) SyncProfiles(ctx context.Context, userID int64, profiles []map[string]any) error {
	// 防御：空 profiles 视为"无同步意图"，直接返回，避免误 DELETE 清空该用户全部档案
	if len(profiles) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	seen := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		uid, _ := profile["uid"].(string)
		if strings.TrimSpace(uid) == "" {
			continue
		}
		name, _ := profile["name"].(string)
		avatar, _ := profile["avatar"].(string)
		raw, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		seen = append(seen, uid)
		_, err = tx.ExecContext(ctx, `
INSERT INTO fire_scenarios (user_id, profile_uid, name, avatar, scenario_json, updated_at, created_at)
VALUES (?, ?, ?, ?, ?, NOW(3), NOW(3))
ON DUPLICATE KEY UPDATE name = VALUES(name), avatar = VALUES(avatar), scenario_json = VALUES(scenario_json), updated_at = NOW(3)
`, userID, uid, name, avatar, string(raw))
		if err != nil {
			return err
		}
	}

	if len(seen) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fire_scenarios WHERE user_id = ?`, userID); err != nil {
			return err
		}
	} else {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(seen)), ",")
		args := make([]any, 0, len(seen)+1)
		args = append(args, userID)
		for _, uid := range seen {
			args = append(args, uid)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM fire_scenarios WHERE user_id = ? AND profile_uid NOT IN (`+placeholders+`)`, args...); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) createSession(ctx context.Context, userID int64) (string, error) {
	return s.createSessionWithExecutor(ctx, s.db, userID)
}

type sessionExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) createSessionWithExecutor(ctx context.Context, executor sessionExecer, userID int64) (string, error) {
	token, err := identity.NewToken()
	if err != nil {
		return "", err
	}
	_, err = executor.ExecContext(ctx, `
INSERT INTO shoreos_sessions (user_id, token_hash, expires_at, created_at)
VALUES (?, ?, ?, NOW(3))
`, userID, identity.HashToken(token), time.Now().Add(s.cfg.SessionTTL))
	if err != nil {
		return "", err
	}
	return token, nil
}
