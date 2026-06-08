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

	"github.com/postup-app/postup/internal/handlers"
)

func createTemplate(t *testing.T, database *sql.DB, name string, regularColumns []string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(`INSERT INTO templates (name, status, created_at) VALUES (?, 'active', ?)`, name, now)
	if err != nil {
		t.Fatalf("create template %q: %v", name, err)
	}
	id, _ := res.LastInsertId()

	database.Exec(
		`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, 'Action items', 0, 'fixed_first')`,
		id,
	)
	for i, col := range regularColumns {
		database.Exec(
			`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, ?, ?, 'regular')`,
			id, col, i+1,
		)
	}
	lastPos := len(regularColumns) + 1
	database.Exec(
		`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, 'Нові дії', ?, 'fixed_last')`,
		id, lastPos,
	)
	return id
}

func createRetroForTemplate(t *testing.T, database *sql.DB, templateID int64) int64 {
	t.Helper()
	teamID := createTestTeam(t, database, fmt.Sprintf("team-%d", templateID))
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(
		`INSERT INTO retros (team_id, template_id, date, vote_limit, status, created_at) VALUES (?, ?, ?, 10, 'active', ?)`,
		teamID, templateID, now[:10], now,
	)
	if err != nil {
		t.Fatalf("create retro for template %d: %v", templateID, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func newTemplatesEditRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/templates_edit.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
	}, nil, "", nil)
}

func newTemplatesNewRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/templates_new.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
	}, nil, "", nil)
}

func postTemplates(t *testing.T, re *handlers.Renderer, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	database := newTestDB(t)
	mux := http.NewServeMux()
	mux.Handle("POST /templates", handlers.HandleTemplatesCreate(database, re))

	req := httptest.NewRequest("POST", "/templates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestHandleTemplatesCreate_OneColumn(t *testing.T) {
	re := newTemplatesNewRenderer()
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.Handle("POST /templates", handlers.HandleTemplatesCreate(database, re))

	form := url.Values{}
	form.Set("name", "Sprint Retro")
	form.Add("columns[]", "Добре")
	req := httptest.NewRequest("POST", "/templates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var status string
	if err := database.QueryRow(`SELECT status FROM templates WHERE name = ?`, "Sprint Retro").Scan(&status); err != nil {
		t.Fatalf("template not in DB: %v", err)
	}
	if status != "active" {
		t.Errorf("expected status 'active', got %q", status)
	}

	rows, err := database.Query(`SELECT position, type FROM template_columns ORDER BY position`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()

	type colRow struct {
		position int
		colType  string
	}
	var cols []colRow
	for rows.Next() {
		var c colRow
		rows.Scan(&c.position, &c.colType)
		cols = append(cols, c)
	}

	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	if cols[0].position != 0 || cols[0].colType != "fixed_first" {
		t.Errorf("col[0]: want position=0 type=fixed_first, got position=%d type=%s", cols[0].position, cols[0].colType)
	}
	if cols[1].position != 1 || cols[1].colType != "regular" {
		t.Errorf("col[1]: want position=1 type=regular, got position=%d type=%s", cols[1].position, cols[1].colType)
	}
	if cols[2].position != 2 || cols[2].colType != "fixed_last" {
		t.Errorf("col[2]: want position=2 type=fixed_last, got position=%d type=%s", cols[2].position, cols[2].colType)
	}
}

func TestHandleTemplatesCreate_FiveColumns(t *testing.T) {
	re := newTemplatesNewRenderer()
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.Handle("POST /templates", handlers.HandleTemplatesCreate(database, re))

	form := url.Values{}
	form.Set("name", "Big Retro")
	for _, col := range []string{"Добре", "Погано", "Покращити", "Ідеї", "Питання"} {
		form.Add("columns[]", col)
	}
	req := httptest.NewRequest("POST", "/templates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	rows, err := database.Query(`SELECT position, type FROM template_columns ORDER BY position`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()

	type colRow struct {
		position int
		colType  string
	}
	var cols []colRow
	for rows.Next() {
		var c colRow
		rows.Scan(&c.position, &c.colType)
		cols = append(cols, c)
	}

	if len(cols) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(cols))
	}
	if cols[0].colType != "fixed_first" {
		t.Errorf("first column: want fixed_first, got %s", cols[0].colType)
	}
	if cols[6].colType != "fixed_last" {
		t.Errorf("last column: want fixed_last, got %s", cols[6].colType)
	}
	for i := 1; i <= 5; i++ {
		if cols[i].colType != "regular" {
			t.Errorf("col[%d]: want regular, got %s", i, cols[i].colType)
		}
	}
}

func TestHandleTemplatesCreate_EmptyName(t *testing.T) {
	re := newTemplatesNewRenderer()
	form := url.Values{}
	form.Set("name", "")
	form.Add("columns[]", "Добре")
	rr := postTemplates(t, re, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "обовʼязкова") {
		t.Errorf("expected validation error in body, got: %s", rr.Body.String())
	}
}

func TestHandleTemplatesCreate_ZeroColumns(t *testing.T) {
	re := newTemplatesNewRenderer()
	form := url.Values{}
	form.Set("name", "Sprint Retro")
	rr := postTemplates(t, re, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "від 1 до 5") {
		t.Errorf("expected column count error in body, got: %s", rr.Body.String())
	}
}

func TestHandleTemplatesCreate_SixColumns(t *testing.T) {
	re := newTemplatesNewRenderer()
	form := url.Values{}
	form.Set("name", "Sprint Retro")
	for _, col := range []string{"А", "Б", "В", "Г", "Д", "Е"} {
		form.Add("columns[]", col)
	}
	rr := postTemplates(t, re, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "від 1 до 5") {
		t.Errorf("expected column count error in body, got: %s", rr.Body.String())
	}
}

func TestHandleTemplatesCreate_EmptyColumnName(t *testing.T) {
	re := newTemplatesNewRenderer()
	form := url.Values{}
	form.Set("name", "Sprint Retro")
	form.Add("columns[]", "Добре")
	form.Add("columns[]", "")
	rr := postTemplates(t, re, form)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "порожньою") {
		t.Errorf("expected empty column name error in body, got: %s", rr.Body.String())
	}
}

func TestHandleTemplatesUpdate_NoRetros(t *testing.T) {
	database := newTestDB(t)
	re := newTemplatesEditRenderer()
	tmplID := createTemplate(t, database, "Старий", []string{"Добре"})

	mux := http.NewServeMux()
	mux.Handle("POST /templates/{id}", handlers.HandleTemplatesUpdate(database, re))

	form := url.Values{}
	form.Set("name", "Новий")
	form.Add("columns[]", "Краще")
	form.Add("columns[]", "Гірше")
	req := httptest.NewRequest("POST", fmt.Sprintf("/templates/%d", tmplID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var name string
	database.QueryRow(`SELECT name FROM templates WHERE id = ?`, tmplID).Scan(&name)
	if name != "Новий" {
		t.Errorf("expected original template name='Новий', got %q", name)
	}

	var totalTemplates int
	database.QueryRow(`SELECT COUNT(*) FROM templates`).Scan(&totalTemplates)
	if totalTemplates != 1 {
		t.Errorf("expected exactly 1 template in DB, got %d", totalTemplates)
	}

	rows, _ := database.Query(`SELECT type FROM template_columns WHERE template_id = ? ORDER BY position`, tmplID)
	defer rows.Close()
	var types []string
	for rows.Next() {
		var ct string
		rows.Scan(&ct)
		types = append(types, ct)
	}
	if len(types) != 4 {
		t.Fatalf("expected 4 columns (fixed+2 regular+fixed), got %d", len(types))
	}
	if types[0] != "fixed_first" || types[3] != "fixed_last" {
		t.Errorf("unexpected column types: %v", types)
	}
}

func TestHandleTemplatesUpdate_WithRetros(t *testing.T) {
	database := newTestDB(t)
	re := newTemplatesEditRenderer()
	tmplID := createTemplate(t, database, "Оригінал", []string{"Добре", "Погано"})
	createRetroForTemplate(t, database, tmplID)

	mux := http.NewServeMux()
	mux.Handle("POST /templates/{id}", handlers.HandleTemplatesUpdate(database, re))

	form := url.Values{}
	form.Set("name", "Змінений")
	form.Add("columns[]", "Нова колонка")
	req := httptest.NewRequest("POST", fmt.Sprintf("/templates/%d", tmplID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var origName string
	database.QueryRow(`SELECT name FROM templates WHERE id = ?`, tmplID).Scan(&origName)
	if origName != "Оригінал" {
		t.Errorf("original template must remain unchanged, got name=%q", origName)
	}

	var copyName string
	if err := database.QueryRow(`SELECT name FROM templates WHERE id != ?`, tmplID).Scan(&copyName); err != nil {
		t.Fatalf("copy template not found: %v", err)
	}
	if copyName != "Змінений (копія)" {
		t.Errorf("expected copy name 'Змінений (копія)', got %q", copyName)
	}

	var copyColCount int
	database.QueryRow(`SELECT COUNT(*) FROM template_columns WHERE template_id != ?`, tmplID).Scan(&copyColCount)
	if copyColCount != 3 {
		t.Errorf("expected 3 columns for copy (fixed+1 regular+fixed), got %d", copyColCount)
	}
}

func TestHandleTemplatesArchive(t *testing.T) {
	database := newTestDB(t)
	tmplID := createTemplate(t, database, "Активний", []string{"Добре"})

	mux := http.NewServeMux()
	mux.Handle("POST /templates/{id}/archive", handlers.HandleTemplatesArchive(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/templates/%d/archive", tmplID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var status string
	database.QueryRow(`SELECT status FROM templates WHERE id = ?`, tmplID).Scan(&status)
	if status != "archived" {
		t.Errorf("expected status 'archived', got %q", status)
	}
}

func TestHandleTemplatesEdit_Archived(t *testing.T) {
	database := newTestDB(t)
	re := newTemplatesEditRenderer()
	tmplID := createTemplate(t, database, "Архів", []string{"Колонка"})
	database.Exec(`UPDATE templates SET status = 'archived' WHERE id = ?`, tmplID)

	mux := http.NewServeMux()
	mux.Handle("GET /templates/{id}/edit", handlers.HandleTemplatesEdit(database, re))

	req := httptest.NewRequest("GET", fmt.Sprintf("/templates/%d/edit", tmplID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/templates") {
		t.Errorf("expected redirect to /templates, got %q", loc)
	}
}

func TestHandleTemplatesDelete_NoRetros(t *testing.T) {
	database := newTestDB(t)
	tmplID := createTemplate(t, database, "Зайвий", []string{"Колонка"})

	mux := http.NewServeMux()
	mux.Handle("POST /templates/{id}/delete", handlers.HandleTemplatesDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/templates/%d/delete", tmplID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var tmplCount int
	database.QueryRow(`SELECT COUNT(*) FROM templates WHERE id = ?`, tmplID).Scan(&tmplCount)
	if tmplCount != 0 {
		t.Errorf("expected template deleted, got count=%d", tmplCount)
	}

	var colCount int
	database.QueryRow(`SELECT COUNT(*) FROM template_columns WHERE template_id = ?`, tmplID).Scan(&colCount)
	if colCount != 0 {
		t.Errorf("expected columns deleted, got count=%d", colCount)
	}
}

func TestHandleTemplatesDelete_WithRetros(t *testing.T) {
	database := newTestDB(t)
	tmplID := createTemplate(t, database, "Використовується", []string{"Колонка"})
	createRetroForTemplate(t, database, tmplID)

	mux := http.NewServeMux()
	mux.Handle("POST /templates/{id}/delete", handlers.HandleTemplatesDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/templates/%d/delete", tmplID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error=") {
		t.Errorf("expected error param in redirect, got %q", loc)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM templates WHERE id = ?`, tmplID).Scan(&count)
	if count != 1 {
		t.Errorf("expected template to remain in DB, got count=%d", count)
	}
}

func TestHandleTemplatesCreate_PositionsReordered(t *testing.T) {
	re := newTemplatesNewRenderer()
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.Handle("POST /templates", handlers.HandleTemplatesCreate(database, re))

	form := url.Values{}
	form.Set("name", "Reorder Test")
	form.Add("columns[]", "Добре")
	form.Add("columns[]", "Погано")
	form.Add("columns[]", "Ідеї")
	// positions[i] — бажана позиція для columns[i]: Добре→3, Погано→1, Ідеї→2
	form.Add("positions[]", "3")
	form.Add("positions[]", "1")
	form.Add("positions[]", "2")

	req := httptest.NewRequest("POST", "/templates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var templateID int64
	database.QueryRow(`SELECT id FROM templates WHERE name = ?`, "Reorder Test").Scan(&templateID)

	type colRow struct {
		title    string
		position int
	}
	rows, err := database.Query(
		`SELECT title, position FROM template_columns WHERE template_id = ? AND type = 'regular' ORDER BY position`,
		templateID,
	)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	var cols []colRow
	for rows.Next() {
		var c colRow
		rows.Scan(&c.title, &c.position)
		cols = append(cols, c)
	}

	if len(cols) != 3 {
		t.Fatalf("expected 3 regular columns, got %d", len(cols))
	}
	want := []colRow{{"Погано", 1}, {"Ідеї", 2}, {"Добре", 3}}
	for i, w := range want {
		if cols[i].title != w.title || cols[i].position != w.position {
			t.Errorf("col[%d]: want {%q, %d}, got {%q, %d}", i, w.title, w.position, cols[i].title, cols[i].position)
		}
	}

	var fixedFirstPos, fixedLastPos int
	database.QueryRow(`SELECT position FROM template_columns WHERE template_id = ? AND type = 'fixed_first'`, templateID).Scan(&fixedFirstPos)
	database.QueryRow(`SELECT position FROM template_columns WHERE template_id = ? AND type = 'fixed_last'`, templateID).Scan(&fixedLastPos)
	if fixedFirstPos != 0 {
		t.Errorf("fixed_first: want position=0, got %d", fixedFirstPos)
	}
	if fixedLastPos != 4 {
		t.Errorf("fixed_last: want position=4, got %d", fixedLastPos)
	}
}

func TestHandleTemplatesCreate_NoPositions(t *testing.T) {
	re := newTemplatesNewRenderer()
	database := newTestDB(t)

	mux := http.NewServeMux()
	mux.Handle("POST /templates", handlers.HandleTemplatesCreate(database, re))

	form := url.Values{}
	form.Set("name", "No Positions Test")
	form.Add("columns[]", "Добре")
	form.Add("columns[]", "Погано")
	// positions[] відсутній — дефолтний порядок

	req := httptest.NewRequest("POST", "/templates", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var templateID int64
	database.QueryRow(`SELECT id FROM templates WHERE name = ?`, "No Positions Test").Scan(&templateID)

	type colRow struct {
		title    string
		position int
	}
	rows, err := database.Query(
		`SELECT title, position FROM template_columns WHERE template_id = ? AND type = 'regular' ORDER BY position`,
		templateID,
	)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	var cols []colRow
	for rows.Next() {
		var c colRow
		rows.Scan(&c.title, &c.position)
		cols = append(cols, c)
	}

	if len(cols) != 2 {
		t.Fatalf("expected 2 regular columns, got %d", len(cols))
	}
	want := []colRow{{"Добре", 1}, {"Погано", 2}}
	for i, w := range want {
		if cols[i].title != w.title || cols[i].position != w.position {
			t.Errorf("col[%d]: want {%q, %d}, got {%q, %d}", i, w.title, w.position, cols[i].title, cols[i].position)
		}
	}
}

func TestHandleTemplatesUpdate_PositionsReordered(t *testing.T) {
	database := newTestDB(t)
	re := newTemplatesEditRenderer()
	tmplID := createTemplate(t, database, "Positions Update", []string{"А", "Б", "В"})

	mux := http.NewServeMux()
	mux.Handle("POST /templates/{id}", handlers.HandleTemplatesUpdate(database, re))

	form := url.Values{}
	form.Set("name", "Positions Update")
	form.Add("columns[]", "А")
	form.Add("columns[]", "Б")
	form.Add("columns[]", "В")
	// positions[i] — бажана позиція: А→2, Б→3, В→1
	form.Add("positions[]", "2")
	form.Add("positions[]", "3")
	form.Add("positions[]", "1")

	req := httptest.NewRequest("POST", fmt.Sprintf("/templates/%d", tmplID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	type colRow struct {
		title    string
		position int
	}
	rows, err := database.Query(
		`SELECT title, position FROM template_columns WHERE template_id = ? AND type = 'regular' ORDER BY position`,
		tmplID,
	)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	var cols []colRow
	for rows.Next() {
		var c colRow
		rows.Scan(&c.title, &c.position)
		cols = append(cols, c)
	}

	if len(cols) != 3 {
		t.Fatalf("expected 3 regular columns, got %d", len(cols))
	}
	want := []colRow{{"В", 1}, {"А", 2}, {"Б", 3}}
	for i, w := range want {
		if cols[i].title != w.title || cols[i].position != w.position {
			t.Errorf("col[%d]: want {%q, %d}, got {%q, %d}", i, w.title, w.position, cols[i].title, cols[i].position)
		}
	}

	var fixedFirstPos, fixedLastPos int
	database.QueryRow(`SELECT position FROM template_columns WHERE template_id = ? AND type = 'fixed_first'`, tmplID).Scan(&fixedFirstPos)
	database.QueryRow(`SELECT position FROM template_columns WHERE template_id = ? AND type = 'fixed_last'`, tmplID).Scan(&fixedLastPos)
	if fixedFirstPos != 0 {
		t.Errorf("fixed_first: want position=0, got %d", fixedFirstPos)
	}
	if fixedLastPos != 4 {
		t.Errorf("fixed_last: want position=4, got %d", fixedLastPos)
	}
}
