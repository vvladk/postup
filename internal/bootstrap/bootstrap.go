package bootstrap

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	dbpkg "github.com/postup-app/postup/internal/db"
)

const minPasswordLen = 4

var (
	bcryptCost   = 12
	dialogReader io.Reader            = os.Stdin
	dialogPwd    func() ([]byte, error) = defaultReadPwd
	osExit       func(int)              = os.Exit
)

func defaultReadPwd() ([]byte, error) {
	return term.ReadPassword(int(os.Stdin.Fd()))
}

// Bootstrap runs migrations, checks for an admin account, and opens an
// interactive initialization dialog if none exists.
func Bootstrap(db *sql.DB) error {
	if err := dbpkg.Migrate(db); err != nil {
		return err
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		return fmt.Errorf("check admin: %w", err)
	}
	if count > 0 {
		return nil
	}
	return runInitDialog(db, dialogReader, dialogPwd)
}

// ResetAdminPassword opens an interactive terminal dialog to set a new admin password.
func ResetAdminPassword(db *sql.DB) error {
	return runResetDialog(db, dialogPwd)
}

func runInitDialog(db *sql.DB, in io.Reader, readPwd func() ([]byte, error)) error {
	scanner := bufio.NewScanner(in)

	fmt.Print("Postup is not initialized.\nInitialize now? [y/n]: ")
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "y" {
		fmt.Println("To initialize later — just run postup again.")
		osExit(0)
		return nil
	}

	email := promptEmail(scanner)
	password := promptPasswordWithConfirm(readPwd)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO users (email, password_hash, role, created_at, updated_at) VALUES (?, ?, 'admin', ?, ?)`,
		email, string(hash), now, now,
	); err != nil {
		return fmt.Errorf("create admin: %w", err)
	}

	fmt.Println("Done! Starting server...")
	return nil
}

func runResetDialog(db *sql.DB, readPwd func() ([]byte, error)) error {
	var email string
	if err := db.QueryRow(`SELECT email FROM users WHERE role = 'admin'`).Scan(&email); err != nil {
		return fmt.Errorf("find admin: %w", err)
	}

	fmt.Printf("Resetting admin password (%s)\n", email)
	password := promptPasswordWithConfirm(readPwd)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE role = 'admin'`,
		string(hash), now,
	); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	fmt.Println("Password updated.")
	return nil
}

func promptEmail(scanner *bufio.Scanner) string {
	for {
		fmt.Print("Admin email: ")
		if !scanner.Scan() {
			fmt.Fprintln(os.Stderr, "unexpected end of input")
			osExit(1)
			return ""
		}
		email := strings.TrimSpace(scanner.Text())
		if email != "" && strings.Contains(email, "@") {
			return email
		}
		fmt.Println("Invalid email. Please try again.")
	}
}

func promptPasswordWithConfirm(readPwd func() ([]byte, error)) string {
	for {
		fmt.Printf("Password (min. %d characters): ", minPasswordLen)
		pwd1, err := readPwd()
		fmt.Println()
		if err != nil || len(pwd1) < minPasswordLen {
			fmt.Printf("Password must be at least %d characters. Try again.\n", minPasswordLen)
			continue
		}

		fmt.Print("Confirm password: ")
		pwd2, err := readPwd()
		fmt.Println()
		if err != nil {
			continue
		}

		if string(pwd1) != string(pwd2) {
			fmt.Println("Passwords do not match. Try again.")
			continue
		}
		return string(pwd1)
	}
}
