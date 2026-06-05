package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type teamListItem struct {
	ID           int64
	Name         string
	MembersCount int
}

type teamMemberItem struct {
	ID        int64
	Email     string
	FirstName string
	LastName  string
}

func HandleTeamsIndex(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT t.id, t.name, COUNT(tm.user_id) AS members_count
			FROM teams t
			LEFT JOIN team_members tm ON tm.team_id = t.id
			GROUP BY t.id
			ORDER BY t.name`)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var teams []teamListItem
		for rows.Next() {
			var t teamListItem
			if err := rows.Scan(&t.ID, &t.Name, &t.MembersCount); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			teams = append(teams, t)
		}

		re.Render(w, r, "teams.html", map[string]any{
			"Teams":      teams,
			"Flash":      r.URL.Query().Get("flash"),
			"FlashError": r.URL.Query().Get("error"),
		})
	}
}

func HandleTeamsNew(re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		re.Render(w, r, "teams_new.html", map[string]any{})
	}
}

func HandleTeamsCreate(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))

		renderErr := func(msg string) {
			re.Render(w, r, "teams_new.html", map[string]any{
				"Error": msg,
				"Name":  name,
			})
		}

		if name == "" {
			renderErr("Назва команди обовʼязкова")
			return
		}

		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM teams WHERE name = ?`, name).Scan(&count); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			renderErr("Команда з такою назвою вже існує")
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(`INSERT INTO teams (name, created_at) VALUES (?, ?)`, name, now)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		teamID, _ := res.LastInsertId()

		var adminID int64
		if err := db.QueryRow(`SELECT id FROM users WHERE role = 'admin'`).Scan(&adminID); err == nil {
			db.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, adminID)
		}

		v := url.Values{}
		v.Set("flash", "Команду створено")
		http.Redirect(w, r, "/teams?"+v.Encode(), http.StatusSeeOther)
	}
}

type teamEditData struct {
	ID   int64
	Name string
}

func HandleTeamsEdit(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var t teamEditData
		err = db.QueryRow(`SELECT id, name FROM teams WHERE id = ?`, teamID).Scan(&t.ID, &t.Name)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		memberRows, err := db.Query(`SELECT user_id FROM team_members WHERE team_id = ?`, teamID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer memberRows.Close()
		memberIDs := map[int64]bool{}
		for memberRows.Next() {
			var uid int64
			memberRows.Scan(&uid)
			memberIDs[uid] = true
		}

		userRows, err := db.Query(`SELECT id, email, first_name, last_name FROM users ORDER BY first_name, last_name`)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer userRows.Close()
		var users []teamMemberItem
		for userRows.Next() {
			var u teamMemberItem
			userRows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName)
			users = append(users, u)
		}

		re.Render(w, r, "teams_edit.html", map[string]any{
			"Team":      t,
			"Users":     users,
			"MemberIDs": memberIDs,
		})
	}
}

func HandleTeamsUpdate(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		userIDStrs := r.Form["user_ids"]

		renderErr := func(msg string) {
			memberRows, _ := db.Query(`SELECT user_id FROM team_members WHERE team_id = ?`, teamID)
			memberIDs := map[int64]bool{}
			if memberRows != nil {
				defer memberRows.Close()
				for memberRows.Next() {
					var uid int64
					memberRows.Scan(&uid)
					memberIDs[uid] = true
				}
			}
			userRows, _ := db.Query(`SELECT id, email, first_name, last_name FROM users ORDER BY first_name, last_name`)
			var users []teamMemberItem
			if userRows != nil {
				defer userRows.Close()
				for userRows.Next() {
					var u teamMemberItem
					userRows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName)
					users = append(users, u)
				}
			}
			re.Render(w, r, "teams_edit.html", map[string]any{
				"Error":     msg,
				"Team":      teamEditData{ID: teamID, Name: name},
				"Users":     users,
				"MemberIDs": memberIDs,
			})
		}

		if name == "" {
			renderErr("Назва команди обовʼязкова")
			return
		}

		if _, err := db.Exec(`UPDATE teams SET name = ? WHERE id = ?`, name, teamID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if _, err := db.Exec(`DELETE FROM team_members WHERE team_id = ?`, teamID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		for _, uidStr := range userIDStrs {
			uid, err := strconv.ParseInt(uidStr, 10, 64)
			if err != nil || uid <= 0 {
				continue
			}
			db.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, uid)
		}

		var adminID int64
		if err := db.QueryRow(`SELECT id FROM users WHERE role = 'admin'`).Scan(&adminID); err == nil {
			db.Exec(`INSERT OR IGNORE INTO team_members (team_id, user_id) VALUES (?, ?)`, teamID, adminID)
		}

		v := url.Values{}
		v.Set("flash", "Збережено")
		http.Redirect(w, r, "/teams?"+v.Encode(), http.StatusSeeOther)
	}
}

func HandleTeamsDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var memberCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM team_members WHERE team_id = ?`, teamID).Scan(&memberCount); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if memberCount > 0 {
			v := url.Values{}
			v.Set("error", "Не можна видалити: команда має учасників")
			http.Redirect(w, r, "/teams?"+v.Encode(), http.StatusSeeOther)
			return
		}

		if _, err := db.Exec(`DELETE FROM teams WHERE id = ?`, teamID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/teams", http.StatusSeeOther)
	}
}
