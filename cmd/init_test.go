package cmd

import (
	"database/sql"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/postup-app/postup/internal/db"
)

func init() {
	bcryptCost = bcrypt.MinCost
}

func newTestDB(t *testing.T) *sql.DB {
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

func TestInitAdminSuccess(t *testing.T) {
	database := newTestDB(t)

	if err := initAdmin(database, "admin@example.com", "password123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hash, role string
	if err := database.QueryRow(
		`SELECT password_hash, role FROM users WHERE email = ?`, "admin@example.com",
	).Scan(&hash, &role); err != nil {
		t.Fatalf("user not found: %v", err)
	}

	if role != "admin" {
		t.Errorf("expected role=admin, got %q", role)
	}
	if hash == "password123" {
		t.Error("password_hash must not equal the original password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123")); err != nil {
		t.Errorf("password_hash is not a valid bcrypt hash of the password: %v", err)
	}
}

func TestInitAdminTwice(t *testing.T) {
	database := newTestDB(t)

	if err := initAdmin(database, "admin@example.com", "password123"); err != nil {
		t.Fatalf("first initAdmin failed: %v", err)
	}

	err := initAdmin(database, "admin@example.com", "password123")
	if !errors.Is(err, errAdminExists) {
		t.Fatalf("expected errAdminExists, got %v", err)
	}
}

func TestInitAdminBadEmail(t *testing.T) {
	database := newTestDB(t)

	err := initAdmin(database, "notanemail", "password123")
	if err == nil {
		t.Fatal("expected error for email without @")
	}
}

func TestInitAdminShortPassword(t *testing.T) {
	database := newTestDB(t)

	err := initAdmin(database, "admin@example.com", "short")
	if err == nil {
		t.Fatal("expected error for password shorter than 8 chars")
	}
}

func TestResetAdminSuccess(t *testing.T) {
	database := newTestDB(t)

	if err := initAdmin(database, "admin@example.com", "password123"); err != nil {
		t.Fatalf("initAdmin failed: %v", err)
	}

	var oldHash string
	database.QueryRow(`SELECT password_hash FROM users WHERE role = 'admin'`).Scan(&oldHash)

	if err := resetAdmin(database, "admin@example.com", "newpassword456"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var newHash string
	database.QueryRow(`SELECT password_hash FROM users WHERE role = 'admin'`).Scan(&newHash)

	if oldHash == newHash {
		t.Error("password_hash must change after reset")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(newHash), []byte("newpassword456")); err != nil {
		t.Errorf("new password_hash is not a valid bcrypt hash of the new password: %v", err)
	}
}

func TestResetAdminNoAdmin(t *testing.T) {
	database := newTestDB(t)

	err := resetAdmin(database, "admin@example.com", "password123")
	if !errors.Is(err, errAdminNotFound) {
		t.Fatalf("expected errAdminNotFound, got %v", err)
	}
}
