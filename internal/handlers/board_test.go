package handlers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/postup-app/postup/internal/handlers"
	"github.com/postup-app/postup/internal/middleware"
	"github.com/postup-app/postup/internal/session"
	"github.com/postup-app/postup/internal/ws"
)

func createRetroWithParticipants(t *testing.T, database *sql.DB, teamID, templateID int64, userIDs []int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'active', ?)`,
		teamID, templateID, future, now,
	)
	if err != nil {
		t.Fatalf("create retro: %v", err)
	}
	retroID, _ := res.LastInsertId()
	for _, uid := range userIDs {
		if _, err := database.Exec(
			`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, uid,
		); err != nil {
			t.Fatalf("add team member %d: %v", uid, err)
		}
	}
	return retroID
}

func createCard(t *testing.T, database *sql.DB, retroID, columnID, authorID int64, content string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO cards (retro_id, column_id, author_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		retroID, columnID, authorID, content, now, now,
	)
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func firstColumnOf(t *testing.T, database *sql.DB, templateID int64) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(
		`SELECT id FROM template_columns WHERE template_id = ? ORDER BY position ASC LIMIT 1`, templateID,
	).Scan(&id); err != nil {
		t.Fatalf("get first column of template %d: %v", templateID, err)
	}
	return id
}

func secondColumnOf(t *testing.T, database *sql.DB, templateID int64) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(
		`SELECT id FROM template_columns WHERE template_id = ? ORDER BY position ASC LIMIT 1 OFFSET 1`, templateID,
	).Scan(&id); err != nil {
		t.Fatalf("get second column of template %d: %v", templateID, err)
	}
	return id
}

func boardMux(database *sql.DB) *http.ServeMux {
	auth := middleware.NewAuth(database)
	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}/cards", auth.RequireAuth(handlers.HandleCardsCreate(database, nil)))
	mux.Handle("POST /cards/{id}", auth.RequireAuth(handlers.HandleCardsUpdate(database, nil)))
	mux.Handle("POST /cards/{id}/delete", auth.RequireAuth(handlers.HandleCardsDelete(database, nil)))
	mux.Handle("POST /cards/{id}/move", auth.RequireAuth(handlers.HandleCardsMoveColumn(database, nil)))
	mux.Handle("POST /cards/{id}/copy", auth.RequireAuth(handlers.HandleCardsCopy(database)))
	mux.Handle("POST /cards/{id}/vote", auth.RequireAuth(handlers.HandleCardsVote(database, nil)))
	return mux
}

func createRetroWithVoteLimit(t *testing.T, database *sql.DB, teamID, templateID int64, voteLimit int, userIDs []int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, ?, 'active', ?)`,
		teamID, templateID, future, voteLimit, now,
	)
	if err != nil {
		t.Fatalf("create retro with vote_limit=%d: %v", voteLimit, err)
	}
	retroID, _ := res.LastInsertId()
	for _, uid := range userIDs {
		if _, err := database.Exec(
			`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, uid,
		); err != nil {
			t.Fatalf("add team member %d: %v", uid, err)
		}
	}
	return retroID
}

func insertVote(t *testing.T, database *sql.DB, cardID, userID int64) {
	t.Helper()
	if _, err := database.Exec(
		`INSERT INTO votes (card_id, user_id, count) VALUES (?, ?, 1)`, cardID, userID,
	); err != nil {
		t.Fatalf("insert vote card=%d user=%d: %v", cardID, userID, err)
	}
}

func createNextRetro(t *testing.T, database *sql.DB, teamID, templateID int64, afterDate time.Time) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	nextDate := afterDate.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'active', ?)`,
		teamID, templateID, nextDate, now,
	)
	if err != nil {
		t.Fatalf("create next retro: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func sessionCookie(t *testing.T, database *sql.DB, userID int64) *http.Cookie {
	t.Helper()
	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session for user %d: %v", userID, err)
	}
	return &http.Cookie{Name: "session_id", Value: sess.Token}
}

func postForm(mux *http.ServeMux, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func postJSON(mux *http.ServeMux, path string, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

// Тест 1 — учасник створює картку (form → 302, картка в БД)
func TestHandleCardsCreate_Participant(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре", "Погано"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	mux := boardMux(database)

	form := url.Values{}
	form.Set("column_id", fmt.Sprintf("%d", colID))
	form.Set("content", "Чудова командна робота!")
	rr := postForm(mux, fmt.Sprintf("/retros/%d/cards", retroID), form, sessionCookie(t, database, userID))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var authorID int64
	var content string
	if err := database.QueryRow(
		`SELECT author_id, content FROM cards WHERE retro_id = ? AND column_id = ?`, retroID, colID,
	).Scan(&authorID, &content); err != nil {
		t.Fatalf("card not found in DB: %v", err)
	}
	if authorID != userID {
		t.Errorf("expected author_id=%d, got %d", userID, authorID)
	}
	if content != "Чудова командна робота!" {
		t.Errorf("unexpected content: %q", content)
	}
}

// Тест 2 — не учасник отримує 403
func TestHandleCardsCreate_NonParticipant(t *testing.T) {
	database := newTestDB(t)
	participantID := createTestMember(t, database, "alice@example.com")
	outsiderID := createTestMember(t, database, "outsider@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{participantID})
	colID := firstColumnOf(t, database, tmplID)
	mux := boardMux(database)

	form := url.Values{}
	form.Set("column_id", fmt.Sprintf("%d", colID))
	form.Set("content", "Спроба")
	rr := postForm(mux, fmt.Sprintf("/retros/%d/cards", retroID), form, sessionCookie(t, database, outsiderID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM cards WHERE retro_id = ?`, retroID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no cards in DB, got %d", count)
	}
}

// Тест 3 — завершене ретро → 403
func TestHandleCardsCreate_FinishedRetro(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	now := time.Now().UTC().Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'finished', ?)`,
		teamID, tmplID, past, now,
	)
	if err != nil {
		t.Fatalf("insert finished retro: %v", err)
	}
	retroID, _ := res.LastInsertId()
	database.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, userID)

	colID := firstColumnOf(t, database, tmplID)
	mux := boardMux(database)

	form := url.Values{}
	form.Set("column_id", fmt.Sprintf("%d", colID))
	form.Set("content", "Спроба")
	rr := postForm(mux, fmt.Sprintf("/retros/%d/cards", retroID), form, sessionCookie(t, database, userID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// Тест 4 — автор редагує свою картку
func TestHandleCardsUpdate_Author(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Стара версія")
	mux := boardMux(database)

	form := url.Values{}
	form.Set("content", "Нова версія")
	rr := postForm(mux, fmt.Sprintf("/cards/%d", cardID), form, sessionCookie(t, database, userID))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var content string
	database.QueryRow(`SELECT content FROM cards WHERE id = ?`, cardID).Scan(&content)
	if content != "Нова версія" {
		t.Errorf("expected updated content, got %q", content)
	}
}

// Тест 5 — не автор намагається редагувати → 403, контент не змінився
func TestHandleCardsUpdate_NonAuthor(t *testing.T) {
	database := newTestDB(t)
	authorID := createTestMember(t, database, "alice@example.com")
	otherID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{authorID, otherID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, authorID, "Оригінал")
	mux := boardMux(database)

	form := url.Values{}
	form.Set("content", "Зламати")
	rr := postForm(mux, fmt.Sprintf("/cards/%d", cardID), form, sessionCookie(t, database, otherID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var content string
	database.QueryRow(`SELECT content FROM cards WHERE id = ?`, cardID).Scan(&content)
	if content != "Оригінал" {
		t.Errorf("expected content unchanged, got %q", content)
	}
}

// Тест 6 — автор видаляє свою картку
func TestHandleCardsDelete_Author(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Видалити мене")
	mux := boardMux(database)

	form := url.Values{}
	rr := postForm(mux, fmt.Sprintf("/cards/%d/delete", cardID), form, sessionCookie(t, database, userID))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM cards WHERE id = ?`, cardID).Scan(&count)
	if count != 0 {
		t.Errorf("expected card to be deleted, found %d", count)
	}
}

// Тест 7 — не автор намагається видалити → 403, картка залишається
func TestHandleCardsDelete_NonAuthor(t *testing.T) {
	database := newTestDB(t)
	authorID := createTestMember(t, database, "alice@example.com")
	otherID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{authorID, otherID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, authorID, "Залишитись")
	mux := boardMux(database)

	form := url.Values{}
	rr := postForm(mux, fmt.Sprintf("/cards/%d/delete", cardID), form, sessionCookie(t, database, otherID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM cards WHERE id = ?`, cardID).Scan(&count)
	if count != 1 {
		t.Errorf("expected card to remain in DB, got count=%d", count)
	}
}

// Тест 8 — автор переносить картку в іншу колонку
func TestHandleCardsMoveColumn_Author(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре", "Погано"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	srcColID := firstColumnOf(t, database, tmplID)
	dstColID := secondColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, srcColID, userID, "Перенести мене")
	mux := boardMux(database)

	body := fmt.Sprintf(`{"column_id":%d}`, dstColID)
	rr := postJSON(mux, fmt.Sprintf("/cards/%d/move", cardID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected {ok: true}, got %v", resp)
	}

	var colID int64
	database.QueryRow(`SELECT column_id FROM cards WHERE id = ?`, cardID).Scan(&colID)
	if colID != dstColID {
		t.Errorf("expected column_id=%d, got %d", dstColID, colID)
	}
}

// Тест 9 — не автор намагається перенести → 403, column_id не змінився
func TestHandleCardsMoveColumn_NonAuthor(t *testing.T) {
	database := newTestDB(t)
	authorID := createTestMember(t, database, "alice@example.com")
	otherID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре", "Погано"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{authorID, otherID})
	srcColID := firstColumnOf(t, database, tmplID)
	dstColID := secondColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, srcColID, authorID, "Стоп")
	mux := boardMux(database)

	body := fmt.Sprintf(`{"column_id":%d}`, dstColID)
	rr := postJSON(mux, fmt.Sprintf("/cards/%d/move", cardID), body, sessionCookie(t, database, otherID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var colID int64
	database.QueryRow(`SELECT column_id FROM cards WHERE id = ?`, cardID).Scan(&colID)
	if colID != srcColID {
		t.Errorf("expected column_id unchanged (%d), got %d", srcColID, colID)
	}
}

// Тест 10 — автор намагається перенести в колонку іншого ретро → 400
func TestHandleCardsMoveColumn_ForeignRetroColumn(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")

	teamID := createTestTeam(t, database, "Alpha")
	tmpl1ID := createTemplate(t, database, "Template1", []string{"Добре"})
	tmpl2ID := createTemplate(t, database, "Template2", []string{"Добре"})

	retro1ID := createRetroWithParticipants(t, database, teamID, tmpl1ID, []int64{userID})
	// retro2 використовує інший шаблон — колонки ніяк не повʼязані з retro1
	createRetroWithParticipants(t, database, teamID, tmpl2ID, []int64{userID})

	srcColID := firstColumnOf(t, database, tmpl1ID)
	foreignColID := firstColumnOf(t, database, tmpl2ID)
	cardID := createCard(t, database, retro1ID, srcColID, userID, "Картка")
	mux := boardMux(database)

	body := fmt.Sprintf(`{"column_id":%d}`, foreignColID)
	rr := postJSON(mux, fmt.Sprintf("/cards/%d/move", cardID), body, sessionCookie(t, database, userID))

	if rr.Code == http.StatusOK {
		t.Errorf("expected error status, got 200")
	}

	var colID int64
	database.QueryRow(`SELECT column_id FROM cards WHERE id = ?`, cardID).Scan(&colID)
	if colID != srcColID {
		t.Errorf("expected column_id unchanged (%d), got %d", srcColID, colID)
	}
}

// Тест 11 — послідовні переміщення A→B→C
func TestHandleCardsMoveColumn_Sequential(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре", "Покращити"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	mux := boardMux(database)
	cookie := sessionCookie(t, database, userID)

	// createTemplate додає: fixed_first + 2 regular + fixed_last → 4 колонки
	var colIDs []int64
	rows, err := database.Query(
		`SELECT id FROM template_columns WHERE template_id = ? ORDER BY position ASC`, tmplID,
	)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		colIDs = append(colIDs, id)
	}
	if len(colIDs) < 3 {
		t.Fatalf("expected at least 3 columns, got %d", len(colIDs))
	}
	colA, colB, colC := colIDs[0], colIDs[1], colIDs[2]

	cardID := createCard(t, database, retroID, colA, userID, "Мандрівна картка")

	// A → B
	rr := postJSON(mux, fmt.Sprintf("/cards/%d/move", cardID),
		fmt.Sprintf(`{"column_id":%d}`, colB), cookie)
	if rr.Code != http.StatusOK {
		t.Errorf("move A→B: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var currentColID int64
	database.QueryRow(`SELECT column_id FROM cards WHERE id = ?`, cardID).Scan(&currentColID)
	if currentColID != colB {
		t.Errorf("after A→B: expected column_id=%d, got %d", colB, currentColID)
	}

	// B → C
	rr = postJSON(mux, fmt.Sprintf("/cards/%d/move", cardID),
		fmt.Sprintf(`{"column_id":%d}`, colC), cookie)
	if rr.Code != http.StatusOK {
		t.Errorf("move B→C: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	database.QueryRow(`SELECT column_id FROM cards WHERE id = ?`, cardID).Scan(&currentColID)
	if currentColID != colC {
		t.Errorf("after B→C: expected column_id=%d, got %d", colC, currentColID)
	}
}

// Тест 12 — переміщення в ту саму колонку (idempotent)
func TestHandleCardsMoveColumn_SameColumn(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Нікуди не рухаюсь")
	mux := boardMux(database)

	body := fmt.Sprintf(`{"column_id":%d}`, colID)
	rr := postJSON(mux, fmt.Sprintf("/cards/%d/move", cardID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var currentColID int64
	database.QueryRow(`SELECT column_id FROM cards WHERE id = ?`, cardID).Scan(&currentColID)
	if currentColID != colID {
		t.Errorf("expected column_id unchanged (%d), got %d", colID, currentColID)
	}
}

// currentRetroDate повертає дату ретро з БД.
func currentRetroDate(t *testing.T, database *sql.DB, retroID int64) time.Time {
	t.Helper()
	var dateStr string
	database.QueryRow(`SELECT date FROM retros WHERE id = ?`, retroID).Scan(&dateStr)
	d, _ := time.Parse(time.RFC3339, dateStr)
	return d
}

// fixedFirstColumnOf повертає id fixed_first колонки шаблону.
func fixedFirstColumnOf(t *testing.T, database *sql.DB, templateID int64) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(
		`SELECT id FROM template_columns WHERE template_id = ? AND type = 'fixed_first'`, templateID,
	).Scan(&id); err != nil {
		t.Fatalf("get fixed_first column of template %d: %v", templateID, err)
	}
	return id
}

// Тест 13 — картка копіюється в колонку з такою самою назвою
func TestHandleCardsCopy_SameColumnName(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	nextRetroID := createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	// Знайти колонку "Добре" в шаблоні
	var goodColID int64
	database.QueryRow(
		`SELECT id FROM template_columns WHERE template_id = ? AND title = 'Добре'`, tmplID,
	).Scan(&goodColID)

	cardID := createCard(t, database, retroID, goodColID, userID, "Чудова робота")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/copy", cardID), "", sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected {ok: true}, got %v", resp)
	}

	// Нова картка в наступному ретро в колонці "Добре" (той самий column_id, бо той самий шаблон)
	var copiedRetroID, copiedColID int64
	if err := database.QueryRow(
		`SELECT retro_id, column_id FROM cards WHERE retro_id = ? AND content = 'Чудова робота'`,
		nextRetroID,
	).Scan(&copiedRetroID, &copiedColID); err != nil {
		t.Fatalf("copied card not found in DB: %v", err)
	}
	if copiedRetroID != nextRetroID {
		t.Errorf("expected retro_id=%d, got %d", nextRetroID, copiedRetroID)
	}
	if copiedColID != goodColID {
		t.Errorf("expected column_id=%d (same-name column), got %d", goodColID, copiedColID)
	}
}

// Тест 14 — колонки з такою назвою немає → копія в fixed_first
func TestHandleCardsCopy_FallbackToFixedFirst(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmpl1ID := createTemplate(t, database, "Template1", []string{"Добре"})
	tmpl2ID := createTemplate(t, database, "Template2", []string{"Погано"}) // немає "Добре"

	retroID := createRetroWithParticipants(t, database, teamID, tmpl1ID, []int64{userID})
	nextRetroID := createNextRetro(t, database, teamID, tmpl2ID, currentRetroDate(t, database, retroID))

	var goodColID int64
	database.QueryRow(
		`SELECT id FROM template_columns WHERE template_id = ? AND title = 'Добре'`, tmpl1ID,
	).Scan(&goodColID)

	cardID := createCard(t, database, retroID, goodColID, userID, "Треба перенести")
	expectedColID := fixedFirstColumnOf(t, database, tmpl2ID)
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/copy", cardID), "", sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var copiedColID int64
	if err := database.QueryRow(
		`SELECT column_id FROM cards WHERE retro_id = ? AND content = 'Треба перенести'`,
		nextRetroID,
	).Scan(&copiedColID); err != nil {
		t.Fatalf("copied card not found in DB: %v", err)
	}
	if copiedColID != expectedColID {
		t.Errorf("expected fixed_first column_id=%d, got %d", expectedColID, copiedColID)
	}
}

// Тест 15 — наступного ретро немає → {error: ...}
func TestHandleCardsCopy_NoNextRetro(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Нікуди")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/copy", cardID), "", sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] == nil {
		t.Errorf("expected error field in response, got %v", resp)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM cards WHERE id != ?`, cardID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no extra cards in DB, got %d", count)
	}
}

// Тест 16 — контент зберігається, author_id == currentUser (не оригінальний автор)
func TestHandleCardsCopy_ContentAndAuthor(t *testing.T) {
	database := newTestDB(t)
	authorID := createTestMember(t, database, "alice@example.com")
	copierID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Покращити"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{authorID, copierID})
	nextRetroID := createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, authorID, "Треба покращити деплой")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/copy", cardID), "", sessionCookie(t, database, copierID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var copiedAuthorID int64
	var copiedContent string
	if err := database.QueryRow(
		`SELECT author_id, content FROM cards WHERE retro_id = ?`, nextRetroID,
	).Scan(&copiedAuthorID, &copiedContent); err != nil {
		t.Fatalf("copied card not found: %v", err)
	}
	if copiedContent != "Треба покращити деплой" {
		t.Errorf("expected original content, got %q", copiedContent)
	}
	if copiedAuthorID != copierID {
		t.Errorf("expected author_id=%d (copier), got %d", copierID, copiedAuthorID)
	}
}

// Тест 17 — не учасник → 403, нова картка не створена
func TestHandleCardsCopy_NonParticipant(t *testing.T) {
	database := newTestDB(t)
	participantID := createTestMember(t, database, "alice@example.com")
	outsiderID := createTestMember(t, database, "outsider@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{participantID})
	createNextRetro(t, database, teamID, tmplID, currentRetroDate(t, database, retroID))

	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, participantID, "Закрита картка")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/copy", cardID), "", sessionCookie(t, database, outsiderID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM cards WHERE id != ?`, cardID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no extra cards in DB, got %d", count)
	}
}

// ─── Action item tests ────────────────────────────────────────────────────────

func fixedLastColumnOf(t *testing.T, database *sql.DB, templateID int64) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(
		`SELECT id FROM template_columns WHERE template_id = ? AND type = 'fixed_last'`, templateID,
	).Scan(&id); err != nil {
		t.Fatalf("get fixed_last column of template %d: %v", templateID, err)
	}
	return id
}

func defaultStatusID(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := database.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 1 LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("get default status: %v", err)
	}
	return id
}

func actionItemsMux(database *sql.DB) *http.ServeMux {
	auth := middleware.NewAuth(database)
	hub := ws.NewHub()
	go hub.Run()
	mux := http.NewServeMux()
	mux.Handle("POST /retros/{id}/action-items", auth.RequireAuth(handlers.HandleActionItemsCreate(database, hub)))
	mux.Handle("POST /action-items/{id}/status", auth.RequireAuth(handlers.HandleActionItemsUpdateStatus(database, hub)))
	return mux
}

func todayDeadline() string {
	return time.Now().UTC().Format("2006-01-02")
}

// Тест AI-1 — учасник створює action item → 200 JSON, запис в БД
// Тест 1 — валідні дані: 200 JSON, поля в БД коректні
func TestHandleActionItemsCreate_Participant(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	deadline := todayDeadline()
	body := fmt.Sprintf(`{"content":"Виправити баг","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, deadline, colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["content"] != "Виправити баг" {
		t.Errorf("response content: got %v, want Виправити баг", resp["content"])
	}

	// Verify DB fields: assignee_id, deadline, default status_id
	wantStatus := defaultStatusID(t, database)
	var gotAssigneeID int64
	var gotDeadline string
	var gotStatusID int64
	err := database.QueryRow(
		`SELECT assignee_id, deadline, status_id FROM action_items WHERE retro_id = ? AND content = 'Виправити баг'`,
		retroID,
	).Scan(&gotAssigneeID, &gotDeadline, &gotStatusID)
	if err != nil {
		t.Fatalf("action item not in DB: %v", err)
	}
	if gotAssigneeID != userID {
		t.Errorf("DB assignee_id: got %d, want %d", gotAssigneeID, userID)
	}
	if gotDeadline != deadline {
		t.Errorf("DB deadline: got %q, want %q", gotDeadline, deadline)
	}
	if gotStatusID != wantStatus {
		t.Errorf("DB status_id: got %d, want default %d", gotStatusID, wantStatus)
	}
}

// Тест AI-2 — не учасник → 403
func TestHandleActionItemsCreate_NonParticipant(t *testing.T) {
	database := newTestDB(t)
	participantID := createTestMember(t, database, "alice@example.com")
	outsiderID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{participantID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"Test","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		participantID, todayDeadline(), colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, outsiderID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// Тест AI-3 — завершене ретро → 403
func TestHandleActionItemsCreate_FinishedRetro(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	now := time.Now().UTC().Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	res, _ := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'finished', ?)`,
		teamID, tmplID, past, now,
	)
	retroID, _ := res.LastInsertId()
	database.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, userID)
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"Test","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, todayDeadline(), colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// Тест AI-4 — assignee_id не є учасником → 400
func TestHandleActionItemsCreate_NonParticipantAssignee(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	outsiderID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"Test","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		outsiderID, todayDeadline(), colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// Тест AI-5 — порожній content → 400
func TestHandleActionItemsCreate_EmptyContent(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, todayDeadline(), colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// Тест AI-6 — дедлайн в минулому → 400
func TestHandleActionItemsCreate_PastDeadline(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	past := time.Now().Add(-48 * time.Hour).Format("2006-01-02")
	body := fmt.Sprintf(`{"content":"Test","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, past, colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// Тест 5 — порожній deadline → 400
func TestHandleActionItemsCreate_EmptyDeadline(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"Задача","assignee_id":%d,"deadline":"","column_id":%d}`, userID, colID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty deadline, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_items WHERE retro_id = ?`, retroID).Scan(&count)
	if count != 0 {
		t.Errorf("expected no action items in DB, got %d", count)
	}
}

// Тест AI-7 — невалідний column_id → 400
func TestHandleActionItemsCreate_InvalidColumn(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	tmpl2ID := createTemplate(t, database, "Other", []string{"Погано"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	foreignColID := fixedLastColumnOf(t, database, tmpl2ID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"Test","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, todayDeadline(), foreignColID)
	rr := postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// Тест AI-8 — дефолтний статус встановлюється автоматично
func TestHandleActionItemsCreate_DefaultStatus(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)
	mux := actionItemsMux(database)

	body := fmt.Sprintf(`{"content":"Задача","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, todayDeadline(), colID)
	postJSON(mux, fmt.Sprintf("/retros/%d/action-items", retroID), body, sessionCookie(t, database, userID))

	wantStatusID := defaultStatusID(t, database)
	var gotStatusID int64
	database.QueryRow(`SELECT status_id FROM action_items WHERE retro_id = ?`, retroID).Scan(&gotStatusID)
	if gotStatusID != wantStatusID {
		t.Errorf("expected default status_id=%d, got %d", wantStatusID, gotStatusID)
	}
}

// Тест 9 — WS broadcast при створенні action item
func TestHandleActionItemsCreate_WSBroadcast(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)

	hub := ws.NewHub()
	go hub.Run()

	// WS listener — user 999 (окремий клієнт, отримає broadcast)
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &ws.Client{
			Hub:     hub,
			Conn:    conn,
			Send:    make(chan []byte, 256),
			RetroID: retroID,
			UserID:  999,
		}
		hub.Register(client)
		go client.WritePump()
		go client.ReadPump()
	}))
	defer wsSrv.Close()

	// HTTP сервер для POST action item
	auth := middleware.NewAuth(database)
	aiSrvMux := http.NewServeMux()
	aiSrvMux.Handle("POST /retros/{id}/action-items",
		auth.RequireAuth(handlers.HandleActionItemsCreate(database, hub)))
	aiSrv := httptest.NewServer(aiSrvMux)
	defer aiSrv.Close()

	// Підключаємо WS клієнта
	wsConn, _, err := websocket.DefaultDialer.Dial("ws"+wsSrv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial WS: %v", err)
	}
	defer wsConn.Close()

	// Чекаємо реєстрацію в hub
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.RoomSize(retroID) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.RoomSize(retroID) != 1 {
		t.Fatal("WS client not registered within 2s")
	}

	// POST action item
	body := fmt.Sprintf(`{"content":"Запустити CI","assignee_id":%d,"deadline":"%s","column_id":%d}`,
		userID, todayDeadline(), colID)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/retros/%d/action-items", aiSrv.URL, retroID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, database, userID))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST action item: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Читаємо WS повідомлення
	wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read WS message: %v", err)
	}

	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal WS message: %v", err)
	}

	if msg["type"] != "action_item_created" {
		t.Errorf("expected type=action_item_created, got %v", msg["type"])
	}

	payload, ok := msg["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", msg["payload"])
	}
	if payload["content"] != "Запустити CI" {
		t.Errorf("expected payload.content=Запустити CI, got %v", payload["content"])
	}
	if payload["assignee_first_name"] == nil && payload["assignee_last_name"] == nil {
		t.Errorf("expected assignee name in payload, got %v", payload)
	}
	if payload["deadline"] != todayDeadline() {
		t.Errorf("expected payload.deadline=%s, got %v", todayDeadline(), payload["deadline"])
	}
}

// Тест AI-9 — учасник змінює статус → 200, БД оновлена
func TestHandleActionItemsUpdateStatus_Participant(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)

	// Insert action item directly
	now := time.Now().UTC().Format(time.RFC3339)
	defStatus := defaultStatusID(t, database)
	res, _ := database.Exec(
		`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		retroID, colID, userID, "Задача", todayDeadline(), defStatus, now,
	)
	itemID, _ := res.LastInsertId()

	// Find a non-default status
	var otherStatusID int64
	database.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 0 LIMIT 1`).Scan(&otherStatusID)
	if otherStatusID == 0 {
		t.Skip("no non-default status to switch to")
	}

	mux := actionItemsMux(database)
	body := fmt.Sprintf(`{"status_id":%d}`, otherStatusID)
	rr := postJSON(mux, fmt.Sprintf("/action-items/%d/status", itemID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected {ok: true} in response, got %v", resp)
	}
	if resp["status_name"] == nil || resp["status_name"] == "" {
		t.Errorf("expected status_name in response, got %v", resp)
	}

	var gotStatusID int64
	database.QueryRow(`SELECT status_id FROM action_items WHERE id = ?`, itemID).Scan(&gotStatusID)
	if gotStatusID != otherStatusID {
		t.Errorf("DB status_id: got %d, want %d", otherStatusID, gotStatusID)
	}
}

// Тест AI-10 — не учасник не може змінити статус → 403
func TestHandleActionItemsUpdateStatus_NonParticipant(t *testing.T) {
	database := newTestDB(t)
	participantID := createTestMember(t, database, "alice@example.com")
	outsiderID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{participantID})
	colID := fixedLastColumnOf(t, database, tmplID)

	now := time.Now().UTC().Format(time.RFC3339)
	defStatus := defaultStatusID(t, database)
	res, _ := database.Exec(
		`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		retroID, colID, participantID, "Задача", todayDeadline(), defStatus, now,
	)
	itemID, _ := res.LastInsertId()

	mux := actionItemsMux(database)
	body := fmt.Sprintf(`{"status_id":%d}`, defStatus)
	rr := postJSON(mux, fmt.Sprintf("/action-items/%d/status", itemID), body, sessionCookie(t, database, outsiderID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// Тест 3 — неіснуючий status_id → 400
func TestHandleActionItemsUpdateStatus_InvalidStatusID(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)

	now := time.Now().UTC().Format(time.RFC3339)
	defStatus := defaultStatusID(t, database)
	res, _ := database.Exec(
		`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		retroID, colID, userID, "Задача", todayDeadline(), defStatus, now,
	)
	itemID, _ := res.LastInsertId()

	mux := actionItemsMux(database)
	body := `{"status_id":999999}`
	rr := postJSON(mux, fmt.Sprintf("/action-items/%d/status", itemID), body, sessionCookie(t, database, userID))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid status_id, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// DB не оновилася
	var gotStatusID int64
	database.QueryRow(`SELECT status_id FROM action_items WHERE id = ?`, itemID).Scan(&gotStatusID)
	if gotStatusID != defStatus {
		t.Errorf("status_id should not change: got %d, want %d", gotStatusID, defStatus)
	}
}

// Тест 4 — WS broadcast при зміні статусу
func TestHandleActionItemsUpdateStatus_WSBroadcast(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := fixedLastColumnOf(t, database, tmplID)

	now := time.Now().UTC().Format(time.RFC3339)
	defStatus := defaultStatusID(t, database)
	res, _ := database.Exec(
		`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		retroID, colID, userID, "Задача", todayDeadline(), defStatus, now,
	)
	itemID, _ := res.LastInsertId()

	var otherStatusID int64
	database.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 0 LIMIT 1`).Scan(&otherStatusID)
	if otherStatusID == 0 {
		t.Skip("no non-default status available")
	}

	hub := ws.NewHub()
	go hub.Run()

	// WS listener — user 999
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &ws.Client{
			Hub:     hub,
			Conn:    conn,
			Send:    make(chan []byte, 256),
			RetroID: retroID,
			UserID:  999,
		}
		hub.Register(client)
		go client.WritePump()
		go client.ReadPump()
	}))
	defer wsSrv.Close()

	// HTTP сервер для PATCH статусу
	auth := middleware.NewAuth(database)
	statusSrvMux := http.NewServeMux()
	statusSrvMux.Handle("POST /action-items/{id}/status",
		auth.RequireAuth(handlers.HandleActionItemsUpdateStatus(database, hub)))
	statusSrv := httptest.NewServer(statusSrvMux)
	defer statusSrv.Close()

	// Підключаємо WS клієнта
	wsConn, _, err := websocket.DefaultDialer.Dial("ws"+wsSrv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial WS: %v", err)
	}
	defer wsConn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.RoomSize(retroID) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.RoomSize(retroID) != 1 {
		t.Fatal("WS client not registered within 2s")
	}

	// POST зміна статусу
	body := fmt.Sprintf(`{"status_id":%d}`, otherStatusID)
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/action-items/%d/status", statusSrv.URL, itemID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, database, userID))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Читаємо WS повідомлення
	wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read WS message: %v", err)
	}

	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal WS message: %v", err)
	}
	if msg["type"] != "action_item_status_updated" {
		t.Errorf("expected type=action_item_status_updated, got %v", msg["type"])
	}
	payload, ok := msg["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", msg["payload"])
	}
	if payload["id"].(float64) != float64(itemID) {
		t.Errorf("payload.id: got %v, want %d", payload["id"], itemID)
	}
	if payload["status_id"].(float64) != float64(otherStatusID) {
		t.Errorf("payload.status_id: got %v, want %d", payload["status_id"], otherStatusID)
	}
	if payload["status_name"] == nil || payload["status_name"] == "" {
		t.Errorf("expected status_name in payload, got %v", payload)
	}
}

// ─── Vote tests ───────────────────────────────────────────────────────────────

// Тест 1 — перший голос: 200, total_votes=1, votes_remaining=9, запис в БД
func TestHandleCardsVote_FirstVote(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Картка")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
	if resp["total_votes"].(float64) != 1 {
		t.Errorf("expected total_votes=1, got %v", resp["total_votes"])
	}
	if resp["votes_remaining"].(float64) != 9 {
		t.Errorf("expected votes_remaining=9, got %v", resp["votes_remaining"])
	}

	var dbCount int
	database.QueryRow(`SELECT count FROM votes WHERE card_id = ? AND user_id = ?`, cardID, userID).Scan(&dbCount)
	if dbCount != 1 {
		t.Errorf("expected vote in DB with count=1, got %d", dbCount)
	}
}

// Тест 2 — повторний голос (toggle off): votes_remaining повертається, запис видалений
func TestHandleCardsVote_ToggleOff(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Картка")
	mux := boardMux(database)
	cookie := sessionCookie(t, database, userID)

	postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", cookie)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", cookie)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["votes_remaining"].(float64) != 10 {
		t.Errorf("expected votes_remaining=10 after toggle, got %v", resp["votes_remaining"])
	}

	var dbCount int
	database.QueryRow(`SELECT COUNT(*) FROM votes WHERE card_id = ? AND user_id = ?`, cardID, userID).Scan(&dbCount)
	if dbCount != 0 {
		t.Errorf("expected vote deleted from DB, got count=%d", dbCount)
	}
}

// Тест 3 — ліміт вичерпано: повертає {error: ...}, нового запису в БД немає
func TestHandleCardsVote_LimitExceeded(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"A", "B", "C", "D"})
	retroID := createRetroWithVoteLimit(t, database, teamID, tmplID, 3, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	mux := boardMux(database)
	cookie := sessionCookie(t, database, userID)

	for i := 0; i < 3; i++ {
		cid := createCard(t, database, retroID, colID, userID, fmt.Sprintf("Картка %d", i))
		rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cid), "", cookie)
		if rr.Code != http.StatusOK {
			t.Fatalf("vote %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	cardID := createCard(t, database, retroID, colID, userID, "Четверта")
	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", cookie)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with error body, got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["error"] == nil {
		t.Errorf("expected error field, got %v", resp)
	}

	var dbCount int
	database.QueryRow(`SELECT COUNT(*) FROM votes WHERE card_id = ?`, cardID).Scan(&dbCount)
	if dbCount != 0 {
		t.Errorf("expected no vote for 4th card in DB, got %d", dbCount)
	}
}

// Тест 4 — будь-який учасник може голосувати за чужу картку
func TestHandleCardsVote_AnyParticipant(t *testing.T) {
	database := newTestDB(t)
	authorID := createTestMember(t, database, "alice@example.com")
	voterID := createTestMember(t, database, "bob@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{authorID, voterID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, authorID, "Чужа картка")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", sessionCookie(t, database, voterID))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
}

// Тест 5 — не учасник ретро → 403
func TestHandleCardsVote_NonParticipant(t *testing.T) {
	database := newTestDB(t)
	authorID := createTestMember(t, database, "alice@example.com")
	outsiderID := createTestMember(t, database, "outsider@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{authorID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, authorID, "Картка")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", sessionCookie(t, database, outsiderID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var dbCount int
	database.QueryRow(`SELECT COUNT(*) FROM votes WHERE card_id = ?`, cardID).Scan(&dbCount)
	if dbCount != 0 {
		t.Errorf("expected no vote in DB, got %d", dbCount)
	}
}

// Тест 6 — завершене ретро → 403
func TestHandleCardsVote_FinishedRetro(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})

	now := time.Now().UTC().Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'finished', ?)`,
		teamID, tmplID, past, now,
	)
	if err != nil {
		t.Fatalf("create finished retro: %v", err)
	}
	retroID, _ := res.LastInsertId()
	database.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, userID)

	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Картка")
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardID), "", sessionCookie(t, database, userID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// Тест 7 — votes_remaining коректно рахується з урахуванням попередніх голосів
func TestHandleCardsVote_VotesRemainingCorrect(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре", "Погано"})
	retroID := createRetroWithVoteLimit(t, database, teamID, tmplID, 5, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)

	cardA := createCard(t, database, retroID, colID, userID, "A")
	cardB := createCard(t, database, retroID, colID, userID, "B")
	cardC := createCard(t, database, retroID, colID, userID, "C")
	insertVote(t, database, cardA, userID)
	insertVote(t, database, cardB, userID)
	mux := boardMux(database)

	rr := postJSON(mux, fmt.Sprintf("/cards/%d/vote", cardC), "", sessionCookie(t, database, userID))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// vote_limit=5, used=3 (A+B+C), remaining=2
	if resp["votes_remaining"].(float64) != 2 {
		t.Errorf("expected votes_remaining=2, got %v", resp["votes_remaining"])
	}
}

// Тест 8 — WS broadcast: POST vote → клієнт отримує type="vote_updated"
func TestHandleCardsVote_WSBroadcast(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "alice@example.com")
	teamID := createTestTeam(t, database, "Alpha")
	tmplID := createTemplate(t, database, "Default", []string{"Добре"})
	retroID := createRetroWithParticipants(t, database, teamID, tmplID, []int64{userID})
	colID := firstColumnOf(t, database, tmplID)
	cardID := createCard(t, database, retroID, colID, userID, "Картка")

	hub := ws.NewHub()
	go hub.Run()

	// WS listener — userID 999 (не постер, отримає broadcast)
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := ws.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &ws.Client{
			Hub:     hub,
			Conn:    conn,
			Send:    make(chan []byte, 256),
			RetroID: retroID,
			UserID:  999,
		}
		hub.Register(client)
		go client.WritePump()
		go client.ReadPump()
	}))
	defer wsSrv.Close()

	auth := middleware.NewAuth(database)
	voteSrvMux := http.NewServeMux()
	voteSrvMux.Handle("POST /cards/{id}/vote", auth.RequireAuth(handlers.HandleCardsVote(database, hub)))
	voteSrv := httptest.NewServer(voteSrvMux)
	defer voteSrv.Close()

	wsConn, _, err := websocket.DefaultDialer.Dial("ws"+wsSrv.URL[4:], nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer wsConn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.RoomSize(retroID) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.RoomSize(retroID) != 1 {
		t.Fatal("WS client not registered within 2s")
	}

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/cards/%d/vote", voteSrv.URL, cardID),
		strings.NewReader(""))
	req.Header.Set("Accept", "application/json")
	req.AddCookie(sessionCookie(t, database, userID))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST vote: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("read WS message: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg["type"] != "vote_updated" {
		t.Errorf("expected type=vote_updated, got %v", msg["type"])
	}
	payload, ok := msg["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got %T", msg["payload"])
	}
	if payload["card_id"].(float64) != float64(cardID) {
		t.Errorf("expected card_id=%d, got %v", cardID, payload["card_id"])
	}
	if payload["total_votes"].(float64) != 1 {
		t.Errorf("expected total_votes=1, got %v", payload["total_votes"])
	}
}
