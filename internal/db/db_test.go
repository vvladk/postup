package db

import (
	"sort"
	"testing"
)

func TestOpen(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Close()

	if err := database.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestMigrateTablesExist(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if err := Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	want := []string{
		"action_items", "action_statuses", "cards",
		"retros", "schema_migrations", "sessions", "settings", "team_members",
		"teams", "template_columns", "templates", "users", "votes",
	}

	rows, err := database.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		got = append(got, name)
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("expected %d tables, got %d: %v", len(want), len(got), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("table[%d]: want %q, got %q", i, name, got[i])
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if err := Migrate(database); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	var count int
	database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count)
	if count == 0 {
		t.Fatal("tables disappeared after second Migrate")
	}
}

func TestMigrateSchemaMigrationsRecorded(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	if err := Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var name string
	err = database.QueryRow(
		`SELECT name FROM schema_migrations WHERE name = '001_init.sql'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("001_init.sql not recorded in schema_migrations: %v", err)
	}
	if name != "001_init.sql" {
		t.Errorf("expected '001_init.sql', got %q", name)
	}
}
