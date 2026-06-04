package handlers_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/vladyslavkondratiuk/postup/internal/handlers"
	"github.com/vladyslavkondratiuk/postup/internal/session"
)

func newRetrosRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/retros_new.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
		"templates/pages/retros_invite.html": {
			Data: []byte(`{{define "content"}}{{.InviteLink}}{{end}}`),
		},
		"templates/pages/retros_edit.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
	})
}

func createTeamWithMembers(t *testing.T, database *sql.DB, teamName string, userIDs []int64) int64 {
	t.Helper()
	teamID := createTestTeam(t, database, teamName)
	for _, uid := range userIDs {
		if _, err := database.Exec(
			`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, uid,
		); err != nil {
			t.Fatalf("add member %d to team %q: %v", uid, teamName, err)
		}
	}
	return teamID
}

func createRetroWithToken(t *testing.T, database *sql.DB, teamID int64, token string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, date, vote_limit, status, invite_token, created_at)
		 VALUES (?, ?, 10, 'active', ?, ?)`,
		teamID, future, token, now,
	)
	if err != nil {
		t.Fatalf("create retro with token: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func postRetros(mux *http.ServeMux, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/retros", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// Тест 1 — POST /retros валідні дані
func TestHandleRetrosCreate_Valid(t *testing.T) {
	database := newTestDB(t)
	member1 := createTestMember(t, database, "alice@example.com")
	member2 := createTestMember(t, database, "bob@example.com")
	teamID := createTeamWithMembers(t, database, "Alpha", []int64{member1, member2})
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros", handlers.HandleRetrosCreate(database, re))

	form := url.Values{}
	form.Set("team_id", fmt.Sprintf("%d", teamID))
	form.Set("template_id", fmt.Sprintf("%d", tmplID))
	form.Set("date", time.Now().Add(24*time.Hour).Format("2006-01-02T15:04"))
	form.Set("vote_limit", "10")
	rr := postRetros(mux, form)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var retroID int64
	if _, err := fmt.Sscanf(rr.Header().Get("Location"), "/retros/%d/invite-link", &retroID); err != nil {
		t.Fatalf("parse retroID from location %q: %v", rr.Header().Get("Location"), err)
	}

	var status string
	var inviteToken sql.NullString
	if err := database.QueryRow(
		`SELECT status, invite_token FROM retros WHERE id = ?`, retroID,
	).Scan(&status, &inviteToken); err != nil {
		t.Fatalf("query retro: %v", err)
	}
	if status != "active" {
		t.Errorf("expected status 'active', got %q", status)
	}
	if !inviteToken.Valid || inviteToken.String == "" {
		t.Error("expected invite_token to be non-NULL and non-empty")
	}

	var count int
	database.QueryRow(
		`SELECT COUNT(*) FROM retro_participants WHERE retro_id = ?`, retroID,
	).Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 participants (all team members), got %d", count)
	}
}

// Тест 2 — POST /retros дата в минулому
func TestHandleRetrosCreate_PastDate(t *testing.T) {
	database := newTestDB(t)
	teamID := createTeamWithMembers(t, database, "Alpha", nil)
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros", handlers.HandleRetrosCreate(database, re))

	form := url.Values{}
	form.Set("team_id", fmt.Sprintf("%d", teamID))
	form.Set("template_id", fmt.Sprintf("%d", tmplID))
	form.Set("date", time.Now().Add(-24*time.Hour).Format("2006-01-02T15:04"))
	form.Set("vote_limit", "10")
	rr := postRetros(mux, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "майбутньому") {
		t.Errorf("expected past-date error in body, got: %s", rr.Body.String())
	}
}

// Тест 3 — POST /retros vote_limit = 0
func TestHandleRetrosCreate_VoteLimitZero(t *testing.T) {
	database := newTestDB(t)
	teamID := createTeamWithMembers(t, database, "Alpha", nil)
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros", handlers.HandleRetrosCreate(database, re))

	form := url.Values{}
	form.Set("team_id", fmt.Sprintf("%d", teamID))
	form.Set("template_id", fmt.Sprintf("%d", tmplID))
	form.Set("date", time.Now().Add(24*time.Hour).Format("2006-01-02T15:04"))
	form.Set("vote_limit", "0")
	rr := postRetros(mux, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "1 до 50") {
		t.Errorf("expected vote_limit error in body, got: %s", rr.Body.String())
	}
}

// Тест 4 — POST /retros vote_limit = 51
func TestHandleRetrosCreate_VoteLimitTooHigh(t *testing.T) {
	database := newTestDB(t)
	teamID := createTeamWithMembers(t, database, "Alpha", nil)
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros", handlers.HandleRetrosCreate(database, re))

	form := url.Values{}
	form.Set("team_id", fmt.Sprintf("%d", teamID))
	form.Set("template_id", fmt.Sprintf("%d", tmplID))
	form.Set("date", time.Now().Add(24*time.Hour).Format("2006-01-02T15:04"))
	form.Set("vote_limit", "51")
	rr := postRetros(mux, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "1 до 50") {
		t.Errorf("expected vote_limit error in body, got: %s", rr.Body.String())
	}
}

// Тест 5 — POST /retros без команди
func TestHandleRetrosCreate_NoTeam(t *testing.T) {
	database := newTestDB(t)
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros", handlers.HandleRetrosCreate(database, re))

	form := url.Values{}
	form.Set("team_id", "0")
	form.Set("template_id", fmt.Sprintf("%d", tmplID))
	form.Set("date", time.Now().Add(24*time.Hour).Format("2006-01-02T15:04"))
	form.Set("vote_limit", "10")
	rr := postRetros(mux, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Оберіть команду") {
		t.Errorf("expected team error in body, got: %s", rr.Body.String())
	}
}

// Тест 6 — GET /retros/:id/invite
func TestHandleRetrosInvite(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Alpha")
	inviteToken := "abc123testtoken456"
	retroID := createRetroWithToken(t, database, teamID, inviteToken)

	re := newRetrosRenderer()
	mux := http.NewServeMux()
	mux.Handle("GET /retros/{id}/invite-link", handlers.HandleRetrosInvite(
		database, re, "8080", func() string { return "127.0.0.1" },
	))

	req := httptest.NewRequest("GET", fmt.Sprintf("/retros/%d/invite-link", retroID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "/join/") {
		t.Errorf("expected '/join/' in body, got: %s", body)
	}
	if !strings.Contains(body, inviteToken) {
		t.Errorf("expected invite token %q in body, got: %s", inviteToken, body)
	}
}

// Тест 7 — GET /retros/invite/:token авторизований user не в учасниках
func TestHandleRetrosJoin_AuthorizedNotParticipant(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Alpha")
	userID := createTestMember(t, database, "member@example.com")
	inviteToken := "join-token-xyz789"
	retroID := createRetroWithToken(t, database, teamID, inviteToken)

	var count int
	database.QueryRow(
		`SELECT COUNT(*) FROM retro_participants WHERE retro_id = ? AND user_id = ?`, retroID, userID,
	).Scan(&count)
	if count != 0 {
		t.Fatalf("pre-condition: expected user not in participants, got count=%d", count)
	}

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /join/{token}", handlers.HandleRetrosJoin(database))

	req := httptest.NewRequest("GET", "/join/"+inviteToken, nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	expectedLoc := fmt.Sprintf("/retros/%d/board", retroID)
	if loc := rr.Header().Get("Location"); loc != expectedLoc {
		t.Errorf("expected redirect to %q, got %q", expectedLoc, loc)
	}

	database.QueryRow(
		`SELECT COUNT(*) FROM retro_participants WHERE retro_id = ? AND user_id = ?`, retroID, userID,
	).Scan(&count)
	if count != 1 {
		t.Errorf("expected user to be added to retro_participants, got count=%d", count)
	}
}

// Тест 8 — GET /retros/invite/:token невалідний токен
func TestHandleRetrosJoin_InvalidToken(t *testing.T) {
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /join/{token}", handlers.HandleRetrosJoin(database))

	req := httptest.NewRequest("GET", "/join/nonexistent-token", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// Тест 9 — POST /retros/:id валідні дані
func TestHandleRetrosUpdate_Valid(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Alpha")
	retroID := createRetroWithToken(t, database, teamID, "tok-update")
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}", handlers.HandleRetrosUpdate(database, re))

	newDate := time.Now().Add(48 * time.Hour).Format("2006-01-02T15:04")
	form := url.Values{}
	form.Set("date", newDate)
	form.Set("vote_limit", "15")
	req := httptest.NewRequest("POST", fmt.Sprintf("/retros/%d", retroID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var voteLimitDB int
	var statusDB string
	database.QueryRow(`SELECT vote_limit, status FROM retros WHERE id = ?`, retroID).Scan(&voteLimitDB, &statusDB)

	if voteLimitDB != 15 {
		t.Errorf("expected vote_limit=15, got %d", voteLimitDB)
	}
	if statusDB != "active" {
		t.Errorf("expected status 'active', got %q", statusDB)
	}
}

// Тест 10 — POST /retros/:id дата в минулому
func TestHandleRetrosUpdate_PastDate(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Alpha")
	retroID := createRetroWithToken(t, database, teamID, "tok-past")
	re := newRetrosRenderer()

	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}", handlers.HandleRetrosUpdate(database, re))

	form := url.Values{}
	form.Set("date", time.Now().Add(-24*time.Hour).Format("2006-01-02T15:04"))
	form.Set("vote_limit", "10")
	req := httptest.NewRequest("POST", fmt.Sprintf("/retros/%d", retroID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "майбутньому") {
		t.Errorf("expected past-date error in body, got: %s", rr.Body.String())
	}
}

// Тест 11 — POST /retros/:id/finish
func TestHandleRetrosFinish(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Alpha")
	retroID := createRetroWithToken(t, database, teamID, "tok-finish")

	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}/finish", handlers.HandleRetrosFinish(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/retros/%d/finish", retroID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var status string
	database.QueryRow(`SELECT status FROM retros WHERE id = ?`, retroID).Scan(&status)
	if status != "finished" {
		t.Errorf("expected status 'finished', got %q", status)
	}
}

func finishRetroMux(database *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}/finish", handlers.HandleRetrosFinish(database))
	return mux
}

func postFinish(mux *http.ServeMux, retroID int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", fmt.Sprintf("/retros/%d/finish", retroID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func insertActionItem(t *testing.T, database *sql.DB, retroID, columnID, assigneeID int64, content string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	var statusID int64
	database.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 1 LIMIT 1`).Scan(&statusID)
	res, err := database.Exec(
		`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		retroID, columnID, assigneeID, content, time.Now().Format("2006-01-02"), statusID, now,
	)
	if err != nil {
		t.Fatalf("insert action item: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// Тест — action items з fixed_last переносяться в fixed_first наступного ретро
func TestHandleRetrosFinish_TransfersActionItems(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	nextRetroID := createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	fixedLastID := fixedLastColumnOf(t, database, tmplID)
	insertActionItem(t, database, retroID, fixedLastID, userID, "Виправити баг")
	insertActionItem(t, database, retroID, fixedLastID, userID, "Написати тести")

	rr := postFinish(finishRetroMux(database), retroID)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 302, got %d", rr.Code)
	}

	fixedFirstID := fixedFirstColumnOf(t, database, tmplID)

	var count int
	database.QueryRow(
		`SELECT COUNT(*) FROM action_items WHERE retro_id = ? AND column_id = ?`,
		nextRetroID, fixedFirstID,
	).Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 action items transferred to next retro, got %d", count)
	}

	// Перевіряємо що оригінальний ретро теж має ці items (не переміщення, а копіювання)
	var origCount int
	database.QueryRow(
		`SELECT COUNT(*) FROM action_items WHERE retro_id = ? AND column_id = ?`,
		retroID, fixedLastID,
	).Scan(&origCount)
	if origCount != 2 {
		t.Errorf("expected original 2 action items to remain in finished retro, got %d", origCount)
	}
}

// Тест — content, assignee_id, deadline, status_id зберігаються при переносі
func TestHandleRetrosFinish_PreservesFields(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	nextRetroID := createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	fixedLastID := fixedLastColumnOf(t, database, tmplID)
	now := time.Now().UTC().Format(time.RFC3339)
	deadline := time.Now().Add(48 * time.Hour).Format("2006-01-02")
	var statusID int64
	database.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 1 LIMIT 1`).Scan(&statusID)
	database.Exec(
		`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		retroID, fixedLastID, userID, "Унікальний контент", deadline, statusID, now,
	)

	postFinish(finishRetroMux(database), retroID)

	fixedFirstID := fixedFirstColumnOf(t, database, tmplID)
	var gotContent, gotDeadline string
	var gotAssignee, gotStatus int64
	err := database.QueryRow(
		`SELECT content, assignee_id, deadline, status_id FROM action_items WHERE retro_id = ? AND column_id = ?`,
		nextRetroID, fixedFirstID,
	).Scan(&gotContent, &gotAssignee, &gotDeadline, &gotStatus)
	if err != nil {
		t.Fatalf("transferred item not found: %v", err)
	}
	if gotContent != "Унікальний контент" {
		t.Errorf("content: got %q, want Унікальний контент", gotContent)
	}
	if gotAssignee != userID {
		t.Errorf("assignee_id: got %d, want %d", gotAssignee, userID)
	}
	if gotDeadline != deadline {
		t.Errorf("deadline: got %q, want %q", gotDeadline, deadline)
	}
	if gotStatus != statusID {
		t.Errorf("status_id: got %d, want %d", gotStatus, statusID)
	}
}

// Тест — немає наступного ретро: завершення без помилки, нічого не переноситься
func TestHandleRetrosFinish_NoNextRetro(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	fixedLastID := fixedLastColumnOf(t, database, tmplID)
	insertActionItem(t, database, retroID, fixedLastID, userID, "Задача")

	rr := postFinish(finishRetroMux(database), retroID)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302 even without next retro, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_items WHERE retro_id != ?`, retroID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no items in other retros, got %d", count)
	}
}

// Тест — немає action items: перенос не відбувається
func TestHandleRetrosFinish_NoActionItems(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	nextRetroID := createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	rr := postFinish(finishRetroMux(database), retroID)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_items WHERE retro_id = ?`, nextRetroID).Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 items in next retro, got %d", count)
	}
}

// Тест 8 — переносяться тільки action items з fixed_last, не з regular колонок
func TestHandleRetrosFinish_OnlyFixedLastTransferred(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре", "Погано"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	nextRetroID := createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	fixedLastID := fixedLastColumnOf(t, database, tmplID)
	regularColID := secondColumnOf(t, database, tmplID) // regular колонка

	// Action item у fixed_last — має перенестись
	insertActionItem(t, database, retroID, fixedLastID, userID, "З fixed_last — переноситься")
	// Action item у regular колонці — НЕ має перенестись
	insertActionItem(t, database, retroID, regularColID, userID, "З regular — не переноситься")

	rr := postFinish(finishRetroMux(database), retroID)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 302, got %d", rr.Code)
	}

	fixedFirstID := fixedFirstColumnOf(t, database, tmplID)

	var count int
	database.QueryRow(
		`SELECT COUNT(*) FROM action_items WHERE retro_id = ? AND column_id = ?`,
		nextRetroID, fixedFirstID,
	).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 action item transferred (from fixed_last), got %d", count)
	}

	// Перевіряємо контент перенесеного item
	var content string
	database.QueryRow(
		`SELECT content FROM action_items WHERE retro_id = ? AND column_id = ?`,
		nextRetroID, fixedFirstID,
	).Scan(&content)
	if content != "З fixed_last — переноситься" {
		t.Errorf("wrong item transferred: got %q", content)
	}
}

// Тест 12 — GET /retros/:id/edit завершеного ретро
func TestHandleRetrosEdit_FinishedRetro(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Alpha")

	now := time.Now().UTC().Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, date, vote_limit, status, created_at) VALUES (?, ?, 10, 'finished', ?)`,
		teamID, past, now,
	)
	if err != nil {
		t.Fatalf("insert finished retro: %v", err)
	}
	retroID, _ := res.LastInsertId()

	re := newRetrosRenderer()
	mux := http.NewServeMux()
	mux.Handle("GET /retros/{id}/edit", handlers.HandleRetrosEdit(database, re))

	req := httptest.NewRequest("GET", fmt.Sprintf("/retros/%d/edit", retroID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected 'error=' in Location, got: %s", loc)
	}
}
