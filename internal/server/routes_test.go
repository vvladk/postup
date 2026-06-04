package server

import (
	"database/sql"
	"embed"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/postup-app/postup/internal/config"
	"github.com/postup-app/postup/internal/db"
	"github.com/postup-app/postup/internal/session"
	"github.com/postup-app/postup/internal/ws"
)

func newTestApp(t *testing.T) (*App, *sql.DB) {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		Port:  "8080",
		GetIP: func() string { return "127.0.0.1" },
		FS:    embed.FS{},
	}
	app := &App{DB: database, Hub: ws.NewHub(), Cfg: cfg}
	return app, database
}

func newTestMux(t *testing.T) (*http.ServeMux, *sql.DB) {
	t.Helper()
	app, database := newTestApp(t)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	return mux, database
}

// TestRoutesRegistered verifies every expected route is registered in the mux.
func TestRoutesRegistered(t *testing.T) {
	mux, _ := newTestMux(t)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/"},
		{"GET", "/dashboard"},
		{"GET", "/retros/new"},
		{"POST", "/retros"},
		{"GET", "/retros/1/invite-link"},
		{"GET", "/retros/1/edit"},
		{"POST", "/retros/1"},
		{"POST", "/retros/1/finish"},
		{"GET", "/join/sometoken"},
		{"GET", "/login"},
		{"POST", "/login"},
		{"GET", "/logout"},
		{"GET", "/invite/sometoken"},
		{"POST", "/invite/sometoken"},
		{"GET", "/forgot-password"},
		{"POST", "/forgot-password"},
		{"GET", "/reset-password/sometoken"},
		{"POST", "/reset-password/sometoken"},
		{"GET", "/users"},
		{"GET", "/users/new"},
		{"POST", "/users"},
		{"GET", "/users/1/edit"},
		{"POST", "/users/1"},
		{"POST", "/users/1/delete"},
		{"POST", "/users/1/reset-password"},
		{"GET", "/teams"},
		{"GET", "/teams/new"},
		{"POST", "/teams"},
		{"GET", "/teams/1/edit"},
		{"POST", "/teams/1"},
		{"POST", "/teams/1/delete"},
		{"GET", "/templates"},
		{"GET", "/templates/new"},
		{"POST", "/templates"},
		{"GET", "/templates/1/edit"},
		{"POST", "/templates/1"},
		{"POST", "/templates/1/archive"},
		{"POST", "/templates/1/delete"},
		{"GET", "/retros/1/board"},
		{"GET", "/retros/1/ws"},
		{"POST", "/retros/1/cards"},
		{"POST", "/cards/1"},
		{"POST", "/cards/1/delete"},
		{"POST", "/cards/1/move"},
		{"POST", "/cards/1/copy"},
		{"POST", "/cards/1/vote"},
		{"POST", "/retros/1/action-items"},
		{"POST", "/action-items/1/status"},
		{"GET", "/settings"},
		{"POST", "/settings"},
		{"GET", "/timer"},
		{"GET", "/statuses"},
		{"POST", "/statuses"},
		{"POST", "/statuses/1"},
		{"POST", "/statuses/1/delete"},
		{"GET", "/static/main.css"},
	}

	for _, tt := range routes {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			_, pattern := mux.Handler(req)
			if pattern == "" {
				t.Errorf("route %s %s is not registered", tt.method, tt.path)
			}
		})
	}
}

// TestProtectedRouteRedirects verifies that accessing a protected route without
// a session cookie results in a redirect to /login.
func TestProtectedRouteRedirects(t *testing.T) {
	mux, _ := newTestMux(t)

	req := httptest.NewRequest("GET", "/users", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// TestAdminRouteBlocksMember verifies that a logged-in non-admin user receives
// 403 when accessing an admin-only route.
func TestAdminRouteBlocksMember(t *testing.T) {
	mux, database := newTestMux(t)

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, created_at, updated_at) VALUES (?, ?, 'member', ?, ?)`,
		"member@test.com", "hash", now, now,
	)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	userID, _ := res.LastInsertId()

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest("GET", "/users", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}
