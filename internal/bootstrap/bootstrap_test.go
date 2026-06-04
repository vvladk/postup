package bootstrap

import (
	"database/sql"
	"strings"
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
	t.Cleanup(func() { database.Close() })
	return database
}

func newMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	database := newTestDB(t)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

// TestBootstrapNoAdmin verifies that Bootstrap runs the init dialog when the
// DB has no admin (covers both "DB absent" and "DB exists but no admin" cases,
// since Bootstrap always runs migrations first).
func TestBootstrapNoAdmin(t *testing.T) {
	database := newTestDB(t)

	oldReader := dialogReader
	oldPwd := dialogPwd
	dialogReader = strings.NewReader("y\nadmin@test.com\n")
	dialogPwd = func() ([]byte, error) { return []byte("testpass1"), nil }
	defer func() {
		dialogReader = oldReader
		dialogPwd = oldPwd
	}()

	if err := Bootstrap(database); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var email, role string
	if err := database.QueryRow(
		`SELECT email, role FROM users WHERE role = 'admin'`,
	).Scan(&email, &role); err != nil {
		t.Fatalf("admin not found after Bootstrap: %v", err)
	}
	if email != "admin@test.com" {
		t.Errorf("expected admin@test.com, got %s", email)
	}
}

// TestBootstrapAdminExists verifies that Bootstrap returns nil immediately
// when an admin account already exists.
func TestBootstrapAdminExists(t *testing.T) {
	database := newMigratedDB(t)

	now := "2024-01-01T00:00:00Z"
	if _, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, created_at, updated_at) VALUES (?, ?, 'admin', ?, ?)`,
		"existing@test.com", "hash", now, now,
	); err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	dialogCalled := false
	oldPwd := dialogPwd
	dialogPwd = func() ([]byte, error) {
		dialogCalled = true
		return nil, nil
	}
	defer func() { dialogPwd = oldPwd }()

	if err := Bootstrap(database); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialogCalled {
		t.Error("init dialog should not be called when admin exists")
	}
}

// TestBootstrapUserDeclines verifies that Bootstrap calls os.Exit(0) when the
// user answers "n" to the initialization prompt.
func TestBootstrapUserDeclines(t *testing.T) {
	database := newTestDB(t)

	oldReader := dialogReader
	dialogReader = strings.NewReader("n\n")
	defer func() { dialogReader = oldReader }()

	var exitCode = -1
	oldExit := osExit
	osExit = func(code int) {
		exitCode = code
		panic("os.Exit")
	}
	defer func() { osExit = oldExit }()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		Bootstrap(database) //nolint:errcheck
	}()

	if !panicked {
		t.Error("expected os.Exit to be called")
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

// TestResetAdminPassword verifies that ResetAdminPassword updates the admin's
// password hash in the database.
func TestResetAdminPassword(t *testing.T) {
	database := newMigratedDB(t)

	now := "2024-01-01T00:00:00Z"
	if _, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, created_at, updated_at) VALUES (?, ?, 'admin', ?, ?)`,
		"admin@test.com", "oldhash", now, now,
	); err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	oldPwd := dialogPwd
	dialogPwd = func() ([]byte, error) { return []byte("newpassword"), nil }
	defer func() { dialogPwd = oldPwd }()

	if err := ResetAdminPassword(database); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hash string
	if err := database.QueryRow(
		`SELECT password_hash FROM users WHERE role = 'admin'`,
	).Scan(&hash); err != nil {
		t.Fatalf("query admin: %v", err)
	}

	if hash == "oldhash" {
		t.Error("password_hash was not updated")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("newpassword")); err != nil {
		t.Errorf("new hash is not a valid bcrypt hash: %v", err)
	}
}
