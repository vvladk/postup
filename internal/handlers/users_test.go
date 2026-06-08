package handlers_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/postup-app/postup/internal/handlers"
	"github.com/postup-app/postup/internal/middleware"
)

func newUsersRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/users_new.html": {
			Data: []byte(`{{define "content"}}{{if .InviteLink}}{{.InviteLink}}{{else}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}{{end}}`),
		},
		"templates/pages/users.html": {
			Data: []byte(`{{define "content"}}{{range .Users}}{{.Email}} {{end}}{{end}}`),
		},
	}, nil, "", nil)
}

func createTestTeam(t *testing.T, database *sql.DB, name string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO teams (name, created_at) VALUES (?, ?)`, name, now,
	)
	if err != nil {
		t.Fatalf("create team %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func createTestMember(t *testing.T, database *sql.DB, email string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO users (email, password_hash, first_name, last_name, role, created_at, updated_at)
		 VALUES (?, '', 'Test', 'User', 'member', ?, ?)`,
		email, now, now,
	)
	if err != nil {
		t.Fatalf("create member %q: %v", email, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func postUsers(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func newUsersMux(database *sql.DB, re *handlers.Renderer) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /users", handlers.HandleUsersCreate(database, re, "8080", func() string { return "127.0.0.1" }))
	return mux
}

func TestHandleUsersCreateValid(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	teamID := createTestTeam(t, database, "Backend")
	re := newUsersRenderer()

	rr := postUsers(newUsersMux(database, re),
		fmt.Sprintf("first_name=John&last_name=Doe&email=john@example.com&team_ids=%d", teamID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/invite/") {
		t.Errorf("expected invite link in body, got: %s", rr.Body.String())
	}

	var inviteToken, inviteUsedAt sql.NullString
	database.QueryRow(
		`SELECT invite_token, invite_used_at FROM users WHERE email = ?`, "john@example.com",
	).Scan(&inviteToken, &inviteUsedAt)

	if !inviteToken.Valid || inviteToken.String == "" {
		t.Error("expected invite_token to be set in DB")
	}
	if inviteUsedAt.Valid {
		t.Error("expected invite_used_at to be NULL for new user")
	}

	var memberCount int
	database.QueryRow(
		`SELECT COUNT(*) FROM team_members
		 WHERE user_id = (SELECT id FROM users WHERE email = ?) AND team_id = ?`,
		"john@example.com", teamID,
	).Scan(&memberCount)
	if memberCount != 1 {
		t.Errorf("expected user to be in team, got count=%d", memberCount)
	}
}

func TestHandleUsersCreateDuplicateEmail(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	createTestMember(t, database, "test@example.com")
	re := newUsersRenderer()

	rr := postUsers(newUsersMux(database, re),
		"first_name=Jane&last_name=Doe&email=test@example.com")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "вже існує") {
		t.Errorf("expected duplicate-email error in body, got: %s", rr.Body.String())
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, "test@example.com").Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 user with that email, got %d", count)
	}
}

func TestHandleUsersCreateEmptyEmail(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	re := newUsersRenderer()

	rr := postUsers(newUsersMux(database, re),
		"first_name=John&last_name=Doe&email=")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "обовʼязкові") {
		t.Errorf("expected required-fields error in body, got: %s", rr.Body.String())
	}
}

func TestHandleUsersCreateEmptyFirstName(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	re := newUsersRenderer()

	rr := postUsers(newUsersMux(database, re),
		"first_name=&last_name=Doe&email=john@example.com")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "обовʼязкові") {
		t.Errorf("expected required-fields error in body, got: %s", rr.Body.String())
	}
}

func TestHandleUsersCreateInviteLinkContainsToken(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	re := newUsersRenderer()

	rr := postUsers(newUsersMux(database, re),
		"first_name=John&last_name=Doe&email=link@example.com")

	bodyStr := rr.Body.String()
	if !strings.Contains(bodyStr, "/invite/") {
		t.Errorf("expected /invite/ in body, got: %s", bodyStr)
	}

	var inviteToken sql.NullString
	database.QueryRow(
		`SELECT invite_token FROM users WHERE email = ?`, "link@example.com",
	).Scan(&inviteToken)

	if !inviteToken.Valid || inviteToken.String == "" {
		t.Fatal("expected invite_token to be set in DB")
	}
	if !strings.Contains(bodyStr, inviteToken.String) {
		t.Errorf("expected token %q in body, got: %s", inviteToken.String, bodyStr)
	}
}

func newUsersEditRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/users_edit.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
	}, nil, "", nil)
}

func createTestRetro(t *testing.T, database *sql.DB, teamID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, date, vote_limit, status, created_at) VALUES (?, ?, 10, 'active', ?)`,
		teamID, now, now,
	)
	if err != nil {
		t.Fatalf("create retro: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func getStatusID(t *testing.T, database *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`SELECT id FROM action_statuses WHERE name = ?`, name).Scan(&id); err != nil {
		t.Fatalf("get status %q: %v", name, err)
	}
	return id
}

func createTestActionItem(t *testing.T, database *sql.DB, retroID, assigneeID, statusID int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO action_items (retro_id, assignee_id, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		retroID, assigneeID, now, statusID, now,
	)
	if err != nil {
		t.Fatalf("create action item: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestHandleUsersUpdateValid(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	team1ID := createTestTeam(t, database, "Frontend")
	team2ID := createTestTeam(t, database, "Backend")
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, team1ID, userID)

	re := newUsersEditRenderer()
	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}", handlers.HandleUsersUpdate(database, re))

	body := fmt.Sprintf("first_name=Updated&last_name=Name&team_ids=%d", team2ID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d", userID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var firstName, lastName string
	database.QueryRow(`SELECT first_name, last_name FROM users WHERE id = ?`, userID).Scan(&firstName, &lastName)
	if firstName != "Updated" || lastName != "Name" {
		t.Errorf("expected 'Updated Name', got '%s %s'", firstName, lastName)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM team_members WHERE user_id = ? AND team_id = ?`, userID, team1ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected user removed from team1, got count=%d", count)
	}
	database.QueryRow(`SELECT COUNT(*) FROM team_members WHERE user_id = ? AND team_id = ?`, userID, team2ID).Scan(&count)
	if count != 1 {
		t.Errorf("expected user added to team2, got count=%d", count)
	}
}

func TestHandleUsersUpdateEmptyName(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")

	re := newUsersEditRenderer()
	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}", handlers.HandleUsersUpdate(database, re))

	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d", userID), strings.NewReader("first_name=&last_name=Name"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "обовʼязкові") {
		t.Errorf("expected validation error in body, got: %s", rr.Body.String())
	}
}

func TestHandleUsersDeleteNoActionItems(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	teamID := createTestTeam(t, database, "Frontend")
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, userID)
	now := time.Now().UTC().Format(time.RFC3339)
	database.Exec(
		`INSERT INTO sessions (user_id, token, expires_at, created_at) VALUES (?, 'test-sess-token', ?, ?)`,
		userID, time.Now().Add(12*time.Hour).Format(time.RFC3339), now,
	)

	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}/delete", handlers.HandleUsersDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d/delete", userID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&count)
	if count != 0 {
		t.Errorf("expected user deleted, got count=%d", count)
	}
	database.QueryRow(`SELECT COUNT(*) FROM team_members WHERE user_id = ?`, userID).Scan(&count)
	if count != 0 {
		t.Errorf("expected team_members cleaned up, got count=%d", count)
	}
	database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, userID).Scan(&count)
	if count != 0 {
		t.Errorf("expected sessions cleaned up via CASCADE, got count=%d", count)
	}
}

func TestHandleUsersDeleteWithOpenActionItems(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	teamID := createTestTeam(t, database, "Frontend")
	retroID := createTestRetro(t, database, teamID)
	statusID := getStatusID(t, database, "Новий")
	createTestActionItem(t, database, retroID, userID, statusID)

	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}/delete", handlers.HandleUsersDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d/delete", userID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Location"), "error=") {
		t.Errorf("expected error param in redirect, got: %s", rr.Header().Get("Location"))
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&count)
	if count != 1 {
		t.Errorf("expected user to remain in DB, got count=%d", count)
	}
}

func TestHandleUsersDeleteWithCompletedActionItems(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	teamID := createTestTeam(t, database, "Frontend")
	retroID := createTestRetro(t, database, teamID)
	statusID := getStatusID(t, database, "Виконано")
	createTestActionItem(t, database, retroID, userID, statusID)

	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}/delete", handlers.HandleUsersDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d/delete", userID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&count)
	if count != 0 {
		t.Errorf("expected user deleted, got count=%d", count)
	}
}

func TestHandleUsersResetPassword(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	database.Exec(
		`UPDATE users SET reset_token = 'old-token', reset_token_expires_at = ? WHERE id = ?`,
		time.Now().Add(-time.Hour).Format(time.RFC3339), userID,
	)

	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}/reset-password", handlers.HandleUsersResetPassword(database, "8080", func() string { return "127.0.0.1" }))

	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d/reset-password", userID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Location"), "reset_link=") {
		t.Errorf("expected reset_link in redirect, got: %s", rr.Header().Get("Location"))
	}

	var resetToken, resetExpiresAt sql.NullString
	database.QueryRow(`SELECT reset_token, reset_token_expires_at FROM users WHERE id = ?`, userID).
		Scan(&resetToken, &resetExpiresAt)

	if !resetToken.Valid || resetToken.String == "" {
		t.Error("expected reset_token to be set in DB")
	}
	if resetToken.String == "old-token" {
		t.Error("expected reset_token to be overwritten")
	}
	if !resetExpiresAt.Valid {
		t.Error("expected reset_token_expires_at to be set")
	}
	expires, _ := time.Parse(time.RFC3339, resetExpiresAt.String)
	if !expires.After(time.Now().UTC()) {
		t.Error("expected reset_token_expires_at to be in the future")
	}
}

func TestHandleUsersIndexActiveStatus(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")
	createTestMember(t, database, "invited@example.com") // no password_hash → inactive

	re := handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/users.html": {
			Data: []byte(`{{define "content"}}{{range .Users}}{{.Email}}:{{.Active}} {{end}}{{end}}`),
		},
	}, nil, "", nil)

	req := httptest.NewRequest("GET", "/users", nil)
	rr := httptest.NewRecorder()
	handlers.HandleUsersIndex(database, re, "8080", func() string { return "127.0.0.1" })(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "admin@example.com:true") {
		t.Errorf("expected admin to be active, got: %s", body)
	}
	if !strings.Contains(body, "invited@example.com:false") {
		t.Errorf("expected invited user without password to be inactive, got: %s", body)
	}
}

func TestHandleUsersDeleteSelf(t *testing.T) {
	database := newTestDB(t)
	adminID := createTestAdmin(t, database, "admin@example.com", "password123")

	now := time.Now().UTC().Format(time.RFC3339)
	database.Exec(
		`INSERT INTO sessions (user_id, token, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		adminID, "self-delete-token", time.Now().Add(12*time.Hour).Format(time.RFC3339), now,
	)

	auth := middleware.NewAuth(database)
	mux := http.NewServeMux()
	mux.Handle("POST /users/{id}/delete", auth.RequireAuth(handlers.HandleUsersDelete(database)))

	req := httptest.NewRequest("POST", fmt.Sprintf("/users/%d/delete", adminID), nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "self-delete-token"})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if rr.Header().Get("Location") != "/login" {
		t.Errorf("expected redirect to /login after self-delete, got: %s", rr.Header().Get("Location"))
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, adminID).Scan(&count)
	if count != 0 {
		t.Errorf("expected admin to be deleted, got count=%d", count)
	}

	// cookie must be cleared
	var cookieCleared bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" && c.MaxAge < 0 {
			cookieCleared = true
		}
	}
	if !cookieCleared {
		t.Error("expected session_id cookie to be cleared")
	}
}

func TestHandleUsersIndexFilterByTeam(t *testing.T) {
	database := newTestDB(t)
	createTestAdmin(t, database, "admin@example.com", "password123")

	team1ID := createTestTeam(t, database, "Frontend")
	team2ID := createTestTeam(t, database, "Backend")

	user1ID := createTestMember(t, database, "alice@example.com")
	user2ID := createTestMember(t, database, "bob@example.com")

	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, team1ID, user1ID)
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, team2ID, user2ID)

	re := newUsersRenderer()

	req := httptest.NewRequest("GET", fmt.Sprintf("/users?team_id=%d", team1ID), nil)
	rr := httptest.NewRecorder()
	handlers.HandleUsersIndex(database, re, "8080", func() string { return "127.0.0.1" })(rr, req)

	bodyStr := rr.Body.String()
	if !strings.Contains(bodyStr, "alice@example.com") {
		t.Errorf("expected alice in filtered result, got: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "bob@example.com") {
		t.Errorf("expected bob to be filtered out, got: %s", bodyStr)
	}
}
