package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/vladyslavkondratiuk/postup/internal/models"
)

func HandleStatusesIndex(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(
			`SELECT id, name, position, is_default FROM action_statuses ORDER BY position ASC`,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var statuses []models.ActionStatus
		for rows.Next() {
			var s models.ActionStatus
			var isDefault int
			if err := rows.Scan(&s.ID, &s.Name, &s.Position, &isDefault); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			s.IsDefault = isDefault == 1
			statuses = append(statuses, s)
		}

		editID, _ := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64)

		re.Render(w, r, "statuses.html", map[string]any{
			"Statuses":   statuses,
			"EditID":     editID,
			"Flash":      r.URL.Query().Get("flash"),
			"FlashError": r.URL.Query().Get("error"),
		})
	}
}

func HandleStatusesCreate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))

		redirectErr := func(msg string) {
			v := url.Values{}
			v.Set("error", msg)
			http.Redirect(w, r, "/statuses?"+v.Encode(), http.StatusSeeOther)
		}

		if name == "" {
			redirectErr("Назва статусу обовʼязкова")
			return
		}

		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM action_statuses WHERE name = ?`, name,
		).Scan(&count); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			redirectErr("Статус з такою назвою вже існує")
			return
		}

		var maxPos int
		db.QueryRow(`SELECT COALESCE(MAX(position), -1) FROM action_statuses`).Scan(&maxPos)

		if _, err := db.Exec(
			`INSERT INTO action_statuses (name, position, is_default) VALUES (?, ?, 0)`,
			name, maxPos+1,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		v := url.Values{}
		v.Set("flash", "Статус додано")
		http.Redirect(w, r, "/statuses?"+v.Encode(), http.StatusSeeOther)
	}
}

func HandleStatusesUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))

		redirectErr := func(msg string) {
			v := url.Values{}
			v.Set("error", msg)
			v.Set("edit", strconv.FormatInt(id, 10))
			http.Redirect(w, r, "/statuses?"+v.Encode(), http.StatusSeeOther)
		}

		if name == "" {
			redirectErr("Назва статусу обовʼязкова")
			return
		}

		var count int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM action_statuses WHERE name = ? AND id != ?`, name, id,
		).Scan(&count); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			redirectErr("Статус з такою назвою вже існує")
			return
		}

		var exists int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM action_statuses WHERE id = ?`, id,
		).Scan(&exists); err != nil || exists == 0 {
			http.NotFound(w, r)
			return
		}

		if _, err := db.Exec(
			`UPDATE action_statuses SET name = ? WHERE id = ?`, name, id,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		v := url.Values{}
		v.Set("flash", "Збережено")
		http.Redirect(w, r, "/statuses?"+v.Encode(), http.StatusSeeOther)
	}
}

func HandleStatusesDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		redirectErr := func(msg string) {
			v := url.Values{}
			v.Set("error", msg)
			http.Redirect(w, r, "/statuses?"+v.Encode(), http.StatusSeeOther)
		}

		var isDefault int
		if err := db.QueryRow(
			`SELECT is_default FROM action_statuses WHERE id = ?`, id,
		).Scan(&isDefault); err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if isDefault == 1 {
			redirectErr("Не можна видалити дефолтний статус")
			return
		}

		var usageCount int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM action_items WHERE status_id = ?`, id,
		).Scan(&usageCount); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if usageCount > 0 {
			redirectErr("Не можна видалити: статус використовується в action items")
			return
		}

		if _, err := db.Exec(`DELETE FROM action_statuses WHERE id = ?`, id); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/statuses", http.StatusSeeOther)
	}
}
