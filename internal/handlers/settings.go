package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/postup-app/postup/internal/middleware"
)

func HandleSettingsShow(db *sql.DB, re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		port := settingGet(db, "port")
		if port == "" {
			port = "8080"
		}
		retroDuration := settingGet(db, "retro_duration")
		if retroDuration == "" {
			retroDuration = "2"
		}
		baseURL := settingGet(db, "base_url")

		saved := r.URL.Query().Get("saved") == "1"
		restart := r.URL.Query().Get("restart") == "1"

		re.Render(w, r, "settings.html", map[string]any{
			"Port":          port,
			"RetroDuration": retroDuration,
			"BaseURL":       baseURL,
			"BaseURLAuto":   baseURL == "",
			"Saved":         saved,
			"Restart":       restart,
		})
	}
}

func HandleSettingsUpdate(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		section := r.FormValue("section")

		switch section {
		case "system":
			user := middleware.GetUser(r)
			if user == nil || !user.IsAdmin() {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			port := strings.TrimSpace(r.FormValue("port"))
			if port == "" {
				port = "8080"
			}
			retroDuration := strings.TrimSpace(r.FormValue("retro_duration"))
			if retroDuration == "" {
				retroDuration = "2"
			}
			baseURL := ""
			if r.FormValue("base_url_auto") != "1" {
				baseURL = strings.TrimSpace(r.FormValue("base_url"))
			}
			currentPort := settingGet(db, "port")
			settingSet(db, "port", port)
			settingSet(db, "retro_duration", retroDuration)
			settingSet(db, "base_url", baseURL)
			if port != currentPort {
				http.Redirect(w, r, "/settings?saved=1&restart=1", http.StatusSeeOther)
				return
			}

		case "personal":
			lang := r.FormValue("lang")
			theme := r.FormValue("theme")

			validLangs := map[string]bool{"uk": true, "en": true}
			if !validLangs[lang] {
				lang = "uk"
			}
			if theme != "light" && theme != "dark" {
				theme = "dark"
			}

			year := int(time.Hour * 24 * 365)
			http.SetCookie(w, &http.Cookie{
				Name:     "lang",
				Value:    lang,
				Path:     "/",
				MaxAge:   year,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			http.SetCookie(w, &http.Cookie{
				Name:     "theme",
				Value:    theme,
				Path:     "/",
				MaxAge:   year,
				HttpOnly: false,
				SameSite: http.SameSiteLaxMode,
			})
		}

		http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
	}
}

func settingGet(db *sql.DB, key string) string {
	var value string
	db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	return value
}

func settingSet(db *sql.DB, key, value string) {
	db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
}

func resolveBaseURL(db *sql.DB, port string, getIP func() string) string {
	if baseURL := settingGet(db, "base_url"); baseURL != "" {
		return baseURL
	}
	return fmt.Sprintf("http://%s:%s", getIP(), port)
}
