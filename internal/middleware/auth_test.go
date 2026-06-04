package middleware_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/postup-app/postup/internal/db"
	"github.com/postup-app/postup/internal/middleware"
	"github.com/postup-app/postup/internal/session"
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

func createTestUser(t *testing.T, database *sql.DB, role string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, first_name, last_name, role, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test@example.com", "hash", "Test", "User", role, now, now,
	)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestRequireAuthNoCookie(t *testing.T) {
	database := openTestDB(t)
	auth := middleware.NewAuth(database)

	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuthValidSession(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database, "member")
	sess, _ := session.Create(database, userID)
	auth := middleware.NewAuth(database)

	var gotUserID int64
	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := middleware.GetUser(r); u != nil {
			gotUserID = u.ID
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if gotUserID != userID {
		t.Errorf("GetUser returned wrong user: want %d, got %d", userID, gotUserID)
	}
}

func TestRequireAdminMember(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database, "member")
	sess, _ := session.Create(database, userID)
	auth := middleware.NewAuth(database)

	handler := auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdminAdmin(t *testing.T) {
	database := openTestDB(t)
	userID := createTestUser(t, database, "admin")
	sess, _ := session.Create(database, userID)
	auth := middleware.NewAuth(database)

	var handlerCalled bool
	handler := auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !handlerCalled {
		t.Error("handler was not called for admin user")
	}
}
