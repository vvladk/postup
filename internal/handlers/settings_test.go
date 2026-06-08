package handlers_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/fstest"

	"github.com/postup-app/postup/internal/handlers"
	"github.com/postup-app/postup/internal/middleware"
)

func newSettingsRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/layout.html": {
			Data: []byte(`{{define "layout"}}{{block "content" .}}{{end}}{{end}}`),
		},
		"templates/pages/settings.html": {
			Data: []byte(`{{define "content"}}settings{{end}}`),
		},
	}, nil, "", nil)
}

func makeSettingsMux(database *sql.DB, re *handlers.Renderer) *http.ServeMux {
	auth := middleware.NewAuth(database)
	mux := http.NewServeMux()
	mux.Handle("GET /settings", auth.RequireAuth(handlers.HandleSettingsShow(database, re)))
	mux.Handle("POST /settings", auth.RequireAuth(handlers.HandleSettingsUpdate(database)))
	return mux
}

// Тест 1 — GET /settings авторизований → 200
func TestHandleSettingsShow_Authorized(t *testing.T) {
	database := newTestDB(t)
	userID := createTestAdmin(t, database, "admin@example.com", "password123")
	mux := makeSettingsMux(database, newSettingsRenderer())

	req := httptest.NewRequest("GET", "/settings", nil)
	req.AddCookie(sessionCookie(t, database, userID))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// Тест 2 — GET /settings без авторизації → редирект на /login
func TestHandleSettingsShow_Unauthorized(t *testing.T) {
	database := newTestDB(t)
	mux := makeSettingsMux(database, newSettingsRenderer())

	req := httptest.NewRequest("GET", "/settings", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// Тест 3 — POST /settings system адміном → 302, base_url збережено в БД
func TestHandleSettingsUpdate_SystemByAdmin(t *testing.T) {
	database := newTestDB(t)
	userID := createTestAdmin(t, database, "admin@example.com", "password123")
	mux := makeSettingsMux(database, newSettingsRenderer())

	rr := postForm(mux, "/settings",
		url.Values{"section": {"system"}, "base_url": {"https://abc.ngrok-free.app"}, "port": {"9090"}},
		sessionCookie(t, database, userID))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var val string
	database.QueryRow(`SELECT value FROM settings WHERE key = 'base_url'`).Scan(&val)
	if val != "https://abc.ngrok-free.app" {
		t.Errorf("expected base_url='https://abc.ngrok-free.app' in DB, got %q", val)
	}
}

// Тест 4 — POST /settings system не адміном → 403, БД не змінилась
func TestHandleSettingsUpdate_SystemByMember(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	mux := makeSettingsMux(database, newSettingsRenderer())

	rr := postForm(mux, "/settings",
		url.Values{"section": {"system"}, "base_url": {"https://evil.example.com"}},
		sessionCookie(t, database, userID))

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}

	var val string
	database.QueryRow(`SELECT value FROM settings WHERE key = 'base_url'`).Scan(&val)
	if val == "https://evil.example.com" {
		t.Error("expected base_url to remain unchanged in DB after forbidden request")
	}
}

// Тест 5 — POST /settings personal lang=en → 302, Set-Cookie lang=en
func TestHandleSettingsUpdate_PersonalLang(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	mux := makeSettingsMux(database, newSettingsRenderer())

	rr := postForm(mux, "/settings",
		url.Values{"section": {"personal"}, "lang": {"en"}, "theme": {"dark"}},
		sessionCookie(t, database, userID))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var found bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "lang" && c.Value == "en" {
			found = true
		}
	}
	if !found {
		t.Error("expected Set-Cookie with lang=en")
	}
}

// Тест 6 — POST /settings personal theme=light → 302, Set-Cookie theme=light
func TestHandleSettingsUpdate_PersonalTheme(t *testing.T) {
	database := newTestDB(t)
	userID := createTestMember(t, database, "member@example.com")
	mux := makeSettingsMux(database, newSettingsRenderer())

	rr := postForm(mux, "/settings",
		url.Values{"section": {"personal"}, "lang": {"uk"}, "theme": {"light"}},
		sessionCookie(t, database, userID))

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 302, got %d", rr.Code)
	}

	var found bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "theme" && c.Value == "light" {
			found = true
		}
	}
	if !found {
		t.Error("expected Set-Cookie with theme=light")
	}
}
