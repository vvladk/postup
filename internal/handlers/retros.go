package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/postup-app/postup/internal/middleware"
	"github.com/postup-app/postup/internal/session"
)

type retroListItem struct {
	ID                int64
	TeamName          string
	TemplateName      string
	Date              time.Time
	VoteLimit         int
	Status            string
	ParticipantsCount int
	IsActive          bool
}

type teamFilterItem struct {
	ID   int64
	Name string
}

type memberRetroItem struct {
	ID       int64
	TeamName string
	Date     time.Time
	Status   string
	IsActive bool
}

func HandleRetrosPMIndex(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		cutoff := now.Add(-time.Hour).Format(time.RFC3339)

		activeRows, err := db.Query(`
			SELECT r.id, t.name, COALESCE(tmpl.name, ''), r.date, r.vote_limit, r.status,
			       COUNT(rp.user_id)
			FROM retros r
			JOIN teams t ON t.id = r.team_id
			LEFT JOIN templates tmpl ON tmpl.id = r.template_id
			LEFT JOIN retro_participants rp ON rp.retro_id = r.id
			WHERE r.status = 'active' AND r.date > ?
			GROUP BY r.id
			ORDER BY r.date ASC`, cutoff)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer activeRows.Close()

		var activeRetros []retroListItem
		for activeRows.Next() {
			var item retroListItem
			var dateStr string
			if err := activeRows.Scan(&item.ID, &item.TeamName, &item.TemplateName,
				&dateStr, &item.VoteLimit, &item.Status, &item.ParticipantsCount); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			item.Date, _ = time.Parse(time.RFC3339, dateStr)
			item.IsActive = true
			activeRetros = append(activeRetros, item)
		}

		dateFrom := r.URL.Query().Get("date_from")
		dateTo := r.URL.Query().Get("date_to")
		teamIDStr := r.URL.Query().Get("team_id")
		var teamIDFilter int64
		if teamIDStr != "" {
			teamIDFilter, _ = strconv.ParseInt(teamIDStr, 10, 64)
		}

		sort := r.URL.Query().Get("sort")
		if sort != "asc" {
			sort = "desc"
		}
		nextSort := "asc"
		if sort == "asc" {
			nextSort = "desc"
		}

		var where []string
		var args []any

		if dateFrom != "" {
			if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
				where = append(where, "r.date >= ?")
				args = append(args, t.UTC().Format(time.RFC3339))
			}
		}
		if dateTo != "" {
			if t, err := time.Parse("2006-01-02", dateTo); err == nil {
				end := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
				where = append(where, "r.date <= ?")
				args = append(args, end.Format(time.RFC3339))
			}
		}
		if teamIDFilter > 0 {
			where = append(where, "r.team_id = ?")
			args = append(args, teamIDFilter)
		}

		whereClause := ""
		if len(where) > 0 {
			whereClause = "WHERE " + strings.Join(where, " AND ")
		}

		orderDir := "DESC"
		if sort == "asc" {
			orderDir = "ASC"
		}

		query := fmt.Sprintf(`
			SELECT r.id, t.name, COALESCE(tmpl.name, ''), r.date, r.vote_limit, r.status,
			       COUNT(rp.user_id)
			FROM retros r
			JOIN teams t ON t.id = r.team_id
			LEFT JOIN templates tmpl ON tmpl.id = r.template_id
			LEFT JOIN retro_participants rp ON rp.retro_id = r.id
			%s
			GROUP BY r.id
			ORDER BY r.date %s`, whereClause, orderDir)

		allRows, err := db.Query(query, args...)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer allRows.Close()

		var allRetros []retroListItem
		for allRows.Next() {
			var item retroListItem
			var dateStr string
			if err := allRows.Scan(&item.ID, &item.TeamName, &item.TemplateName,
				&dateStr, &item.VoteLimit, &item.Status, &item.ParticipantsCount); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			item.Date, _ = time.Parse(time.RFC3339, dateStr)
			item.IsActive = item.Status == "active" && item.Date.After(now.Add(-time.Hour))
			allRetros = append(allRetros, item)
		}

		teamRows, err := db.Query(`SELECT id, name FROM teams ORDER BY name`)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer teamRows.Close()

		var teams []teamFilterItem
		for teamRows.Next() {
			var t teamFilterItem
			teamRows.Scan(&t.ID, &t.Name)
			teams = append(teams, t)
		}

		var totalRetros int
		db.QueryRow(`SELECT COUNT(*) FROM retros`).Scan(&totalRetros)

		hasFilters := dateFrom != "" || dateTo != "" || teamIDFilter > 0

		re.Render(w, r, "retros_pm.html", map[string]any{
			"ActiveRetros":   activeRetros,
			"AllRetros":      allRetros,
			"Teams":          teams,
			"DateFrom":       dateFrom,
			"DateTo":         dateTo,
			"TeamID":         teamIDFilter,
			"Sort":           sort,
			"NextSort":       nextSort,
			"ShowTeamFilter": len(teams) > 1,
			"HasFilters":     hasFilters,
			"HasRetros":      totalRetros > 0,
			"FlashError":     r.URL.Query().Get("error"),
		})
	}
}

func HandleRetrosMemberIndex(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := middleware.GetUser(r)
		now := time.Now().UTC()

		rows, err := db.Query(`
			SELECT r.id, t.name, r.date, r.status
			FROM retros r
			JOIN teams t ON t.id = r.team_id
			JOIN retro_participants rp ON rp.retro_id = r.id
			WHERE rp.user_id = ?
			ORDER BY r.date DESC`, user.ID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var active, past []memberRetroItem
		for rows.Next() {
			var item memberRetroItem
			var dateStr string
			if err := rows.Scan(&item.ID, &item.TeamName, &dateStr, &item.Status); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			item.Date, _ = time.Parse(time.RFC3339, dateStr)
			item.IsActive = item.Status == "active" && item.Date.After(now.Add(-time.Hour))
			if item.IsActive {
				active = append(active, item)
			} else {
				past = append(past, item)
			}
		}

		re.Render(w, r, "retros_member.html", map[string]any{
			"Active": active,
			"Past":   past,
		})
	}
}

type retroNewTeam struct {
	ID   int64
	Name string
}

type retroNewTemplate struct {
	ID             int64
	Name           string
	ColumnsPreview string
}

func loadRetroNewData(db *sql.DB) (teams []retroNewTeam, templates []retroNewTemplate, err error) {
	teamRows, err := db.Query(`SELECT id, name FROM teams ORDER BY name`)
	if err != nil {
		return
	}
	defer teamRows.Close()
	for teamRows.Next() {
		var t retroNewTeam
		teamRows.Scan(&t.ID, &t.Name)
		teams = append(teams, t)
	}

	tmplRows, err := db.Query(`
		SELECT t.id, t.name, tc.title
		FROM templates t
		JOIN template_columns tc ON tc.template_id = t.id
		WHERE t.status = 'active'
		ORDER BY t.id, tc.position`)
	if err != nil {
		return
	}
	defer tmplRows.Close()

	tmplMap := map[int64]*retroNewTemplate{}
	var tmplOrder []int64
	for tmplRows.Next() {
		var id int64
		var name, colTitle string
		tmplRows.Scan(&id, &name, &colTitle)
		if _, ok := tmplMap[id]; !ok {
			tmplMap[id] = &retroNewTemplate{ID: id, Name: name}
			tmplOrder = append(tmplOrder, id)
		}
		t := tmplMap[id]
		if t.ColumnsPreview != "" {
			t.ColumnsPreview += " · "
		}
		t.ColumnsPreview += colTitle
	}
	for _, id := range tmplOrder {
		templates = append(templates, *tmplMap[id])
	}
	return
}

func HandleRetrosNew(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teams, templates, err := loadRetroNewData(db)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if len(teams) == 0 || len(templates) == 0 {
			v := url.Values{}
			v.Set("error", "Спочатку створи команду і шаблон")
			http.Redirect(w, r, "/?"+v.Encode(), http.StatusSeeOther)
			return
		}
		re.Render(w, r, "retros_new.html", map[string]any{
			"Teams":     teams,
			"Templates": templates,
			"VoteLimit": 10,
		})
	}
}

func HandleRetrosCreate(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		teamIDStr := r.FormValue("team_id")
		templateIDStr := r.FormValue("template_id")
		dateStr := r.FormValue("date")
		voteLimitStr := r.FormValue("vote_limit")

		teamID, _ := strconv.ParseInt(teamIDStr, 10, 64)
		templateID, _ := strconv.ParseInt(templateIDStr, 10, 64)

		voteLimit := 10
		voteLimitValid := true
		if voteLimitStr != "" {
			if v, err := strconv.Atoi(voteLimitStr); err != nil || v < 1 || v > 50 {
				voteLimitValid = false
			} else {
				voteLimit = v
			}
		}

		teams, templates, err := loadRetroNewData(db)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		renderErr := func(msg string) {
			re.Render(w, r, "retros_new.html", map[string]any{
				"Error":      msg,
				"Teams":      teams,
				"Templates":  templates,
				"TeamID":     teamID,
				"TemplateID": templateID,
				"Date":       dateStr,
				"VoteLimit":  voteLimit,
			})
		}

		if !voteLimitValid {
			renderErr("Ліміт голосів має бути від 1 до 50")
			return
		}

		if teamID <= 0 {
			renderErr("Оберіть команду")
			return
		}
		if templateID <= 0 {
			renderErr("Оберіть шаблон")
			return
		}

		var teamCount int
		db.QueryRow(`SELECT COUNT(*) FROM teams WHERE id = ?`, teamID).Scan(&teamCount)
		if teamCount == 0 {
			renderErr("Команду не знайдено")
			return
		}

		var tmplCount int
		db.QueryRow(`SELECT COUNT(*) FROM templates WHERE id = ? AND status = 'active'`, templateID).Scan(&tmplCount)
		if tmplCount == 0 {
			renderErr("Шаблон не знайдено або заархівований")
			return
		}

		if dateStr == "" {
			renderErr("Вкажи дату і час")
			return
		}
		retroDate, err := time.Parse("2006-01-02T15:04", dateStr)
		if err != nil {
			renderErr("Невірний формат дати")
			return
		}
		if !retroDate.After(time.Now()) {
			renderErr("Дата має бути в майбутньому")
			return
		}

		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		inviteToken := hex.EncodeToString(b)

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO retros (team_id, template_id, date, vote_limit, status, invite_token, created_at)
			 VALUES (?, ?, ?, ?, 'active', ?, ?)`,
			teamID, templateID, retroDate.UTC().Format(time.RFC3339), voteLimit, inviteToken, now,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		retroID, _ := res.LastInsertId()

		memberRows, err := db.Query(`SELECT user_id FROM team_members WHERE team_id = ?`, teamID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		var memberIDs []int64
		for memberRows.Next() {
			var uid int64
			memberRows.Scan(&uid)
			memberIDs = append(memberIDs, uid)
		}
		memberRows.Close()

		for _, uid := range memberIDs {
			db.Exec(`INSERT OR IGNORE INTO retro_participants (retro_id, user_id) VALUES (?, ?)`, retroID, uid)
		}

		http.Redirect(w, r, fmt.Sprintf("/retros/%d/invite-link", retroID), http.StatusSeeOther)
	}
}

func HandleRetrosInvite(db *sql.DB, re *Renderer, port string, getIP func() string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var inviteToken sql.NullString
		var teamName, dateStr string
		err = db.QueryRow(`
			SELECT r.invite_token, t.name, r.date
			FROM retros r
			JOIN teams t ON t.id = r.team_id
			WHERE r.id = ?`, retroID).Scan(&inviteToken, &teamName, &dateStr)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		inviteLink := ""
		if inviteToken.Valid {
			inviteLink = fmt.Sprintf("http://%s:%s/join/%s", getIP(), port, inviteToken.String)
		}

		retroDate, _ := time.Parse(time.RFC3339, dateStr)

		re.Render(w, r, "retros_invite.html", map[string]any{
			"RetroID":    retroID,
			"TeamName":   teamName,
			"Date":       retroDate,
			"InviteLink": inviteLink,
		})
	}
}

func HandleRetrosEdit(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		var dateStr, status, teamName string
		var voteLimit int
		err = db.QueryRow(`
			SELECT r.date, r.vote_limit, r.status, t.name
			FROM retros r
			JOIN teams t ON t.id = r.team_id
			WHERE r.id = ?`, retroID).Scan(&dateStr, &voteLimit, &status, &teamName)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if status != "active" {
			v := url.Values{}
			v.Set("error", "Завершене ретро не можна редагувати")
			http.Redirect(w, r, "/?"+v.Encode(), http.StatusSeeOther)
			return
		}

		retroDate, _ := time.Parse(time.RFC3339, dateStr)

		re.Render(w, r, "retros_edit.html", map[string]any{
			"RetroID":   retroID,
			"TeamName":  teamName,
			"Date":      retroDate.Format("2006-01-02T15:04"),
			"VoteLimit": voteLimit,
		})
	}
}

func HandleRetrosUpdate(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		dateStr := r.FormValue("date")
		voteLimitStr := r.FormValue("vote_limit")

		voteLimit := 10
		voteLimitValid := true
		if voteLimitStr != "" {
			if v, err := strconv.Atoi(voteLimitStr); err != nil || v < 1 || v > 50 {
				voteLimitValid = false
			} else {
				voteLimit = v
			}
		}

		renderErr := func(msg string) {
			var teamName string
			db.QueryRow(`SELECT t.name FROM retros r JOIN teams t ON t.id = r.team_id WHERE r.id = ?`, retroID).Scan(&teamName)
			re.Render(w, r, "retros_edit.html", map[string]any{
				"Error":     msg,
				"RetroID":   retroID,
				"TeamName":  teamName,
				"Date":      dateStr,
				"VoteLimit": voteLimit,
			})
		}

		if dateStr == "" {
			renderErr("Вкажи дату і час")
			return
		}
		retroDate, err := time.Parse("2006-01-02T15:04", dateStr)
		if err != nil {
			renderErr("Невірний формат дати")
			return
		}
		if !retroDate.After(time.Now()) {
			renderErr("Дата має бути в майбутньому")
			return
		}
		if !voteLimitValid {
			renderErr("Ліміт голосів має бути від 1 до 50")
			return
		}

		if _, err := db.Exec(
			`UPDATE retros SET date = ?, vote_limit = ? WHERE id = ? AND status = 'active'`,
			retroDate.UTC().Format(time.RFC3339), voteLimit, retroID,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func HandleRetrosFinish(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if _, err := db.Exec(`UPDATE retros SET status = 'finished' WHERE id = ?`, retroID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		transferActionItems(db, retroID)

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func transferActionItems(db *sql.DB, retroID int64) {
	var teamID int64
	var dateStr string
	if err := db.QueryRow(`SELECT team_id, date FROM retros WHERE id = ?`, retroID).
		Scan(&teamID, &dateStr); err != nil {
		log.Printf("transferActionItems: load retro %d: %v", retroID, err)
		return
	}

	var fixedLastID int64
	if err := db.QueryRow(`
		SELECT tc.id FROM template_columns tc
		JOIN retros r ON r.template_id = tc.template_id
		WHERE r.id = ? AND tc.type = 'fixed_last'`, retroID).
		Scan(&fixedLastID); err != nil {
		return // retro has no template or no fixed_last column
	}

	type aiRow struct {
		AssigneeID int64
		Content    string
		Deadline   string
		StatusID   int64
	}
	rows, err := db.Query(`
		SELECT assignee_id, content, deadline, status_id
		FROM action_items
		WHERE retro_id = ? AND column_id = ?`, retroID, fixedLastID)
	if err != nil {
		log.Printf("transferActionItems: query items: %v", err)
		return
	}
	defer rows.Close()

	var items []aiRow
	for rows.Next() {
		var ai aiRow
		rows.Scan(&ai.AssigneeID, &ai.Content, &ai.Deadline, &ai.StatusID)
		items = append(items, ai)
	}
	rows.Close()

	if len(items) == 0 {
		return
	}

	var nextRetroID int64
	if err := db.QueryRow(`
		SELECT id FROM retros
		WHERE team_id = ? AND date > ? AND status = 'active'
		ORDER BY date ASC LIMIT 1`, teamID, dateStr).
		Scan(&nextRetroID); err != nil {
		return // no next retro — nothing to transfer
	}

	var fixedFirstID int64
	if err := db.QueryRow(`
		SELECT tc.id FROM template_columns tc
		JOIN retros r ON r.template_id = tc.template_id
		WHERE r.id = ? AND tc.type = 'fixed_first'`, nextRetroID).
		Scan(&fixedFirstID); err != nil {
		log.Printf("transferActionItems: no fixed_first in next retro %d: %v", nextRetroID, err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, ai := range items {
		if _, err := db.Exec(`
			INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			nextRetroID, fixedFirstID, ai.AssigneeID, ai.Content, ai.Deadline, ai.StatusID, now,
		); err != nil {
			log.Printf("transferActionItems: insert into retro %d: %v", nextRetroID, err)
		}
	}
}

func HandleRetrosJoin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")

		var retroID int64
		err := db.QueryRow(`SELECT id FROM retros WHERE invite_token = ?`, token).Scan(&retroID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		cookie, err := r.Cookie("session_id")
		if err != nil {
			http.Redirect(w, r, "/login?next=/join/"+token, http.StatusSeeOther)
			return
		}
		sess, err := session.Get(db, cookie.Value)
		if err != nil {
			http.Redirect(w, r, "/login?next=/join/"+token, http.StatusSeeOther)
			return
		}

		db.Exec(`INSERT OR IGNORE INTO retro_participants (retro_id, user_id) VALUES (?, ?)`, retroID, sess.UserID)

		http.Redirect(w, r, fmt.Sprintf("/retros/%d/board", retroID), http.StatusSeeOther)
	}
}
