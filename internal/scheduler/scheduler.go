package scheduler

import (
	"database/sql"
	"log"
	"time"
)

func ArchiveOverdue(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE retros SET status = 'finished'
		WHERE status = 'active'
		AND datetime(date, '+1 hour') < datetime('now')`)
	return err
}

func StartArchiveScheduler(db *sql.DB) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := ArchiveOverdue(db); err != nil {
				log.Printf("archive scheduler: %v", err)
			}
		}
	}()
}
