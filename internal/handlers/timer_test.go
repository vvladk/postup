package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/vladyslavkondratiuk/postup/internal/handlers"
	"github.com/vladyslavkondratiuk/postup/internal/middleware"
	"github.com/vladyslavkondratiuk/postup/internal/session"
)

func newTimerRenderer() *handlers.Renderer {
	return handlers.NewRenderer(fstest.MapFS{
		"templates/pages/timer.html": {
			Data: []byte(`<div id="timer-display">10:00</div>`),
		},
	})
}

// TestHandleTimerShow_Admin — GET /timer as admin returns 200 with timer element.
func TestHandleTimerShow_Admin(t *testing.T) {
	database := newTestDB(t)
	userID := createTestAdmin(t, database, "admin@timer.com", "password123")
	re := newTimerRenderer()
	auth := middleware.NewAuth(database)

	sess, err := session.Create(database, userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /timer", auth.RequireAdmin(handlers.HandleTimerShow(re)))

	req := httptest.NewRequest("GET", "/timer", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "timer-display") {
		t.Errorf("expected body to contain timer-display, got: %s", rr.Body.String())
	}
}

// TestHandleTimerShow_Member — GET /timer as non-admin returns 403.
func TestHandleTimerShow_Member(t *testing.T) {
	database := newTestDB(t)
	memberID := createTestMember(t, database, "member@timer.com")
	re := newTimerRenderer()
	auth := middleware.NewAuth(database)

	sess, err := session.Create(database, memberID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /timer", auth.RequireAdmin(handlers.HandleTimerShow(re)))

	req := httptest.NewRequest("GET", "/timer", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: sess.Token})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}
