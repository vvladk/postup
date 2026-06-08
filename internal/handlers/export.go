package handlers

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type exportRetro struct {
	TeamName string
	Date     time.Time
}

type exportCol struct {
	ID    int64
	Title string
}

type exportCard struct {
	Content   string
	FirstName string
	LastName  string
	Votes     int
}

type exportAI struct {
	Content   string
	FirstName string
	LastName  string
	Deadline  time.Time
}

type exportData struct {
	Retro       exportRetro
	RegularCols []exportCol
	Cards       map[int64][]exportCard
	ActionItems []exportAI
}

func loadExportData(db *sql.DB, retroID int64) (*exportData, error) {
	var data exportData

	var dateStr string
	err := db.QueryRow(`
		SELECT t.name, r.date
		FROM retros r JOIN teams t ON t.id = r.team_id
		WHERE r.id = ?`, retroID,
	).Scan(&data.Retro.TeamName, &dateStr)
	if err != nil {
		return nil, err
	}
	data.Retro.Date, _ = time.Parse(time.RFC3339, dateStr)

	var templateID int64
	db.QueryRow(`SELECT COALESCE(template_id, 0) FROM retros WHERE id = ?`, retroID).Scan(&templateID)

	colRows, err := db.Query(`
		SELECT id, title, type FROM template_columns
		WHERE template_id = ? ORDER BY position ASC`, templateID)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()

	var lastColID int64
	for colRows.Next() {
		var id int64
		var title, colType string
		colRows.Scan(&id, &title, &colType)
		switch colType {
		case "fixed_first":
			// skip
		case "fixed_last":
			lastColID = id
		default:
			data.RegularCols = append(data.RegularCols, exportCol{ID: id, Title: title})
		}
	}

	votesByCard := map[int64]int{}
	voteRows, _ := db.Query(`
		SELECT v.card_id, SUM(v.count)
		FROM votes v JOIN cards c ON c.id = v.card_id
		WHERE c.retro_id = ? GROUP BY v.card_id`, retroID)
	if voteRows != nil {
		defer voteRows.Close()
		for voteRows.Next() {
			var cid int64
			var total int
			voteRows.Scan(&cid, &total)
			votesByCard[cid] = total
		}
	}

	cardRows, err := db.Query(`
		SELECT c.id, c.column_id, c.content, u.first_name, u.last_name
		FROM cards c JOIN users u ON u.id = c.author_id
		WHERE c.retro_id = ? ORDER BY c.created_at ASC`, retroID)
	if err != nil {
		return nil, err
	}
	defer cardRows.Close()

	data.Cards = map[int64][]exportCard{}
	for cardRows.Next() {
		var id, colID int64
		var content, fn, ln string
		cardRows.Scan(&id, &colID, &content, &fn, &ln)
		data.Cards[colID] = append(data.Cards[colID], exportCard{
			Content: content, FirstName: fn, LastName: ln, Votes: votesByCard[id],
		})
	}

	if lastColID > 0 {
		aiRows, _ := db.Query(`
			SELECT ai.content,
			       COALESCE(u.first_name, ''), COALESCE(u.last_name, ''),
			       COALESCE(ai.deadline, '')
			FROM action_items ai
			LEFT JOIN users u ON u.id = ai.assignee_id
			WHERE ai.retro_id = ? AND ai.column_id = ?
			ORDER BY ai.created_at ASC`, retroID, lastColID)
		if aiRows != nil {
			defer aiRows.Close()
			for aiRows.Next() {
				var content, fn, ln, deadlineStr string
				aiRows.Scan(&content, &fn, &ln, &deadlineStr)
				dl, _ := time.Parse("2006-01-02", deadlineStr)
				data.ActionItems = append(data.ActionItems, exportAI{
					Content: content, FirstName: fn, LastName: ln, Deadline: dl,
				})
			}
		}
	}

	return &data, nil
}

func votesLabel(n int) string {
	switch {
	case n == 1:
		return "1 голос"
	case n >= 2 && n <= 4:
		return fmt.Sprintf("%d голоси", n)
	default:
		return fmt.Sprintf("%d голосів", n)
	}
}

func HandleRetrosExportMD(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		data, err := loadExportData(db, retroID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s — %s\n",
			data.Retro.Date.Format("02.01.2006 о 15:04"),
			data.Retro.TeamName,
		))

		for _, col := range data.RegularCols {
			sb.WriteString(fmt.Sprintf("\n## %s\n", col.Title))
			for _, c := range data.Cards[col.ID] {
				sb.WriteString(fmt.Sprintf("- %s @%s %s (%s)\n",
					c.Content, c.FirstName, c.LastName, votesLabel(c.Votes),
				))
			}
			if len(data.Cards[col.ID]) == 0 {
				sb.WriteString("_немає карток_\n")
			}
		}

		if len(data.ActionItems) > 0 {
			sb.WriteString("\n## Action Items\n")
			for _, ai := range data.ActionItems {
				sb.WriteString(fmt.Sprintf("- [ ] %s @%s %s (до %s)\n",
					ai.Content, ai.FirstName, ai.LastName,
					ai.Deadline.Format("02.01.2006"),
				))
			}
		}

		filename := fmt.Sprintf("retro-%s.md", data.Retro.Date.Format("2006-01-02"))
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		fmt.Fprint(w, sb.String())
	}
}

func HandleRetrosExportCSV(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		retroID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		data, err := loadExportData(db, retroID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("retro-%s.csv", data.Retro.Date.Format("2006-01-02"))
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Write([]byte("\xef\xbb\xbf")) // BOM for Excel UTF-8

		cw := csv.NewWriter(w)
		cw.Write([]string{"Колонка", "Текст", "Автор", "Голоси / Дедлайн"})

		for _, col := range data.RegularCols {
			for _, c := range data.Cards[col.ID] {
				cw.Write([]string{
					col.Title,
					c.Content,
					c.FirstName + " " + c.LastName,
					strconv.Itoa(c.Votes),
				})
			}
		}

		for _, ai := range data.ActionItems {
			cw.Write([]string{
				"Action Items",
				ai.Content,
				ai.FirstName + " " + ai.LastName,
				ai.Deadline.Format("02.01.2006"),
			})
		}

		cw.Flush()
	}
}
