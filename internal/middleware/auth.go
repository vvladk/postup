package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/postup-app/postup/internal/models"
	"github.com/postup-app/postup/internal/session"
)

type contextKey string

const userKey contextKey = "user"

type Auth struct {
	db *sql.DB
}

func NewAuth(db *sql.DB) *Auth {
	return &Auth{db: db}
}

func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		sess, err := session.Get(a.db, cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := userByID(a.db, sess.UserID)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if !user.IsAdmin() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func GetUser(r *http.Request) *models.User {
	u, _ := r.Context().Value(userKey).(*models.User)
	return u
}

func userByID(db *sql.DB, id int64) (*models.User, error) {
	u := &models.User{}
	var createdAt, updatedAt string
	var inviteToken, inviteUsedAt, resetToken, resetTokenExpiresAt sql.NullString

	err := db.QueryRow(`
		SELECT id, email, password_hash, first_name, last_name, role,
		       invite_token, invite_used_at, reset_token, reset_token_expires_at,
		       created_at, updated_at
		FROM users WHERE id = ?`, id,
	).Scan(
		&u.ID, &u.Email, &u.PasswordHash,
		&u.FirstName, &u.LastName, &u.Role,
		&inviteToken, &inviteUsedAt,
		&resetToken, &resetTokenExpiresAt,
		&createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, session.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if inviteToken.Valid {
		u.InviteToken = &inviteToken.String
	}
	if inviteUsedAt.Valid {
		t, _ := time.Parse(time.RFC3339, inviteUsedAt.String)
		u.InviteUsedAt = &t
	}
	if resetToken.Valid {
		u.ResetToken = &resetToken.String
	}
	if resetTokenExpiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, resetTokenExpiresAt.String)
		u.ResetTokenExpiresAt = &t
	}

	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	u.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return u, nil
}
