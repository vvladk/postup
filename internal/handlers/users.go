package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/postup-app/postup/internal/middleware"
)

type userListItem struct {
	ID        int64
	Email     string
	FirstName string
	LastName  string
	Role      string
	Active    bool
	TeamNames string
	CreatedAt time.Time
}

type teamItem struct {
	ID   int64
	Name string
}

func HandleUsersIndex(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("search")
		teamIDStr := r.URL.Query().Get("team_id")
		teamID, _ := strconv.ParseInt(teamIDStr, 10, 64)

		query := `
			SELECT u.id, u.email, u.first_name, u.last_name, u.role,
			       u.invite_used_at, u.password_hash, u.created_at,
			       (SELECT GROUP_CONCAT(t.name, ', ')
			        FROM team_members tm
			        JOIN teams t ON tm.team_id = t.id
			        WHERE tm.user_id = u.id) AS team_names
			FROM users u
			WHERE 1=1`

		var args []any
		if search != "" {
			like := "%" + search + "%"
			query += ` AND (u.email LIKE ? OR u.first_name LIKE ? OR u.last_name LIKE ?)`
			args = append(args, like, like, like)
		}
		if teamID > 0 {
			query += ` AND u.id IN (SELECT user_id FROM team_members WHERE team_id = ?)`
			args = append(args, teamID)
		}
		query += ` ORDER BY u.created_at DESC`

		rows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var users []userListItem
		for rows.Next() {
			var item userListItem
			var inviteUsedAt, passwordHash, createdAt, teamNames sql.NullString
			if err := rows.Scan(
				&item.ID, &item.Email, &item.FirstName, &item.LastName, &item.Role,
				&inviteUsedAt, &passwordHash, &createdAt, &teamNames,
			); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			item.Active = passwordHash.String != ""
			item.TeamNames = teamNames.String
			item.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
			users = append(users, item)
		}

		teamRows, err := db.Query(`SELECT id, name FROM teams ORDER BY name`)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer teamRows.Close()

		var teams []teamItem
		for teamRows.Next() {
			var t teamItem
			teamRows.Scan(&t.ID, &t.Name)
			teams = append(teams, t)
		}

		re.Render(w, r, "users.html", map[string]any{
			"Users":          users,
			"Teams":          teams,
			"Search":         search,
			"SelectedTeamID": teamID,
			"Flash":          r.URL.Query().Get("flash"),
			"FlashError":     r.URL.Query().Get("error"),
			"ResetLink":      r.URL.Query().Get("reset_link"),
		})
	}
}

func HandleUsersNew(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teams, err := allTeams(db)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		re.Render(w, r, "users_new.html", map[string]any{"Teams": teams})
	}
}

func HandleUsersCreate(db *sql.DB, re *Renderer, port string, getIP func() string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		firstName := strings.TrimSpace(r.FormValue("first_name"))
		lastName := strings.TrimSpace(r.FormValue("last_name"))
		email := strings.TrimSpace(r.FormValue("email"))
		teamIDStrs := r.Form["team_ids"]

		renderErr := func(msg string) {
			teams, _ := allTeams(db)
			re.Render(w, r, "users_new.html", map[string]any{
				"Error":     msg,
				"Teams":     teams,
				"FirstName": firstName,
				"LastName":  lastName,
				"Email":     email,
			})
		}

		if firstName == "" || lastName == "" || email == "" {
			renderErr("Усі поля обовʼязкові")
			return
		}
		if !strings.Contains(email, "@") {
			renderErr("Невірний email: має містити @")
			return
		}

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&count); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			renderErr("Користувач з таким email вже існує")
			return
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		token := hex.EncodeToString(b)

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO users (email, password_hash, first_name, last_name, role, invite_token, created_at, updated_at)
			 VALUES (?, '', ?, ?, 'member', ?, ?, ?)`,
			email, firstName, lastName, token, now, now,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		userID, _ := res.LastInsertId()

		for _, tidStr := range teamIDStrs {
			tid, err := strconv.ParseInt(tidStr, 10, 64)
			if err != nil || tid <= 0 {
				continue
			}
			db.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, tid, userID)
		}

		link := fmt.Sprintf("http://%s:%s/invite/%s", getIP(), port, token)

		teams, _ := allTeams(db)
		re.Render(w, r, "users_new.html", map[string]any{
			"Teams":      teams,
			"InviteLink": link,
		})
	}
}

type userEditData struct {
	ID        int64
	Email     string
	FirstName string
	LastName  string
}

func HandleUsersEdit(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var u userEditData
		err = db.QueryRow(
			`SELECT id, email, first_name, last_name FROM users WHERE id = ?`, userID,
		).Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		rows, err := db.Query(`SELECT team_id FROM team_members WHERE user_id = ?`, userID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		userTeamIDs := map[int64]bool{}
		for rows.Next() {
			var tid int64
			rows.Scan(&tid)
			userTeamIDs[tid] = true
		}

		teams, err := allTeams(db)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		re.Render(w, r, "users_edit.html", map[string]any{
			"User":        u,
			"Teams":       teams,
			"UserTeamIDs": userTeamIDs,
		})
	}
}

func HandleUsersUpdate(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		firstName := strings.TrimSpace(r.FormValue("first_name"))
		lastName := strings.TrimSpace(r.FormValue("last_name"))
		teamIDStrs := r.Form["team_ids"]

		if firstName == "" || lastName == "" {
			var u userEditData
			db.QueryRow(`SELECT id, email, first_name, last_name FROM users WHERE id = ?`, userID).
				Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName)
			u.FirstName = firstName
			u.LastName = lastName

			selectedIDs := map[int64]bool{}
			for _, s := range teamIDStrs {
				if tid, err := strconv.ParseInt(s, 10, 64); err == nil && tid > 0 {
					selectedIDs[tid] = true
				}
			}
			teams, _ := allTeams(db)
			re.Render(w, r, "users_edit.html", map[string]any{
				"Error":       "Імʼя та прізвище обовʼязкові",
				"User":        u,
				"Teams":       teams,
				"UserTeamIDs": selectedIDs,
			})
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := db.Exec(
			`UPDATE users SET first_name = ?, last_name = ?, updated_at = ? WHERE id = ?`,
			firstName, lastName, now, userID,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if _, err := db.Exec(`DELETE FROM team_members WHERE user_id = ?`, userID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		for _, tidStr := range teamIDStrs {
			tid, err := strconv.ParseInt(tidStr, 10, 64)
			if err != nil || tid <= 0 {
				continue
			}
			db.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, tid, userID)
		}

		v := url.Values{}
		v.Set("flash", "Збережено")
		http.Redirect(w, r, "/users?"+v.Encode(), http.StatusSeeOther)
	}
}

func HandleUsersDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		currentUser := middleware.GetUser(r)
		if currentUser != nil && currentUser.ID == userID {
			v := url.Values{}
			v.Set("error", "Не можна видалити власний акаунт")
			http.Redirect(w, r, "/users?"+v.Encode(), http.StatusSeeOther)
			return
		}

		var openCount int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM action_items ai
			JOIN action_statuses s ON ai.status_id = s.id
			WHERE ai.assignee_id = ? AND s.name NOT IN ('Виконано', 'Відмінено')
		`, userID).Scan(&openCount)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if openCount > 0 {
			v := url.Values{}
			v.Set("error", "Не можна видалити: є відкриті action items")
			http.Redirect(w, r, "/users?"+v.Encode(), http.StatusSeeOther)
			return
		}

		if _, err := db.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/users", http.StatusSeeOther)
	}
}

func HandleUsersResetPassword(db *sql.DB, port string, getIP func() string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&count); err != nil || count == 0 {
			http.NotFound(w, r)
			return
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		token := hex.EncodeToString(b)

		now := time.Now().UTC()
		if _, err := db.Exec(
			`UPDATE users SET reset_token = ?, reset_token_expires_at = ?, updated_at = ? WHERE id = ?`,
			token, now.Add(24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), userID,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		link := fmt.Sprintf("http://%s:%s/reset-password/%s", getIP(), port, token)
		v := url.Values{}
		v.Set("reset_link", link)
		http.Redirect(w, r, "/users?"+v.Encode(), http.StatusSeeOther)
	}
}

func allTeams(db *sql.DB) ([]teamItem, error) {
	rows, err := db.Query(`SELECT id, name FROM teams ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var teams []teamItem
	for rows.Next() {
		var t teamItem
		rows.Scan(&t.ID, &t.Name)
		teams = append(teams, t)
	}
	return teams, nil
}
