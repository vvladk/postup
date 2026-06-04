package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound = errors.New("session not found")
	ErrExpired  = errors.New("session expired")
)

const TTL = 12 * time.Hour

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	ExpiresAt time.Time
}

func Create(db *sql.DB, userID int64) (*Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(b)

	now := time.Now().UTC()
	expiresAt := now.Add(TTL)

	res, err := db.Exec(
		`INSERT INTO sessions (user_id, token, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		userID,
		token,
		expiresAt.Format(time.RFC3339),
		now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	id, _ := res.LastInsertId()
	return &Session{ID: id, UserID: userID, Token: token, ExpiresAt: expiresAt}, nil
}

func Get(db *sql.DB, token string) (*Session, error) {
	var s Session
	var expiresAt string

	err := db.QueryRow(
		`SELECT id, user_id, token, expires_at FROM sessions WHERE token = ?`, token,
	).Scan(&s.ID, &s.UserID, &s.Token, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse expires_at: %w", err)
	}
	s.ExpiresAt = t

	if time.Now().UTC().After(s.ExpiresAt) {
		return nil, ErrExpired
	}
	return &s, nil
}

func Delete(db *sql.DB, token string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
