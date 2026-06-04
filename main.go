package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/postup-app/postup/internal/bootstrap"
	"github.com/postup-app/postup/internal/config"
	"github.com/postup-app/postup/internal/db"
	"github.com/postup-app/postup/internal/ipdetect"
	"github.com/postup-app/postup/internal/scheduler"
	"github.com/postup-app/postup/internal/server"
	"github.com/postup-app/postup/internal/ws"
)

//go:embed templates static
var embedded embed.FS

func main() {
	doReset := flag.Bool("reset", false, "Reset admin password")
	flag.Parse()

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

	if *doReset {
		if err := bootstrap.ResetAdminPassword(database); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	if err := bootstrap.Bootstrap(database); err != nil {
		log.Fatal(err)
	}

	scheduler.StartArchiveScheduler(database)

	hub := ws.NewHub()
	go hub.Run()

	cfg := &config.Config{
		Port:  port,
		GetIP: func() string { return ipdetect.DetectWithFallback("") },
		FS:    embedded,
	}

	app := &server.App{DB: database, Hub: hub, Cfg: cfg}

	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
