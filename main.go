package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/postup-app/postup/cmd"
	"github.com/postup-app/postup/internal/db"
	"github.com/postup-app/postup/internal/handlers"
	"github.com/postup-app/postup/internal/ipdetect"
	"github.com/postup-app/postup/internal/middleware"
	"github.com/postup-app/postup/internal/scheduler"
	"github.com/postup-app/postup/internal/session"
	"github.com/postup-app/postup/internal/ws"
)

//go:embed templates static
var embedded embed.FS

func main() {
	if cmd.Run() {
		os.Exit(0)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(filepath.Dir(exe), "retro.db")

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	scheduler.StartArchiveScheduler(database)

	hub := ws.NewHub()
	go hub.Run()

	r := handlers.NewRenderer(embedded)
	auth := middleware.NewAuth(database)

	mux := http.NewServeMux()
	retrosPMHandler := auth.RequireAdmin(handlers.HandleRetrosPMIndex(database, r))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		var count int
		database.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
		if count == 0 {
			http.Redirect(w, req, "/setup", http.StatusSeeOther)
			return
		}
		if cookie, err := req.Cookie("session_id"); err == nil {
			if sess, err := session.Get(database, cookie.Value); err == nil {
				var role string
				database.QueryRow(`SELECT role FROM users WHERE id = ?`, sess.UserID).Scan(&role)
				if role != "admin" {
					http.Redirect(w, req, "/dashboard", http.StatusSeeOther)
					return
				}
			}
		}
		retrosPMHandler.ServeHTTP(w, req)
	})
	mux.Handle("GET /dashboard", auth.RequireAuth(handlers.HandleRetrosMemberIndex(database, r)))
	mux.Handle("GET /retros/new", auth.RequireAdmin(handlers.HandleRetrosNew(database, r)))
	mux.Handle("POST /retros", auth.RequireAdmin(handlers.HandleRetrosCreate(database, r)))
	mux.Handle("GET /retros/{id}/invite-link", auth.RequireAdmin(handlers.HandleRetrosInvite(database, r, port, func() string {
		return ipdetect.DetectWithFallback("")
	})))
	mux.Handle("GET /retros/{id}/edit", auth.RequireAdmin(handlers.HandleRetrosEdit(database, r)))
	mux.Handle("POST /retros/{id}", auth.RequireAdmin(handlers.HandleRetrosUpdate(database, r)))
	mux.Handle("POST /retros/{id}/finish", auth.RequireAdmin(handlers.HandleRetrosFinish(database)))
	mux.HandleFunc("GET /join/{token}", handlers.HandleRetrosJoin(database))
	mux.HandleFunc("GET /setup", handlers.HandleSetup(r))
	mux.HandleFunc("GET /login", handlers.HandleLoginGet(database, r))
	mux.HandleFunc("POST /login", handlers.HandleLoginPost(database, r))
	mux.HandleFunc("GET /logout", handlers.HandleLogout(database))
	mux.HandleFunc("GET /invite/{token}", handlers.HandleInviteGet(database, r))
	mux.HandleFunc("POST /invite/{token}", handlers.HandleInvitePost(database, r))
	mux.HandleFunc("GET /forgot-password", handlers.HandleForgotGet(r))
	mux.HandleFunc("POST /forgot-password", handlers.HandleForgotPost(database, r, port, func() string {
		return ipdetect.DetectWithFallback("")
	}))
	mux.HandleFunc("GET /reset-password/{token}", handlers.HandleResetGet(database, r))
	mux.HandleFunc("POST /reset-password/{token}", handlers.HandleResetPost(database, r))
	mux.Handle("GET /users", auth.RequireAdmin(handlers.HandleUsersIndex(database, r)))
	mux.Handle("GET /users/new", auth.RequireAdmin(handlers.HandleUsersNew(database, r)))
	mux.Handle("POST /users", auth.RequireAdmin(handlers.HandleUsersCreate(database, r, port, func() string {
		return ipdetect.DetectWithFallback("")
	})))
	mux.Handle("GET /users/{id}/edit", auth.RequireAdmin(handlers.HandleUsersEdit(database, r)))
	mux.Handle("POST /users/{id}", auth.RequireAdmin(handlers.HandleUsersUpdate(database, r)))
	mux.Handle("POST /users/{id}/delete", auth.RequireAdmin(handlers.HandleUsersDelete(database)))
	mux.Handle("POST /users/{id}/reset-password", auth.RequireAdmin(handlers.HandleUsersResetPassword(database, port, func() string {
		return ipdetect.DetectWithFallback("")
	})))
	mux.Handle("GET /teams", auth.RequireAdmin(handlers.HandleTeamsIndex(database, r)))
	mux.Handle("GET /teams/new", auth.RequireAdmin(handlers.HandleTeamsNew(r)))
	mux.Handle("POST /teams", auth.RequireAdmin(handlers.HandleTeamsCreate(database, r)))
	mux.Handle("GET /teams/{id}/edit", auth.RequireAdmin(handlers.HandleTeamsEdit(database, r)))
	mux.Handle("POST /teams/{id}", auth.RequireAdmin(handlers.HandleTeamsUpdate(database, r)))
	mux.Handle("POST /teams/{id}/delete", auth.RequireAdmin(handlers.HandleTeamsDelete(database)))
	mux.Handle("GET /templates", auth.RequireAdmin(handlers.HandleTemplatesIndex(database, r)))
	mux.Handle("GET /templates/new", auth.RequireAdmin(handlers.HandleTemplatesNew(r)))
	mux.Handle("POST /templates", auth.RequireAdmin(handlers.HandleTemplatesCreate(database, r)))
	mux.Handle("GET /templates/{id}/edit", auth.RequireAdmin(handlers.HandleTemplatesEdit(database, r)))
	mux.Handle("POST /templates/{id}", auth.RequireAdmin(handlers.HandleTemplatesUpdate(database, r)))
	mux.Handle("POST /templates/{id}/archive", auth.RequireAdmin(handlers.HandleTemplatesArchive(database)))
	mux.Handle("POST /templates/{id}/delete", auth.RequireAdmin(handlers.HandleTemplatesDelete(database)))
	mux.Handle("GET /retros/{id}/board", auth.RequireAuth(handlers.HandleBoardShow(database, r)))
	mux.Handle("GET /retros/{id}/ws", auth.RequireAuth(handlers.HandleBoardWS(database, hub)))
	mux.Handle("POST /retros/{id}/cards", auth.RequireAuth(handlers.HandleCardsCreate(database, hub)))
	mux.Handle("POST /cards/{id}", auth.RequireAuth(handlers.HandleCardsUpdate(database, hub)))
	mux.Handle("POST /cards/{id}/delete", auth.RequireAuth(handlers.HandleCardsDelete(database, hub)))
	mux.Handle("POST /cards/{id}/move", auth.RequireAuth(handlers.HandleCardsMoveColumn(database, hub)))
	mux.Handle("POST /cards/{id}/copy", auth.RequireAuth(handlers.HandleCardsCopy(database)))
	mux.Handle("POST /cards/{id}/vote", auth.RequireAuth(handlers.HandleCardsVote(database, hub)))
	mux.Handle("POST /retros/{id}/action-items", auth.RequireAuth(handlers.HandleActionItemsCreate(database, hub)))
	mux.Handle("POST /action-items/{id}/status", auth.RequireAuth(handlers.HandleActionItemsUpdateStatus(database, hub)))
	mux.Handle("GET /settings", auth.RequireAuth(handlers.HandleSettingsShow(database, r)))
	mux.Handle("POST /settings", auth.RequireAuth(handlers.HandleSettingsUpdate(database)))
	mux.Handle("GET /timer", auth.RequireAdmin(handlers.HandleTimerShow(r)))
	mux.Handle("GET /statuses", auth.RequireAdmin(handlers.HandleStatusesIndex(database, r)))
	mux.Handle("POST /statuses", auth.RequireAdmin(handlers.HandleStatusesCreate(database)))
	mux.Handle("POST /statuses/{id}", auth.RequireAdmin(handlers.HandleStatusesUpdate(database)))
	mux.Handle("POST /statuses/{id}/delete", auth.RequireAdmin(handlers.HandleStatusesDelete(database)))
	mux.Handle("GET /static/", http.FileServer(http.FS(embedded)))

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
