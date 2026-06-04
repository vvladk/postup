package scheduler_test

import (
	"database/sql"
	"testing"
	"time"

	idb "github.com/vladyslavkondratiuk/postup/internal/db"
	"github.com/vladyslavkondratiuk/postup/internal/scheduler"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := idb.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := idb.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createRetroWithDate(t *testing.T, database *sql.DB, date time.Time, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := database.Exec(`INSERT INTO teams (name, created_at) VALUES ('Test', ?)`, now)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	teamID, _ := res.LastInsertId()

	res, err = database.Exec(
		`INSERT INTO retros (team_id, date, vote_limit, status, created_at) VALUES (?, ?, 10, ?, ?)`,
		teamID, date.UTC().Format(time.RFC3339), status, now,
	)
	if err != nil {
		t.Fatalf("create retro: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// Тест 5 — архівування прострочених ретро
func TestArchiveOverdue_ExpiredRetro(t *testing.T) {
	database := newTestDB(t)
	retroID := createRetroWithDate(t, database, time.Now().Add(-2*time.Hour), "active")

	if err := scheduler.ArchiveOverdue(database); err != nil {
		t.Fatalf("ArchiveOverdue: %v", err)
	}

	var status string
	database.QueryRow(`SELECT status FROM retros WHERE id = ?`, retroID).Scan(&status)
	if status != "finished" {
		t.Errorf("expected status 'finished', got %q", status)
	}
}

// Тест 6 — активне ретро не архівується передчасно
func TestArchiveOverdue_FutureRetro(t *testing.T) {
	database := newTestDB(t)
	retroID := createRetroWithDate(t, database, time.Now().Add(time.Hour), "active")

	if err := scheduler.ArchiveOverdue(database); err != nil {
		t.Fatalf("ArchiveOverdue: %v", err)
	}

	var status string
	database.QueryRow(`SELECT status FROM retros WHERE id = ?`, retroID).Scan(&status)
	if status != "active" {
		t.Errorf("expected status 'active', got %q", status)
	}
}

// Тест 7 — завершене ретро не змінюється
func TestArchiveOverdue_AlreadyFinished(t *testing.T) {
	database := newTestDB(t)
	retroID := createRetroWithDate(t, database, time.Now().Add(-2*time.Hour), "finished")

	if err := scheduler.ArchiveOverdue(database); err != nil {
		t.Fatalf("ArchiveOverdue: %v", err)
	}

	var status string
	database.QueryRow(`SELECT status FROM retros WHERE id = ?`, retroID).Scan(&status)
	if status != "finished" {
		t.Errorf("expected status 'finished', got %q (should be unchanged)", status)
	}
}
