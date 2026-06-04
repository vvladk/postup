package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vladyslavkondratiuk/postup/internal/session"
)

func HandleLoginGet(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("session_id"); err == nil {
			if _, err := session.Get(db, cookie.Value); err == nil {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
		var data map[string]any
		if r.URL.Query().Get("reset") == "1" {
			data = map[string]any{"Success": "Пароль успішно змінено"}
		}
		re.Render(w, r, "login.html", data)
	}
}

func HandleLoginPost(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := r.FormValue("email")
		password := r.FormValue("password")

		var id int64
		var hash string
		err := db.QueryRow(
			`SELECT id, password_hash FROM users WHERE email = ?`, email,
		).Scan(&id, &hash)

		if err == sql.ErrNoRows || err != nil ||
			bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
			re.Render(w, r, "login.html", map[string]any{"Error": "Невірний email або пароль"})
			return
		}

		sess, err := session.Create(db, id)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sess.Token,
			Expires:  sess.ExpiresAt,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func HandleInviteGet(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")

		var usedAt sql.NullString
		err := db.QueryRow(
			`SELECT invite_used_at FROM users WHERE invite_token = ?`, token,
		).Scan(&usedAt)

		if err == sql.ErrNoRows || usedAt.Valid {
			re.Render(w, r, "invite.html", map[string]any{"Invalid": true})
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		re.Render(w, r, "invite.html", map[string]any{"Token": token})
	}
}

func HandleInvitePost(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		renderErr := func(msg string) {
			re.Render(w, r, "invite.html", map[string]any{"Token": token, "Error": msg})
		}

		var userID int64
		var usedAt sql.NullString
		err := db.QueryRow(
			`SELECT id, invite_used_at FROM users WHERE invite_token = ?`, token,
		).Scan(&userID, &usedAt)

		if err == sql.ErrNoRows || usedAt.Valid {
			re.Render(w, r, "invite.html", map[string]any{"Invalid": true})
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		password := r.FormValue("password")
		confirm := r.FormValue("password_confirm")

		if len(password) < 8 {
			renderErr("Пароль повинен містити щонайменше 8 символів")
			return
		}
		if password != confirm {
			renderErr("Паролі не збігаються")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`UPDATE users SET password_hash = ?, invite_used_at = ?, invite_token = NULL, updated_at = ?
			 WHERE invite_token = ? AND invite_used_at IS NULL`,
			string(hash), now, now, token,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			re.Render(w, r, "invite.html", map[string]any{"Invalid": true})
			return
		}

		sess, err := session.Create(db, userID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sess.Token,
			Expires:  sess.ExpiresAt,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func HandleForgotGet(re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		re.Render(w, r, "forgot.html", nil)
	}
}

func HandleForgotPost(db *sql.DB, re *Renderer, port string, getIP func() string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := r.FormValue("email")

		var userID int64
		err := db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&userID)

		if err == sql.ErrNoRows {
			re.Render(w, r, "forgot.html", map[string]any{"Success": true})
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		token := hex.EncodeToString(b)

		now := time.Now().UTC()
		expiresAt := now.Add(24 * time.Hour)

		if _, err := db.Exec(
			`UPDATE users SET reset_token = ?, reset_token_expires_at = ?, updated_at = ? WHERE id = ?`,
			token, expiresAt.Format(time.RFC3339), now.Format(time.RFC3339), userID,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		link := fmt.Sprintf("http://%s:%s/reset-password/%s", getIP(), port, token)

		re.Render(w, r, "forgot.html", map[string]any{"Success": true, "Link": link})
	}
}

func HandleResetGet(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")

		now := time.Now().UTC().Format(time.RFC3339)
		var id int64
		err := db.QueryRow(
			`SELECT id FROM users WHERE reset_token = ? AND reset_token_expires_at > ?`, token, now,
		).Scan(&id)

		if err == sql.ErrNoRows {
			re.Render(w, r, "reset.html", map[string]any{"Invalid": true})
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		re.Render(w, r, "reset.html", map[string]any{"Token": token})
	}
}

func HandleResetPost(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		renderErr := func(msg string) {
			re.Render(w, r, "reset.html", map[string]any{"Token": token, "Error": msg})
		}

		now := time.Now().UTC()
		var userID int64
		err := db.QueryRow(
			`SELECT id FROM users WHERE reset_token = ? AND reset_token_expires_at > ?`,
			token, now.Format(time.RFC3339),
		).Scan(&userID)

		if err == sql.ErrNoRows {
			re.Render(w, r, "reset.html", map[string]any{"Invalid": true})
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		password := r.FormValue("password")
		confirm := r.FormValue("password_confirm")

		if len(password) < 8 {
			renderErr("Пароль повинен містити щонайменше 8 символів")
			return
		}
		if password != confirm {
			renderErr("Паролі не збігаються")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		res, err := db.Exec(
			`UPDATE users
			 SET password_hash = ?, reset_token = NULL, reset_token_expires_at = NULL, updated_at = ?
			 WHERE reset_token = ? AND reset_token_expires_at > ?`,
			string(hash), now.Format(time.RFC3339), token, now.Format(time.RFC3339),
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			re.Render(w, r, "reset.html", map[string]any{"Invalid": true})
			return
		}

		http.Redirect(w, r, "/login?reset=1", http.StatusSeeOther)
	}
}

func HandleLogout(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie("session_id"); err == nil {
			session.Delete(db, cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    "",
			MaxAge:   -1,
			Path:     "/",
			HttpOnly: true,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
