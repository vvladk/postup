package server

import (
	"database/sql"

	"github.com/postup-app/postup/internal/config"
	"github.com/postup-app/postup/internal/ws"
)

type App struct {
	DB  *sql.DB
	Hub *ws.Hub
	Cfg *config.Config
}
