package session_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/vladyslavkondratiuk/postup/internal/db"
	"github.com/vladyslavkondratiuk/postup/internal/session"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createTestUser(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, first_name, last_name, role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test@example.com", "hash", "Test", "User", "member", now, now,
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestCreate(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database)

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.Token == "" {
		t.Error("Token is empty")
	}
	if sess.UserID != userID {
		t.Errorf("UserID: want %d, got %d", userID, sess.UserID)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE token = ?`, sess.Token).Scan(&count)
	if count != 1 {
		t.Errorf("session not found in DB, count=%d", count)
	}
}

func TestGetValid(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database)

	created, _ := session.Create(database, userID)
	got, err := session.Get(database, created.Token)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID: want %d, got %d", created.ID, got.ID)
	}
	if got.Token != created.Token {
		t.Error("Token mismatch")
	}
}

func TestGetNotFound(t *testing.T) {
	database := openTestDB(t)

	_, err := session.Get(database, "nonexistent-token")
	if err == nil {
		t.Fatal("expected error for nonexistent token, got nil")
	}
	if err != session.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetExpired(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database)

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := database.Exec(
		`INSERT INTO sessions (user_id, token, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		userID, "expired-token", past, now,
	)
	if err != nil {
		t.Fatalf("insert expired session: %v", err)
	}

	_, err = session.Get(database, "expired-token")
	if err == nil {
		t.Fatal("expected error for expired session, got nil")
	}
	if err != session.ErrExpired {
		t.Errorf("expected ErrExpired, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database)

	sess, _ := session.Create(database, userID)
	if err := session.Delete(database, sess.Token); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := session.Get(database, sess.Token)
	if err == nil {
		t.Fatal("expected error after Delete, got nil")
	}
}
