package server

import (
	"net/http"

	"github.com/postup-app/postup/internal/handlers"
	"github.com/postup-app/postup/internal/middleware"
	"github.com/postup-app/postup/internal/session"
)

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	r := handlers.NewRenderer(a.Cfg.FS, a.DB, a.Cfg.Port, a.Cfg.GetIP)
	auth := middleware.NewAuth(a.DB)

	retrosPMHandler := auth.RequireAdmin(handlers.HandleRetrosPMIndex(a.DB, r))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		if cookie, err := req.Cookie("session_id"); err == nil {
			if sess, err := session.Get(a.DB, cookie.Value); err == nil {
				var role string
				a.DB.QueryRow(`SELECT role FROM users WHERE id = ?`, sess.UserID).Scan(&role)
				if role != "admin" {
					http.Redirect(w, req, "/dashboard", http.StatusSeeOther)
					return
				}
			}
		}
		retrosPMHandler.ServeHTTP(w, req)
	})
	mux.Handle("GET /dashboard", auth.RequireAuth(handlers.HandleRetrosMemberIndex(a.DB, r)))
	mux.Handle("GET /retros/new", auth.RequireAdmin(handlers.HandleRetrosNew(a.DB, r)))
	mux.Handle("POST /retros", auth.RequireAdmin(handlers.HandleRetrosCreate(a.DB, r)))
	mux.Handle("GET /retros/{id}/edit", auth.RequireAdmin(handlers.HandleRetrosEdit(a.DB, r)))
	mux.Handle("POST /retros/{id}", auth.RequireAdmin(handlers.HandleRetrosUpdate(a.DB, r)))
	mux.Handle("POST /retros/{id}/finish", auth.RequireAdmin(handlers.HandleRetrosFinish(a.DB)))
	mux.Handle("POST /retros/{id}/copy-action-items", auth.RequireAdmin(handlers.HandleRetrosCopyActionItems(a.DB)))
	mux.Handle("GET /retros/{id}/export.md", auth.RequireAdmin(handlers.HandleRetrosExportMD(a.DB)))
	mux.Handle("GET /retros/{id}/export.csv", auth.RequireAdmin(handlers.HandleRetrosExportCSV(a.DB)))
	mux.HandleFunc("GET /login", handlers.HandleLoginGet(a.DB, r))
	mux.HandleFunc("POST /login", handlers.HandleLoginPost(a.DB, r))
	mux.HandleFunc("GET /logout", handlers.HandleLogout(a.DB))
	mux.HandleFunc("GET /invite/{token}", handlers.HandleInviteGet(a.DB, r))
	mux.HandleFunc("POST /invite/{token}", handlers.HandleInvitePost(a.DB, r))
	mux.HandleFunc("GET /forgot-password", handlers.HandleForgotGet(r))
	mux.HandleFunc("POST /forgot-password", handlers.HandleForgotPost(a.DB, r, a.Cfg.Port, a.Cfg.GetIP))
	mux.HandleFunc("GET /reset-password/{token}", handlers.HandleResetGet(a.DB, r))
	mux.HandleFunc("POST /reset-password/{token}", handlers.HandleResetPost(a.DB, r))
	mux.Handle("GET /users", auth.RequireAdmin(handlers.HandleUsersIndex(a.DB, r, a.Cfg.Port, a.Cfg.GetIP)))
	mux.Handle("GET /users/new", auth.RequireAdmin(handlers.HandleUsersNew(a.DB, r)))
	mux.Handle("POST /users", auth.RequireAdmin(handlers.HandleUsersCreate(a.DB, r, a.Cfg.Port, a.Cfg.GetIP)))
	mux.Handle("GET /users/{id}/edit", auth.RequireAdmin(handlers.HandleUsersEdit(a.DB, r)))
	mux.Handle("POST /users/{id}", auth.RequireAdmin(handlers.HandleUsersUpdate(a.DB, r)))
	mux.Handle("POST /users/{id}/delete", auth.RequireAdmin(handlers.HandleUsersDelete(a.DB)))
	mux.Handle("POST /users/{id}/reset-password", auth.RequireAdmin(handlers.HandleUsersResetPassword(a.DB, a.Cfg.Port, a.Cfg.GetIP)))
	mux.Handle("GET /teams", auth.RequireAdmin(handlers.HandleTeamsIndex(a.DB, r)))
	mux.Handle("GET /teams/new", auth.RequireAdmin(handlers.HandleTeamsNew(r)))
	mux.Handle("POST /teams", auth.RequireAdmin(handlers.HandleTeamsCreate(a.DB, r)))
	mux.Handle("GET /teams/{id}/edit", auth.RequireAdmin(handlers.HandleTeamsEdit(a.DB, r)))
	mux.Handle("POST /teams/{id}", auth.RequireAdmin(handlers.HandleTeamsUpdate(a.DB, r)))
	mux.Handle("POST /teams/{id}/delete", auth.RequireAdmin(handlers.HandleTeamsDelete(a.DB)))
	mux.Handle("GET /templates", auth.RequireAdmin(handlers.HandleTemplatesIndex(a.DB, r)))
	mux.Handle("GET /templates/new", auth.RequireAdmin(handlers.HandleTemplatesNew(r)))
	mux.Handle("POST /templates", auth.RequireAdmin(handlers.HandleTemplatesCreate(a.DB, r)))
	mux.Handle("GET /templates/{id}/edit", auth.RequireAdmin(handlers.HandleTemplatesEdit(a.DB, r)))
	mux.Handle("POST /templates/{id}", auth.RequireAdmin(handlers.HandleTemplatesUpdate(a.DB, r)))
	mux.Handle("POST /templates/{id}/archive", auth.RequireAdmin(handlers.HandleTemplatesArchive(a.DB)))
	mux.Handle("POST /templates/{id}/delete", auth.RequireAdmin(handlers.HandleTemplatesDelete(a.DB)))
	mux.Handle("GET /retros/{id}/board", auth.RequireAuth(handlers.HandleBoardShow(a.DB, r)))
	mux.Handle("GET /retros/{id}/ws", auth.RequireAuth(handlers.HandleBoardWS(a.DB, a.Hub)))
	mux.Handle("POST /retros/{id}/cards", auth.RequireAuth(handlers.HandleCardsCreate(a.DB, a.Hub)))
	mux.Handle("POST /cards/{id}", auth.RequireAuth(handlers.HandleCardsUpdate(a.DB, a.Hub)))
	mux.Handle("POST /cards/{id}/delete", auth.RequireAuth(handlers.HandleCardsDelete(a.DB, a.Hub)))
	mux.Handle("POST /cards/{id}/move", auth.RequireAuth(handlers.HandleCardsMoveColumn(a.DB, a.Hub)))
	mux.Handle("POST /cards/{id}/copy", auth.RequireAdmin(handlers.HandleCardsCopy(a.DB)))
	mux.Handle("POST /cards/{id}/to-action-item", auth.RequireAuth(handlers.HandleCardsToActionItem(a.DB, a.Hub)))
	mux.Handle("POST /cards/{id}/vote", auth.RequireAuth(handlers.HandleCardsVote(a.DB, a.Hub)))
	mux.Handle("POST /retros/{id}/action-items", auth.RequireAuth(handlers.HandleActionItemsCreate(a.DB, a.Hub)))
	mux.Handle("POST /action-items/{id}/status", auth.RequireAuth(handlers.HandleActionItemsUpdateStatus(a.DB, a.Hub)))
	mux.Handle("GET /settings", auth.RequireAuth(handlers.HandleSettingsShow(a.DB, r)))
	mux.Handle("POST /settings", auth.RequireAuth(handlers.HandleSettingsUpdate(a.DB)))
	mux.Handle("GET /timer", auth.RequireAdmin(handlers.HandleTimerShow(r)))
	mux.Handle("GET /statuses", auth.RequireAdmin(handlers.HandleStatusesIndex(a.DB, r)))
	mux.Handle("POST /statuses", auth.RequireAdmin(handlers.HandleStatusesCreate(a.DB)))
	mux.Handle("POST /statuses/{id}", auth.RequireAdmin(handlers.HandleStatusesUpdate(a.DB)))
	mux.Handle("POST /statuses/{id}/delete", auth.RequireAdmin(handlers.HandleStatusesDelete(a.DB)))
	mux.Handle("GET /static/", http.FileServer(http.FS(a.Cfg.FS)))
}
