package cmd

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vladyslavkondratiuk/postup/internal/db"
)

var (
	errAdminExists   = errors.New("адмін вже існує")
	errAdminNotFound = errors.New("адмін не знайдений")
)

// bcryptCost is a package-level var so tests can override it with bcrypt.MinCost.
var bcryptCost = 12

// Run parses CLI flags and executes --init or --reset if present.
// Returns true if a CLI command was handled (caller should exit with 0).
func Run() bool {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	doInit := fs.Bool("init", false, "Initialize admin account")
	doReset := fs.Bool("reset", false, "Reset admin password")
	email := fs.String("email", "", "Admin email address")
	password := fs.String("password", "", "Admin password (min 8 chars)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if !*doInit && !*doReset {
		return false
	}

	dbPath, err := resolveDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "помилка визначення шляху до БД: %v\n", err)
		os.Exit(1)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "помилка відкриття БД: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		fmt.Fprintf(os.Stderr, "помилка міграцій: %v\n", err)
		os.Exit(1)
	}

	if *doInit {
		err := initAdmin(database, *email, *password)
		if errors.Is(err, errAdminExists) {
			fmt.Println("Адмін вже існує. Використай --reset для зміни пароля.")
			os.Exit(0)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Ініціалізація успішна. Запусти postup і відкрий браузер.")
	} else {
		if err := resetAdmin(database, *email, *password); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Пароль адміна оновлено.")
	}

	return true
}

func initAdmin(database *sql.DB, email, password string) error {
	if err := validateInput(email, password); err != nil {
		return err
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		return fmt.Errorf("перевірка адміна: %w", err)
	}
	if count > 0 {
		return errAdminExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("хешування пароля: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(
		`INSERT INTO users (email, password_hash, role, created_at, updated_at) VALUES (?, ?, 'admin', ?, ?)`,
		email, string(hash), now, now,
	); err != nil {
		return fmt.Errorf("створення адміна: %w", err)
	}

	return nil
}

func resetAdmin(database *sql.DB, email, password string) error {
	if err := validateInput(email, password); err != nil {
		return err
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		return fmt.Errorf("перевірка адміна: %w", err)
	}
	if count == 0 {
		return errAdminNotFound
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("хешування пароля: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE role = 'admin'`,
		string(hash), now,
	); err != nil {
		return fmt.Errorf("оновлення пароля: %w", err)
	}

	return nil
}

func validateInput(email, password string) error {
	if email == "" || !strings.Contains(email, "@") {
		return fmt.Errorf("невірний email: поле не може бути порожнім і має містити @")
	}
	if len(password) < 8 {
		return fmt.Errorf("пароль занадто короткий: мінімум 8 символів")
	}
	return nil
}

func resolveDBPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "retro.db"), nil
}
