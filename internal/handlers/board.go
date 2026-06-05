package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/postup-app/postup/internal/middleware"
	"github.com/postup-app/postup/internal/models"
	"github.com/postup-app/postup/internal/ws"
)

type boardRetro struct {
	ID           int64
	TeamName     string
	TemplateName string
	Date         time.Time
	VoteLimit    int
	Status       string
}

func (b *boardRetro) IsActive() bool {
	return b.Status == "active" && b.Date.After(time.Now().Add(-time.Hour))
}

func isRetroMember(db *sql.DB, retroID, userID int64) bool {
	var count int
	db.QueryRow(`
		SELECT COUNT(*) FROM team_members tm
		JOIN retros r ON r.team_id = tm.team_id
		WHERE r.id = ? AND tm.user_id = ?`, retroID, userID).Scan(&count)
	return count > 0
}

func renderForbidden(w http.ResponseWriter, r *http.Request, re *Renderer) {
	w.WriteHeader(http.StatusForbidden)
	re.Render(w, r, "forbidden.html", nil)
}

func HandleBoardShow(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		if !isRetroMember(db, retroID, user.ID) {
			renderForbidden(w, r, re)
			return
		}

		var retro boardRetro
		var dateStr string
		err = db.QueryRow(`
			SELECT r.id, t.name, COALESCE(tmpl.name, ''), r.date, r.vote_limit, r.status
			FROM retros r
			JOIN teams t ON t.id = r.team_id
			LEFT JOIN templates tmpl ON tmpl.id = r.template_id
			WHERE r.id = ?`, retroID,
		).Scan(&retro.ID, &retro.TeamName, &retro.TemplateName, &dateStr, &retro.VoteLimit, &retro.Status)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		retro.Date, _ = time.Parse(time.RFC3339, dateStr)

		var templateID int64
		db.QueryRow(`SELECT COALESCE(template_id, 0) FROM retros WHERE id = ?`, retroID).Scan(&templateID)

		colRows, err := db.Query(`
			SELECT id, title, position, type
			FROM template_columns
			WHERE template_id = ?
			ORDER BY position ASC`, templateID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer colRows.Close()

		var columns []models.TemplateColumn
		for colRows.Next() {
			var col models.TemplateColumn
			col.TemplateID = templateID
			colRows.Scan(&col.ID, &col.Title, &col.Position, &col.Type)
			columns = append(columns, col)
		}

		cardRows, err := db.Query(`
			SELECT c.id, c.retro_id, c.column_id, c.author_id, c.content, c.created_at, c.updated_at,
			       u.first_name, u.last_name
			FROM cards c
			JOIN users u ON u.id = c.author_id
			WHERE c.retro_id = ?
			ORDER BY c.created_at ASC`, retroID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer cardRows.Close()

		cardsByColumn := map[int64][]models.CardWithAuthor{}
		for cardRows.Next() {
			var cwa models.CardWithAuthor
			var createdAt, updatedAt string
			cardRows.Scan(
				&cwa.ID, &cwa.RetroID, &cwa.ColumnID, &cwa.AuthorID, &cwa.Content,
				&createdAt, &updatedAt,
				&cwa.AuthorFirstName, &cwa.AuthorLastName,
			)
			cwa.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
			cwa.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
			cardsByColumn[cwa.ColumnID] = append(cardsByColumn[cwa.ColumnID], cwa)
		}

		// Total votes per card
		votesByCard := map[int64]int{}
		voteRows, err := db.Query(`
			SELECT v.card_id, SUM(v.count) as total
			FROM votes v
			JOIN cards c ON c.id = v.card_id
			WHERE c.retro_id = ?
			GROUP BY v.card_id`, retroID)
		if err == nil {
			defer voteRows.Close()
			for voteRows.Next() {
				var cid int64
				var total int
				voteRows.Scan(&cid, &total)
				votesByCard[cid] = total
			}
		}

		// Current user's votes per card
		myVotes := map[int64]int{}
		myVoteRows, err := db.Query(`
			SELECT v.card_id, v.count
			FROM votes v
			JOIN cards c ON c.id = v.card_id
			WHERE c.retro_id = ? AND v.user_id = ?`, retroID, user.ID)
		if err == nil {
			defer myVoteRows.Close()
			for myVoteRows.Next() {
				var cid int64
				var cnt int
				myVoteRows.Scan(&cid, &cnt)
				myVotes[cid] = cnt
			}
		}

		var votesUsed int
		db.QueryRow(`
			SELECT COALESCE(SUM(v.count), 0)
			FROM votes v
			JOIN cards c ON c.id = v.card_id
			WHERE c.retro_id = ? AND v.user_id = ?`, retroID, user.ID,
		).Scan(&votesUsed)

		// Action items keyed by column_id
		aiRows, err := db.Query(`
			SELECT ai.id, ai.retro_id, COALESCE(ai.column_id, 0), ai.assignee_id,
			       ai.content, ai.deadline, ai.status_id, ai.created_at,
			       u.first_name, u.last_name, s.name
			FROM action_items ai
			JOIN users u ON u.id = ai.assignee_id
			JOIN action_statuses s ON s.id = ai.status_id
			WHERE ai.retro_id = ?
			ORDER BY ai.created_at ASC`, retroID)
		var actionItemsByColumn map[int64][]models.ActionItemWithDetails
		if err == nil {
			defer aiRows.Close()
			actionItemsByColumn = map[int64][]models.ActionItemWithDetails{}
			for aiRows.Next() {
				var ai models.ActionItemWithDetails
				var deadlineStr, createdAt string
				aiRows.Scan(
					&ai.ID, &ai.RetroID, &ai.ColumnID, &ai.AssigneeID,
					&ai.Content, &deadlineStr, &ai.StatusID, &createdAt,
					&ai.AssigneeFirstName, &ai.AssigneeLastName, &ai.StatusName,
				)
				ai.Deadline, _ = time.Parse("2006-01-02", deadlineStr)
				ai.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
				actionItemsByColumn[ai.ColumnID] = append(actionItemsByColumn[ai.ColumnID], ai)
			}
		}

		// Participants for dropdown
		partRows, _ := db.Query(`
			SELECT u.id, u.first_name, u.last_name
			FROM users u
			JOIN team_members tm ON tm.user_id = u.id
			JOIN retros r ON r.team_id = tm.team_id
			WHERE r.id = ?
			ORDER BY u.first_name, u.last_name`, retroID)
		var participants []models.User
		if partRows != nil {
			defer partRows.Close()
			for partRows.Next() {
				var u models.User
				partRows.Scan(&u.ID, &u.FirstName, &u.LastName)
				participants = append(participants, u)
			}
		}

		// Statuses for dropdown
		stRows, _ := db.Query(`SELECT id, name, position, is_default FROM action_statuses ORDER BY position ASC`)
		var statuses []models.ActionStatus
		if stRows != nil {
			defer stRows.Close()
			for stRows.Next() {
				var s models.ActionStatus
				var isDefault int
				stRows.Scan(&s.ID, &s.Name, &s.Position, &isDefault)
				s.IsDefault = isDefault == 1
				statuses = append(statuses, s)
			}
		}

		statusesJSON, _ := json.Marshal(statuses)

		re.Render(w, r, "board.html", map[string]any{
			"Retro":               retro,
			"Columns":             columns,
			"CardsByColumn":       cardsByColumn,
			"ActionItemsByColumn": actionItemsByColumn,
			"Participants":        participants,
			"Statuses":            statuses,
			"StatusesJSON":        template.JS(statusesJSON),
			"CurrentUserID":       user.ID,
			"IsAdmin":             user.IsAdmin(),
			"IsActive":            retro.IsActive(),
			"VotesByCard":         votesByCard,
			"MyVotes":             myVotes,
			"VotesRemaining":      retro.VoteLimit - votesUsed,
		})
	}
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func loadCardByID(db *sql.DB, cardID int64) (*models.Card, error) {
	c := &models.Card{}
	var createdAt, updatedAt string
	err := db.QueryRow(
		`SELECT id, retro_id, column_id, author_id, content, created_at, updated_at FROM cards WHERE id = ?`,
		cardID,
	).Scan(&c.ID, &c.RetroID, &c.ColumnID, &c.AuthorID, &c.Content, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return c, nil
}

func columnBelongsToRetro(db *sql.DB, columnID, retroID int64) bool {
	var count int
	db.QueryRow(`
		SELECT COUNT(*) FROM template_columns tc
		JOIN retros r ON r.template_id = tc.template_id
		WHERE tc.id = ? AND r.id = ?`, columnID, retroID).Scan(&count)
	return count > 0
}

func HandleBoardWS(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		if !isRetroMember(db, retroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		conn, err := ws.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade retro=%d user=%d: %v", retroID, user.ID, err)
			return
		}

		client := &ws.Client{
			Hub:     hub,
			Conn:    conn,
			Send:    make(chan []byte, 256),
			RetroID: retroID,
			UserID:  user.ID,
		}
		hub.Register(client)

		go client.WritePump()
		go client.ReadPump()
	}
}

func HandleCardsCreate(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		if !isRetroMember(db, retroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var dateStr, status string
		err = db.QueryRow(`SELECT date, status FROM retros WHERE id = ?`, retroID).Scan(&dateStr, &status)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		retroDate, _ := time.Parse(time.RFC3339, dateStr)
		retro := &boardRetro{Status: status, Date: retroDate}
		if !retro.IsActive() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var columnIDStr, content string
		if wantsJSON(r) {
			var body struct {
				ColumnID int64  `json:"column_id"`
				Content  string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			columnIDStr = strconv.FormatInt(body.ColumnID, 10)
			content = body.Content
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			columnIDStr = r.FormValue("column_id")
			content = r.FormValue("content")
		}

		content = strings.TrimSpace(content)
		if content == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}
		if len([]rune(content)) > 1000 {
			http.Error(w, "content is too long", http.StatusBadRequest)
			return
		}

		columnID, _ := strconv.ParseInt(columnIDStr, 10, 64)
		if columnID <= 0 || !columnBelongsToRetro(db, columnID, retroID) {
			http.Error(w, "invalid column_id", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO cards (retro_id, column_id, author_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			retroID, columnID, user.ID, content, now, now,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		cardID, _ := res.LastInsertId()

		if hub != nil {
			hub.Broadcast(retroID, user.ID, "card_created", map[string]any{
				"id":          cardID,
				"column_id":   columnID,
				"author_id":   user.ID,
				"author_name": fmt.Sprintf("%s %s", user.FirstName, user.LastName),
				"content":     content,
				"created_at":  now,
			})
		}

		if wantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id":      cardID,
				"content": content,
				"author":  fmt.Sprintf("%s %s", user.FirstName, user.LastName),
			})
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/retros/%d/board", retroID), http.StatusSeeOther)
	}
}

func HandleCardsUpdate(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		card, err := loadCardByID(db, cardID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if !card.IsOwnedBy(user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var content string
		if wantsJSON(r) {
			var body struct {
				Content string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			content = body.Content
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			content = r.FormValue("content")
		}

		content = strings.TrimSpace(content)
		if content == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}
		if len([]rune(content)) > 1000 {
			http.Error(w, "content is too long", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := db.Exec(`UPDATE cards SET content = ?, updated_at = ? WHERE id = ?`, content, now, cardID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if hub != nil {
			hub.Broadcast(card.RetroID, user.ID, "card_updated", map[string]any{
				"id":      cardID,
				"content": content,
			})
		}

		if wantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"id": cardID, "content": content})
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/retros/%d/board", card.RetroID), http.StatusSeeOther)
	}
}

func HandleCardsDelete(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		card, err := loadCardByID(db, cardID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if !card.IsOwnedBy(user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if _, err := db.Exec(`DELETE FROM cards WHERE id = ?`, cardID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if hub != nil {
			hub.Broadcast(card.RetroID, user.ID, "card_deleted", map[string]any{
				"id": cardID,
			})
		}

		if wantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"deleted": true})
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/retros/%d/board", card.RetroID), http.StatusSeeOther)
	}
}

func HandleCardsMoveColumn(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		card, err := loadCardByID(db, cardID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if !card.IsOwnedBy(user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var body struct {
			ColumnID int64 `json:"column_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if body.ColumnID <= 0 || !columnBelongsToRetro(db, body.ColumnID, card.RetroID) {
			http.Error(w, "invalid column_id", http.StatusBadRequest)
			return
		}

		if _, err := db.Exec(`UPDATE cards SET column_id = ? WHERE id = ?`, body.ColumnID, cardID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if hub != nil {
			hub.Broadcast(card.RetroID, user.ID, "card_moved", map[string]any{
				"id":        cardID,
				"column_id": body.ColumnID,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

func HandleCardsCopy(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		card, err := loadCardByID(db, cardID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !isRetroMember(db, card.RetroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var sourceColumnTitle string
		if err := db.QueryRow(`SELECT title FROM template_columns WHERE id = ?`, card.ColumnID).
			Scan(&sourceColumnTitle); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var nextRetroID int64
		err = db.QueryRow(`
			SELECT id FROM retros
			WHERE team_id = (SELECT team_id FROM retros WHERE id = ?)
			  AND date > (SELECT date FROM retros WHERE id = ?)
			  AND status = 'active'
			ORDER BY date ASC LIMIT 1`,
			card.RetroID, card.RetroID,
		).Scan(&nextRetroID)
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"error": "Наступного ретро не знайдено"})
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var targetColumnID int64
		err = db.QueryRow(`
			SELECT tc.id FROM template_columns tc
			JOIN retros r ON r.template_id = tc.template_id
			WHERE r.id = ? AND tc.title = ?
			LIMIT 1`,
			nextRetroID, sourceColumnTitle,
		).Scan(&targetColumnID)
		if err == sql.ErrNoRows {
			if err2 := db.QueryRow(`
				SELECT tc.id FROM template_columns tc
				JOIN retros r ON r.template_id = tc.template_id
				WHERE r.id = ? AND tc.type = 'fixed_first'
				LIMIT 1`,
				nextRetroID,
			).Scan(&targetColumnID); err2 != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
		} else if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := db.Exec(
			`INSERT INTO cards (retro_id, column_id, author_id, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			nextRetroID, targetColumnID, user.ID, card.Content, now, now,
		); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "retro_id": nextRetroID})
	}
}

func HandleActionItemsCreate(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		if !isRetroMember(db, retroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var dateStr, status string
		err = db.QueryRow(`SELECT date, status FROM retros WHERE id = ?`, retroID).Scan(&dateStr, &status)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		retroDate, _ := time.Parse(time.RFC3339, dateStr)
		if !(&boardRetro{Status: status, Date: retroDate}).IsActive() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var content, deadlineStr, columnIDStr, assigneeIDStr string
		if wantsJSON(r) {
			var body struct {
				Content    string `json:"content"`
				AssigneeID int64  `json:"assignee_id"`
				Deadline   string `json:"deadline"`
				ColumnID   int64  `json:"column_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			content = body.Content
			deadlineStr = body.Deadline
			columnIDStr = strconv.FormatInt(body.ColumnID, 10)
			assigneeIDStr = strconv.FormatInt(body.AssigneeID, 10)
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			content = r.FormValue("content")
			deadlineStr = r.FormValue("deadline")
			columnIDStr = r.FormValue("column_id")
			assigneeIDStr = r.FormValue("assignee_id")
		}

		content = strings.TrimSpace(content)
		if content == "" {
			http.Error(w, "content is required", http.StatusBadRequest)
			return
		}
		if len([]rune(content)) > 500 {
			http.Error(w, "content is too long", http.StatusBadRequest)
			return
		}

		assigneeID, _ := strconv.ParseInt(assigneeIDStr, 10, 64)
		if assigneeID <= 0 {
			http.Error(w, "assignee_id is required", http.StatusBadRequest)
			return
		}
		if !isRetroMember(db, retroID, assigneeID) {
			http.Error(w, "assignee_id is not a participant", http.StatusBadRequest)
			return
		}

		if deadlineStr == "" {
			http.Error(w, "deadline is required", http.StatusBadRequest)
			return
		}
		deadline, err := time.Parse("2006-01-02", deadlineStr)
		if err != nil {
			http.Error(w, "invalid deadline format", http.StatusBadRequest)
			return
		}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if deadline.Before(today) {
			http.Error(w, "deadline cannot be in the past", http.StatusBadRequest)
			return
		}

		columnID, _ := strconv.ParseInt(columnIDStr, 10, 64)
		if columnID <= 0 || !columnBelongsToRetro(db, columnID, retroID) {
			http.Error(w, "invalid column_id", http.StatusBadRequest)
			return
		}

		var defaultStatusID int64
		if err := db.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 1 LIMIT 1`).
			Scan(&defaultStatusID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			retroID, columnID, assigneeID, content, deadline.Format("2006-01-02"), defaultStatusID, now,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		itemID, _ := res.LastInsertId()

		var assigneeFirstName, assigneeLastName, statusName string
		db.QueryRow(`SELECT first_name, last_name FROM users WHERE id = ?`, assigneeID).
			Scan(&assigneeFirstName, &assigneeLastName)
		db.QueryRow(`SELECT name FROM action_statuses WHERE id = ?`, defaultStatusID).
			Scan(&statusName)

		payload := map[string]any{
			"id":                  itemID,
			"retro_id":            retroID,
			"column_id":           columnID,
			"content":             content,
			"assignee_id":         assigneeID,
			"assignee_first_name": assigneeFirstName,
			"assignee_last_name":  assigneeLastName,
			"deadline":            deadline.Format("2006-01-02"),
			"status_id":           defaultStatusID,
			"status_name":         statusName,
		}

		if hub != nil {
			hub.Broadcast(retroID, user.ID, "action_item_created", payload)
		}

		if wantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/retros/%d/board", retroID), http.StatusSeeOther)
	}
}

func HandleCardsToActionItem(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)
		if !user.IsAdmin() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		card, err := loadCardByID(db, cardID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !isRetroMember(db, card.RetroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var dateStr, status string
		if err := db.QueryRow(`SELECT date, status FROM retros WHERE id = ?`, card.RetroID).
			Scan(&dateStr, &status); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		retroDate, _ := time.Parse(time.RFC3339, dateStr)
		if !(&boardRetro{Status: status, Date: retroDate}).IsActive() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var columnID int64
		if err := db.QueryRow(`
			SELECT tc.id FROM template_columns tc
			JOIN retros r ON r.template_id = tc.template_id
			WHERE r.id = ? AND tc.type = 'fixed_last'
			LIMIT 1`, card.RetroID).Scan(&columnID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var defaultStatusID int64
		if err := db.QueryRow(`SELECT id FROM action_statuses WHERE is_default = 1 LIMIT 1`).
			Scan(&defaultStatusID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var statusName string
		db.QueryRow(`SELECT name FROM action_statuses WHERE id = ?`, defaultStatusID).Scan(&statusName)

		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(
			`INSERT INTO action_items (retro_id, column_id, assignee_id, content, deadline, status_id, created_at) VALUES (?, ?, NULL, ?, NULL, ?, ?)`,
			card.RetroID, columnID, card.Content, defaultStatusID, now,
		)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		itemID, _ := res.LastInsertId()

		payload := map[string]any{
			"id":                  itemID,
			"retro_id":            card.RetroID,
			"column_id":           columnID,
			"content":             card.Content,
			"assignee_id":         nil,
			"assignee_first_name": "",
			"assignee_last_name":  "",
			"deadline":            "",
			"status_id":           defaultStatusID,
			"status_name":         statusName,
		}

		if hub != nil {
			hub.Broadcast(card.RetroID, user.ID, "action_item_created", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "item": payload})
	}
}

func HandleActionItemsUpdateStatus(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		itemID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		var retroID int64
		if err := db.QueryRow(`SELECT retro_id FROM action_items WHERE id = ?`, itemID).
			Scan(&retroID); err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		} else if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !isRetroMember(db, retroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var statusIDStr string
		if wantsJSON(r) {
			var body struct {
				StatusID int64 `json:"status_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			statusIDStr = strconv.FormatInt(body.StatusID, 10)
		} else {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			statusIDStr = r.FormValue("status_id")
		}

		statusID, _ := strconv.ParseInt(statusIDStr, 10, 64)
		if statusID <= 0 {
			http.Error(w, "status_id is required", http.StatusBadRequest)
			return
		}

		var statusCount int
		db.QueryRow(`SELECT COUNT(*) FROM action_statuses WHERE id = ?`, statusID).Scan(&statusCount)
		if statusCount == 0 {
			http.Error(w, "invalid status_id", http.StatusBadRequest)
			return
		}

		if _, err := db.Exec(`UPDATE action_items SET status_id = ? WHERE id = ?`, statusID, itemID); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var statusName string
		db.QueryRow(`SELECT name FROM action_statuses WHERE id = ?`, statusID).Scan(&statusName)

		if hub != nil {
			hub.Broadcast(retroID, user.ID, "action_item_status_updated", map[string]any{
				"id":          itemID,
				"status_id":   statusID,
				"status_name": statusName,
			})
		}

		if wantsJSON(r) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "status_name": statusName})
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/retros/%d/board", retroID), http.StatusSeeOther)
	}
}

func HandleCardsVote(db *sql.DB, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		user := middleware.GetUser(r)

		card, err := loadCardByID(db, cardID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		retroID := card.RetroID

		if !isRetroMember(db, retroID, user.ID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		var dateStr, status string
		var voteLimit int
		if err := db.QueryRow(`SELECT date, status, vote_limit FROM retros WHERE id = ?`, retroID).
			Scan(&dateStr, &status, &voteLimit); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		retroDate, _ := time.Parse(time.RFC3339, dateStr)
		if !(&boardRetro{Status: status, Date: retroDate}).IsActive() {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// Toggle: check if vote exists
		var existingCount int
		voteExists := true
		if err := db.QueryRow(`SELECT count FROM votes WHERE card_id = ? AND user_id = ?`,
			cardID, user.ID).Scan(&existingCount); err == sql.ErrNoRows {
			voteExists = false
		} else if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		replyJSON := func(payload map[string]any) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload)
		}

		var votesUsed int

		if voteExists {
			if _, err := db.Exec(`DELETE FROM votes WHERE card_id = ? AND user_id = ?`, cardID, user.ID); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			db.QueryRow(`
				SELECT COALESCE(SUM(v.count), 0) FROM votes v
				JOIN cards c ON c.id = v.card_id
				WHERE c.retro_id = ? AND v.user_id = ?`, retroID, user.ID,
			).Scan(&votesUsed)
		} else {
			var currentSum int
			db.QueryRow(`
				SELECT COALESCE(SUM(v.count), 0) FROM votes v
				JOIN cards c ON c.id = v.card_id
				WHERE c.retro_id = ? AND v.user_id = ?`, retroID, user.ID,
			).Scan(&currentSum)

			if currentSum >= voteLimit {
				replyJSON(map[string]any{"error": "ліміт голосів вичерпано", "remaining": 0})
				return
			}

			if _, err := db.Exec(`INSERT INTO votes (card_id, user_id, count) VALUES (?, ?, 1)`,
				cardID, user.ID); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			votesUsed = currentSum + 1
		}

		votesRemaining := voteLimit - votesUsed

		var totalVotes int
		db.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM votes WHERE card_id = ?`, cardID).Scan(&totalVotes)

		if hub != nil {
			hub.Broadcast(retroID, user.ID, "vote_updated", map[string]any{
				"card_id":         cardID,
				"total_votes":     totalVotes,
				"voter_id":        user.ID,
				"votes_remaining": votesRemaining,
			})
		}

		replyJSON(map[string]any{
			"ok":              true,
			"total_votes":     totalVotes,
			"votes_remaining": votesRemaining,
		})
	}
}
