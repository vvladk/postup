package handlers_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/postup-app/postup/internal/handlers"
)

func newTeamsNewRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/teams_new.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
	}, nil, "", nil)
}

func newTeamsEditRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/teams_edit.html": {
			Data: []byte(`{{define "content"}}{{if .Error}}{{.Error}}{{end}}<form></form>{{end}}`),
		},
	}, nil, "", nil)
}

func newTeamsIndexRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/teams.html": {
			Data: []byte(`{{define "content"}}{{range .Teams}}{{.Name}} {{.MembersCount}} {{end}}{{end}}`),
		},
	}, nil, "", nil)
}

func TestHandleTeamsCreateValid(t *testing.T) {
	database := newTestDB(t)
	re := newTeamsNewRenderer()
	mux := http.NewServeMux()
	mux.Handle("POST /teams", handlers.HandleTeamsCreate(database, re))

	req := httptest.NewRequest("POST", "/teams", strings.NewReader("name=Backend"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM teams WHERE name = ?`, "Backend").Scan(&count)
	if count != 1 {
		t.Errorf("expected team in DB, got count=%d", count)
	}
}

func TestHandleTeamsCreateDuplicateName(t *testing.T) {
	database := newTestDB(t)
	createTestTeam(t, database, "Backend")
	re := newTeamsNewRenderer()
	mux := http.NewServeMux()
	mux.Handle("POST /teams", handlers.HandleTeamsCreate(database, re))

	req := httptest.NewRequest("POST", "/teams", strings.NewReader("name=Backend"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "вже існує") {
		t.Errorf("expected duplicate error in body, got: %s", rr.Body.String())
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM teams WHERE name = ?`, "Backend").Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 team with that name, got %d", count)
	}
}

func TestHandleTeamsCreateEmptyName(t *testing.T) {
	database := newTestDB(t)
	re := newTeamsNewRenderer()
	mux := http.NewServeMux()
	mux.Handle("POST /teams", handlers.HandleTeamsCreate(database, re))

	req := httptest.NewRequest("POST", "/teams", strings.NewReader("name="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "обовʼязкова") {
		t.Errorf("expected required-name error in body, got: %s", rr.Body.String())
	}
}

func TestHandleTeamsUpdateValid(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "OldName")
	user1ID := createTestMember(t, database, "user1@example.com")
	user2ID := createTestMember(t, database, "user2@example.com")
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, user1ID)

	re := newTeamsEditRenderer()
	mux := http.NewServeMux()
	mux.Handle("POST /teams/{id}", handlers.HandleTeamsUpdate(database, re))

	body := fmt.Sprintf("name=NewName&user_ids=%d", user2ID)
	req := httptest.NewRequest("POST", fmt.Sprintf("/teams/%d", teamID), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var name string
	database.QueryRow(`SELECT name FROM teams WHERE id = ?`, teamID).Scan(&name)
	if name != "NewName" {
		t.Errorf("expected name 'NewName', got %q", name)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, user1ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected user1 removed from team, got count=%d", count)
	}
	database.QueryRow(`SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, user2ID).Scan(&count)
	if count != 1 {
		t.Errorf("expected user2 added to team, got count=%d", count)
	}
}

func TestHandleTeamsDeleteEmpty(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Empty Team")

	mux := http.NewServeMux()
	mux.Handle("POST /teams/{id}/delete", handlers.HandleTeamsDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/teams/%d/delete", teamID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM teams WHERE id = ?`, teamID).Scan(&count)
	if count != 0 {
		t.Errorf("expected team deleted, got count=%d", count)
	}
}

func TestHandleTeamsDeleteWithMembers(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Active Team")
	userID := createTestMember(t, database, "member@example.com")
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, userID)

	mux := http.NewServeMux()
	mux.Handle("POST /teams/{id}/delete", handlers.HandleTeamsDelete(database))

	req := httptest.NewRequest("POST", fmt.Sprintf("/teams/%d/delete", teamID), nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Location"), "error=") {
		t.Errorf("expected error param in redirect, got: %s", rr.Header().Get("Location"))
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM teams WHERE id = ?`, teamID).Scan(&count)
	if count != 1 {
		t.Errorf("expected team to remain in DB, got count=%d", count)
	}
}

func TestHandleTeamsIndexMemberCount(t *testing.T) {
	database := newTestDB(t)
	teamID := createTestTeam(t, database, "Frontend")
	user1ID := createTestMember(t, database, "alice@example.com")
	user2ID := createTestMember(t, database, "bob@example.com")
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, user1ID)
	database.Exec(`INSERT INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, user2ID)

	re := newTeamsIndexRenderer()
	req := httptest.NewRequest("GET", "/teams", nil)
	rr := httptest.NewRecorder()
	handlers.HandleTeamsIndex(database, re)(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "2") {
		t.Errorf("expected member count '2' in body, got: %s", rr.Body.String())
	}
}
