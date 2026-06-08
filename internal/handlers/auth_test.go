package handlers_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/postup-app/postup/internal/db"
	"github.com/postup-app/postup/internal/handlers"
	"github.com/postup-app/postup/internal/session"
)

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

// newTestRenderer returns a Renderer backed by minimal in-memory templates.
func newTestRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/login.html": {
			Data: []byte(`{{define "content"}}<form>{{if .Error}}{{.Error}}{{end}}</form>{{end}}`),
		},
		"templates/pages/invite.html": {
			Data: []byte(`{{define "content"}}{{if .Invalid}}недійсне або вже використане{{else}}Встанови пароль{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}{{end}}`),
		},
		"templates/pages/forgot.html": {
			Data: []byte(`{{define "content"}}{{if .Success}}{{if .Link}}посилання: {{.Link}}{{end}}{{end}}<form></form>{{end}}`),
		},
		"templates/pages/reset.html": {
			Data: []byte(`{{define "content"}}{{if .Invalid}}недійсне або прострочене{{else}}Новий пароль{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}{{end}}`),
		},
	}, nil, "", nil)
}

func createTestAdmin(t *testing.T, database *sql.DB, email, password string) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, created_at, updated_at) VALUES (?, ?, 'admin', ?, ?)`,
		email, string(hash), now, now,
	)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestHandleLoginGetUnauthorized(t *testing.T) {
	database := newTestDB(t)
	re := newTestRenderer()

	req := httptest.NewRequest("GET", "/login", nil)
	rr := httptest.NewRecorder()
	handlers.HandleLoginGet(database, re)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "<form") {
		t.Errorf("expected body to contain <form, got: %s", rr.Body.String())
	}
}

func TestHandleLoginGetAuthorized(t *testing.T) {
	database := newTestDB(t)
	userID := createTestAdmin(t, database, "admin@example.com", "password123")
	re := newTestRenderer()

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest("GET", "/login", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	handlers.HandleLoginGet(database, re)(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
}

func TestHandleLoginPostValid(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	re := newTestRenderer()

	body := strings.NewReader("email=admin%40example.com&password=password123")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handlers.HandleLoginPost(database, re)(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var found bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected Set-Cookie with non-empty session_id")
	}
}

func TestHandleLoginPostWrongPassword(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	re := newTestRenderer()

	body := strings.NewReader("email=admin%40example.com&password=wrongpassword")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handlers.HandleLoginPost(database, re)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Невірний email або пароль") {
		t.Errorf("expected error message in body, got: %s", rr.Body.String())
	}
}

func TestHandleLoginPostUnknownEmail(t *testing.T) {
	database := newTestDB(t)
	re := newTestRenderer()

	body := strings.NewReader("email=nobody%40example.com&password=password123")
	req := httptest.NewRequest("POST", "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handlers.HandleLoginPost(database, re)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Невірний email або пароль") {
		t.Errorf("expected error message in body, got: %s", rr.Body.String())
	}
}

// insertUserWithInvite creates a member user with the given invite_token.
// Pass a non-nil usedAt to simulate an already-used invite.
func insertUserWithInvite(t *testing.T, database *sql.DB, email, inviteToken string, usedAt *string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, invite_token, invite_used_at, created_at, updated_at)
		 VALUES (?, 'placeholder', 'member', ?, ?, ?, ?)`,
		email, inviteToken, usedAt, now, now,
	)
	if err != nil {
		t.Fatalf("insert user with invite: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestHandleInviteGetValid(t *testing.T) {
	database := newTestDB(t)
	insertUserWithInvite(t, database, "member@example.com", "test-token-123", nil)
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /invite/{token}", handlers.HandleInviteGet(database, re))

	req := httptest.NewRequest("GET", "/invite/test-token-123", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Встанови пароль") {
		t.Errorf("expected body to contain 'Встанови пароль', got: %s", rr.Body.String())
	}
}

func TestHandleInviteGetInvalidToken(t *testing.T) {
	database := newTestDB(t)
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /invite/{token}", handlers.HandleInviteGet(database, re))

	req := httptest.NewRequest("GET", "/invite/no-such-token", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "недійсне або вже використане") {
		t.Errorf("expected invalid-invite message, got: %s", rr.Body.String())
	}
}

func TestHandleInviteGetAlreadyUsed(t *testing.T) {
	database := newTestDB(t)
	usedAt := time.Now().UTC().Format(time.RFC3339)
	insertUserWithInvite(t, database, "member@example.com", "test-token-123", &usedAt)
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /invite/{token}", handlers.HandleInviteGet(database, re))

	req := httptest.NewRequest("GET", "/invite/test-token-123", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "недійсне або вже використане") {
		t.Errorf("expected invalid-invite message, got: %s", rr.Body.String())
	}
}

func TestHandleInvitePostValid(t *testing.T) {
	database := newTestDB(t)
	insertUserWithInvite(t, database, "member@example.com", "test-token-123", nil)
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /invite/{token}", handlers.HandleInvitePost(database, re))

	body := strings.NewReader("password=newpassword1&password_confirm=newpassword1")
	req := httptest.NewRequest("POST", "/invite/test-token-123", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var inviteToken, inviteUsedAt, passwordHash sql.NullString
	database.QueryRow(
		`SELECT invite_token, invite_used_at, password_hash FROM users WHERE email = ?`,
		"member@example.com",
	).Scan(&inviteToken, &inviteUsedAt, &passwordHash)

	if inviteToken.Valid {
		t.Errorf("expected invite_token to be NULL after activation, got %q", inviteToken.String)
	}
	if !inviteUsedAt.Valid {
		t.Error("expected invite_used_at to be set after activation")
	}
	if passwordHash.String == "placeholder" {
		t.Error("expected password_hash to be updated after activation")
	}
}

func TestHandleInvitePostPasswordMismatch(t *testing.T) {
	database := newTestDB(t)
	insertUserWithInvite(t, database, "member@example.com", "test-token-123", nil)
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /invite/{token}", handlers.HandleInvitePost(database, re))

	body := strings.NewReader("password=newpassword1&password_confirm=differentpass")
	req := httptest.NewRequest("POST", "/invite/test-token-123", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "збігаються") {
		t.Errorf("expected mismatch error in body, got: %s", rr.Body.String())
	}
}

func TestHandleInvitePostShortPassword(t *testing.T) {
	database := newTestDB(t)
	insertUserWithInvite(t, database, "member@example.com", "test-token-123", nil)
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /invite/{token}", handlers.HandleInvitePost(database, re))

	body := strings.NewReader("password=abc&password_confirm=abc")
	req := httptest.NewRequest("POST", "/invite/test-token-123", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "символи") {
		t.Errorf("expected short-password error in body, got: %s", rr.Body.String())
	}
}

func insertUserWithResetToken(t *testing.T, database *sql.DB, email, token string, expiresAt time.Time) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, reset_token, reset_token_expires_at, created_at, updated_at)
		 VALUES (?, 'placeholder', 'member', ?, ?, ?, ?)`,
		email, token, expiresAt.UTC().Format(time.RFC3339), now, now,
	)
	if err != nil {
		t.Fatalf("insert user with reset token: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestHandleForgotPostExistingEmail(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	re := newTestRenderer()

	body := strings.NewReader("email=admin%40example.com")
	req := httptest.NewRequest("POST", "/forgot-password", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handlers.HandleForgotPost(database, re, "8080", func() string { return "127.0.0.1" })(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/reset-password/") {
		t.Errorf("expected reset link in body, got: %s", rr.Body.String())
	}

	var resetToken, resetExpiresAt sql.NullString
	database.QueryRow(
		`SELECT reset_token, reset_token_expires_at FROM users WHERE email = ?`,
		"admin@example.com",
	).Scan(&resetToken, &resetExpiresAt)

	if !resetToken.Valid || resetToken.String == "" {
		t.Error("expected reset_token to be set in DB")
	}
	if !resetExpiresAt.Valid {
		t.Error("expected reset_token_expires_at to be set in DB")
	}
	expires, _ := time.Parse(time.RFC3339, resetExpiresAt.String)
	if !expires.After(time.Now().UTC()) {
		t.Error("expected reset_token_expires_at to be in the future")
	}
}

func TestHandleForgotPostUnknownEmail(t *testing.T) {
	database := newTestDB(t)
	re := newTestRenderer()

	body := strings.NewReader("email=nobody%40example.com")
	req := httptest.NewRequest("POST", "/forgot-password", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handlers.HandleForgotPost(database, re, "8080", func() string { return "127.0.0.1" })(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM users WHERE reset_token IS NOT NULL`).Scan(&count)
	if count != 0 {
		t.Error("expected no reset tokens to be set for unknown email")
	}
}

func TestHandleResetGetValid(t *testing.T) {
	database := newTestDB(t)
	insertUserWithResetToken(t, database, "member@example.com", "reset-token-123", time.Now().Add(time.Hour))
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /reset-password/{token}", handlers.HandleResetGet(database, re))

	req := httptest.NewRequest("GET", "/reset-password/reset-token-123", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Новий пароль") {
		t.Errorf("expected reset form in body, got: %s", rr.Body.String())
	}
}

func TestHandleResetGetExpired(t *testing.T) {
	database := newTestDB(t)
	insertUserWithResetToken(t, database, "member@example.com", "reset-token-123", time.Now().Add(-time.Hour))
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /reset-password/{token}", handlers.HandleResetGet(database, re))

	req := httptest.NewRequest("GET", "/reset-password/reset-token-123", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "недійсне або прострочене") {
		t.Errorf("expected expired message in body, got: %s", rr.Body.String())
	}
}

func TestHandleResetPostValid(t *testing.T) {
	database := newTestDB(t)
	insertUserWithResetToken(t, database, "member@example.com", "reset-token-123", time.Now().Add(time.Hour))
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /reset-password/{token}", handlers.HandleResetPost(database, re))

	body := strings.NewReader("password=newpassword1&password_confirm=newpassword1")
	req := httptest.NewRequest("POST", "/reset-password/reset-token-123", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login?reset=1" {
		t.Errorf("expected redirect to /login?reset=1, got %q", loc)
	}

	var resetToken sql.NullString
	var passwordHash string
	database.QueryRow(
		`SELECT reset_token, password_hash FROM users WHERE email = ?`,
		"member@example.com",
	).Scan(&resetToken, &passwordHash)

	if resetToken.Valid {
		t.Error("expected reset_token to be NULL after reset")
	}
	if passwordHash == "placeholder" {
		t.Error("expected password_hash to be updated after reset")
	}
}

func TestHandleResetPostPasswordMismatch(t *testing.T) {
	database := newTestDB(t)
	insertUserWithResetToken(t, database, "member@example.com", "reset-token-123", time.Now().Add(time.Hour))
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /reset-password/{token}", handlers.HandleResetPost(database, re))

	body := strings.NewReader("password=newpassword1&password_confirm=differentpass")
	req := httptest.NewRequest("POST", "/reset-password/reset-token-123", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "збігаються") {
		t.Errorf("expected mismatch error in body, got: %s", rr.Body.String())
	}
}

func TestHandleResetPostReusedToken(t *testing.T) {
	database := newTestDB(t)
	insertUserWithResetToken(t, database, "member@example.com", "reset-token-123", time.Now().Add(time.Hour))
	re := newTestRenderer()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /reset-password/{token}", handlers.HandleResetPost(database, re))

	postReset := func() *httptest.ResponseRecorder {
		body := strings.NewReader("password=newpassword1&password_confirm=newpassword1")
		req := httptest.NewRequest("POST", "/reset-password/reset-token-123", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	if rr := postReset(); rr.Code != http.StatusSeeOther {
		t.Fatalf("expected first reset to succeed with 302, got %d", rr.Code)
	}

	rr := postReset()
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 on reused token, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "недійсне або прострочене") {
		t.Errorf("expected invalid message on reused token, got: %s", rr.Body.String())
	}
}

func TestHandleLogout(t *testing.T) {
	database := newTestDB(t)
	userID := createTestAdmin(t, database, "admin@example.com", "password123")

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest("GET", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	handlers.HandleLogout(database)(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}

	var found bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			found = true
			if c.MaxAge != -1 {
				t.Errorf("expected session_id MaxAge=-1, got %d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("expected Set-Cookie with session_id to clear it")
	}
}
