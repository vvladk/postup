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
)

// newStatusesRenderer outputs names separated by ";" for easy order checking.
func newStatusesRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/statuses.html": {
			Data: []byte(`{{define "content"}}{{range .Statuses}}{{.Name}};{{end}}{{end}}`),
		},
	})
}

func createTestStatus(t *testing.T, database *sql.DB, name string, isDefault bool) int64 {
	t.Helper()
	var maxPos int
	database.QueryRow(`SELECT COALESCE(MAX(position), -1) FROM action_statuses`).Scan(&maxPos)
	def := 0
	if isDefault {
		def = 1
	}
	res, err := database.Exec(
		`INSERT INTO action_statuses (name, position, is_default) VALUES (?, ?, ?)`,
		name, maxPos+1, def,
	)
	if err != nil {
		t.Fatalf("create status %q: %v", name, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func createTestRetroForStatus(t *testing.T, database *sql.DB) int64 {
	t.Helper()
	teamID := createTestTeam(t, database, fmt.Sprintf("team-s-%d", time.Now().UnixNano()))
	return createTestRetro(t, database, teamID)
}

// Test 1 — POST /statuses valid data: 302, status in DB with is_default=false.
func TestHandleStatusesCreate_ValidData(t *testing.T) {
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.Handle("POST /statuses", handlers.HandleStatusesCreate(database))

	form := url.Values{}
	form.Set("name", "Заблоковано")
	req := httptest.NewRequest("POST", "/statuses", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var isDefault int
	if err := database.QueryRow(
		`SELECT is_default FROM action_statuses WHERE name = ?`, "Заблоковано",
	).Scan(&isDefault); err != nil {
		t.Fatalf("status not in DB: %v", err)
	}
	if isDefault != 0 {
		t.Errorf("expected is_default=0, got %d", isDefault)
	}
}

// Test 2 — POST /statuses duplicate name: 302 with error, only 1 record in DB.
func TestHandleStatusesCreate_DuplicateName(t *testing.T) {
	database := newTestDB(t)
	// "Новий" already exists from 002_default_statuses migration.

	mux := http.NewServeMux()
	mux.Handle("POST /statuses", handlers.HandleStatusesCreate(database))

	form := url.Values{}
	form.Set("name", "Новий")
	req := httptest.NewRequest("POST", "/statuses", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error= in redirect location, got %q", loc)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_statuses WHERE name = 'Новий'`).Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 'Новий' in DB, got %d", count)
	}
}

// Test 3 — POST /statuses empty name: 302 with error.
func TestHandleStatusesCreate_EmptyName(t *testing.T) {
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.Handle("POST /statuses", handlers.HandleStatusesCreate(database))

	form := url.Values{}
	form.Set("name", "   ")
	req := httptest.NewRequest("POST", "/statuses", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error= in redirect location, got %q", loc)
	}
}

// Test 4 — POST /statuses/:id update: 302, name updated in DB.
func TestHandleStatusesUpdate(t *testing.T) {
	database := newTestDB(t)
	statusID := createTestStatus(t, database, "Старий", false)

	mux := http.NewServeMux()
	mux.Handle("POST /statuses/{id}", handlers.HandleStatusesUpdate(database))

	form := url.Values{}
	form.Set("name", "Оновлений")
	req := httptest.NewRequest("POST", fmt.Sprintf("/statuses/%d", statusID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var name string
	database.QueryRow(`SELECT name FROM action_statuses WHERE id = ?`, statusID).Scan(&name)
	if name != "Оновлений" {
		t.Errorf("expected name='Оновлений', got %q", name)
	}
}

// Test 5 — POST /statuses/:id/delete unused: 302, status absent from DB.
func TestHandleStatusesDelete_Unused(t *testing.T) {
	database := newTestDB(t)
	statusID := createTestStatus(t, database, "Зайвий", false)

	mux := http.NewServeMux()
	mux.Handle("POST /statuses/{id}/delete", handlers.HandleStatusesDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/statuses/%d/delete", statusID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_statuses WHERE id = ?`, statusID).Scan(&count)
	if count != 0 {
		t.Errorf("expected status deleted from DB, got count=%d", count)
	}
}

// Test 6 — POST /statuses/:id/delete used in action_items: 302 with error, status remains.
func TestHandleStatusesDelete_InUse(t *testing.T) {
	database := newTestDB(t)
	statusID := createTestStatus(t, database, "Активний", false)

	userID := createTestMember(t, database, "assignee@test.com")
	retroID := createTestRetroForStatus(t, database)
	now := time.Now().UTC().Format(time.RFC3339)
	database.Exec(
		`INSERT INTO action_items (retro_id, card_id, assignee_id, deadline, status_id, created_at)
		 VALUES (?, NULL, ?, ?, ?, ?)`,
		retroID, userID, now[:10], statusID, now,
	)

	mux := http.NewServeMux()
	mux.Handle("POST /statuses/{id}/delete", handlers.HandleStatusesDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/statuses/%d/delete", statusID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error= in redirect location, got %q", loc)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_statuses WHERE id = ?`, statusID).Scan(&count)
	if count != 1 {
		t.Errorf("expected status to remain in DB, got count=%d", count)
	}
}

// Test 7 — POST /statuses/:id/delete default: 302 with error, status remains.
func TestHandleStatusesDelete_Default(t *testing.T) {
	database := newTestDB(t)

	var defaultID int64
	database.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 1 LIMIT 1`).Scan(&defaultID)

	mux := http.NewServeMux()
	mux.Handle("POST /statuses/{id}/delete", handlers.HandleStatusesDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/statuses/%d/delete", defaultID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error= in redirect location, got %q", loc)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM action_statuses WHERE id = ?`, defaultID).Scan(&count)
	if count != 1 {
		t.Errorf("expected default status to remain in DB, got count=%d", count)
	}
}

// Test 8 — GET /statuses returns statuses in correct order: Новий → В процесі → Виконано → Відмінено.
func TestHandleStatusesIndex_Order(t *testing.T) {
	database := newTestDB(t)
	re := newStatusesRenderer()

	mux := http.NewServeMux()
	mux.Handle("GET /statuses", handlers.HandleStatusesIndex(database, re))

	req := httptest.NewRequest("GET", "/statuses", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	want := []string{"Новий", "В процесі", "Виконано", "Відмінено"}
	cursor := 0
	for _, name := range want {
		idx := strings.Index(body[cursor:], name)
		if idx == -1 {
			t.Errorf("expected %q after position %d in body %q", name, cursor, body)
			return
		}
		cursor += idx + len(name)
	}
}
