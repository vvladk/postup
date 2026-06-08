package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/postup-app/postup/internal/models"
)

func HandleTemplatesIndex(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT t.id, t.name, t.status, t.created_at, COUNT(r.id) AS retros_count
			FROM templates t
			LEFT JOIN retros r ON r.template_id = t.id
			GROUP BY t.id
			ORDER BY t.created_at DESC`)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var templates []models.Template
		for rows.Next() {
			var tmpl models.Template
			var createdAt string
			if err := rows.Scan(&tmpl.ID, &tmpl.Name, &tmpl.Status, &createdAt, &tmpl.RetrosCount); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			tmpl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
			templates = append(templates, tmpl)
		}

		if len(templates) > 0 {
			colRows, err := db.Query(`
				SELECT id, template_id, title, position, type
				FROM template_columns
				ORDER BY template_id, position`)
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			defer colRows.Close()

			colsByTemplate := map[int64][]models.TemplateColumn{}
			for colRows.Next() {
				var col models.TemplateColumn
				if err := colRows.Scan(&col.ID, &col.TemplateID, &col.Title, &col.Position, &col.Type); err != nil {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				colsByTemplate[col.TemplateID] = append(colsByTemplate[col.TemplateID], col)
			}

			for i := range templates {
				templates[i].Columns = colsByTemplate[templates[i].ID]
			}
		}

		re.Render(w, r, "templates.html", map[string]any{
			"Templates":  templates,
			"Flash":      r.URL.Query().Get("flash"),
			"FlashError": r.URL.Query().Get("error"),
		})
	}
}

func HandleTemplatesNew(re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		re.Render(w, r, "templates_new.html", map[string]any{})
	}
}

func HandleTemplatesCreate(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(r.FormValue("name"))
		columns := sortedColumns(r.Form["columns[]"], r.Form["positions[]"])

		renderErr := func(msg string) {
			re.Render(w, r, "templates_new.html", map[string]any{
				"Error":   msg,
				"Name":    name,
				"Columns": columns,
			})
		}

		if errMsg := validateTemplateForm(name, columns); errMsg != "" {
			renderErr(errMsg)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)

		res, err := db.Exec(`INSERT INTO templates (name, status, created_at) VALUES (?, 'active', ?)`, name, now)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		templateID, err := res.LastInsertId()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if err := insertTemplateColumns(db, templateID, columns); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		v := url.Values{}
		v.Set("flash", "Шаблон створено")
		http.Redirect(w, r, "/templates?"+v.Encode(), http.StatusSeeOther)
	}
}

func sortedColumns(rawCols, rawPos []string) []string {
	type item struct {
		title string
		pos   int
	}
	items := make([]item, len(rawCols))
	for i, c := range rawCols {
		pos := i + 1
		if i < len(rawPos) {
			if p, err := strconv.Atoi(rawPos[i]); err == nil && p > 0 {
				pos = p
			}
		}
		items[i] = item{strings.TrimSpace(c), pos}
	}
	sort.Slice(items, func(a, b int) bool { return items[a].pos < items[b].pos })
	titles := make([]string, len(items))
	for i, it := range items {
		titles[i] = it.title
	}
	return titles
}

func insertTemplateColumns(db *sql.DB, templateID int64, regularColumns []string) error {
	if _, err := db.Exec(
		`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, 'Незакриті Action Items', 0, 'fixed_first')`,
		templateID,
	); err != nil {
		return err
	}
	for i, title := range regularColumns {
		if _, err := db.Exec(
			`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, ?, ?, 'regular')`,
			templateID, title, i+1,
		); err != nil {
			return err
		}
	}
	lastPos := len(regularColumns) + 1
	if _, err := db.Exec(
		`INSERT INTO template_columns (template_id, title, position, type) VALUES (?, 'Action Items', ?, 'fixed_last')`,
		templateID, lastPos,
	); err != nil {
		return err
	}
	return nil
}

func loadTemplateWithRetrosCount(db *sql.DB, id int64) (models.Template, error) {
	var tmpl models.Template
	var createdAt string
	err := db.QueryRow(`
		SELECT t.id, t.name, t.status, t.created_at, COUNT(r.id)
		FROM templates t
		LEFT JOIN retros r ON r.template_id = t.id
		WHERE t.id = ?
		GROUP BY t.id`, id,
	).Scan(&tmpl.ID, &tmpl.Name, &tmpl.Status, &createdAt, &tmpl.RetrosCount)
	if err != nil {
		return tmpl, err
	}
	tmpl.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return tmpl, nil
}

func loadTemplateColumns(db *sql.DB, templateID int64) ([]models.TemplateColumn, error) {
	rows, err := db.Query(
		`SELECT id, template_id, title, position, type FROM template_columns WHERE template_id = ? ORDER BY position`,
		templateID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []models.TemplateColumn
	for rows.Next() {
		var col models.TemplateColumn
		if err := rows.Scan(&col.ID, &col.TemplateID, &col.Title, &col.Position, &col.Type); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, nil
}

func validateTemplateForm(name string, columns []string) string {
	if name == "" {
		return "Назва шаблону обовʼязкова"
	}
	if len(columns) < 1 || len(columns) > 5 {
		return "Потрібно від 1 до 5 середніх колонок"
	}
	for _, c := range columns {
		if c == "" {
			return "Назва кожної колонки не може бути порожньою"
		}
	}
	return ""
}

func HandleTemplatesEdit(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		tmpl, err := loadTemplateWithRetrosCount(db, id)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if tmpl.IsArchived() {
			v := url.Values{}
			v.Set("error", "Архівований шаблон не можна редагувати")
			http.Redirect(w, r, "/templates?"+v.Encode(), http.StatusSeeOther)
			return
		}

		cols, err := loadTemplateColumns(db, id)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var regularCols []models.TemplateColumn
		for _, c := range cols {
			if c.Type == "regular" {
				regularCols = append(regularCols, c)
			}
		}

		re.Render(w, r, "templates_edit.html", map[string]any{
			"Template":   tmpl,
			"Columns":    regularCols,
			"HasRetros":  tmpl.HasRetros(),
		})
	}
}

func HandleTemplatesUpdate(db *sql.DB, re *Renderer) http.HandlerFunc {
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
		columns := sortedColumns(r.Form["columns[]"], r.Form["positions[]"])

		tmpl, err := loadTemplateWithRetrosCount(db, id)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		renderErr := func(msg string) {
			var regularCols []models.TemplateColumn
			for i, title := range columns {
				regularCols = append(regularCols, models.TemplateColumn{Title: title, Position: i + 1, Type: "regular"})
			}
			re.Render(w, r, "templates_edit.html", map[string]any{
				"Error":     msg,
				"Template":  tmpl,
				"Columns":   regularCols,
				"HasRetros": tmpl.HasRetros(),
			})
		}

		if errMsg := validateTemplateForm(name, columns); errMsg != "" {
			renderErr(errMsg)
			return
		}

		if !tmpl.HasRetros() {
			if _, err := db.Exec(`UPDATE templates SET name = ? WHERE id = ?`, name, id); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if _, err := db.Exec(`DELETE FROM template_columns WHERE template_id = ?`, id); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			if err := insertTemplateColumns(db, id, columns); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			v := url.Values{}
			v.Set("flash", "Збережено")
			http.Redirect(w, r, "/templates?"+v.Encode(), http.StatusSeeOther)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO templates (name, status, created_at) VALUES (?, 'active', ?)`,
			name+" (копія)", now,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		newID, err := res.LastInsertId()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if err := insertTemplateColumns(db, newID, columns); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		v := url.Values{}
		v.Set("flash", "Створено копію шаблону")
		http.Redirect(w, r, "/templates?"+v.Encode(), http.StatusSeeOther)
	}
}

func HandleTemplatesArchive(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if _, err := db.Exec(`UPDATE templates SET status = 'archived' WHERE id = ?`, id); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/templates", http.StatusSeeOther)
	}
}

func HandleTemplatesDelete(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		tmpl, err := loadTemplateWithRetrosCount(db, id)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if tmpl.HasRetros() {
			v := url.Values{}
			v.Set("error", "Не можна видалити: є ретроспективи з цим шаблоном")
			http.Redirect(w, r, "/templates?"+v.Encode(), http.StatusSeeOther)
			return
		}

		if _, err := db.Exec(`DELETE FROM template_columns WHERE template_id = ?`, id); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if _, err := db.Exec(`DELETE FROM templates WHERE id = ?`, id); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/templates", http.StatusSeeOther)
	}
}
