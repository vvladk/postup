package handlers

import (
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/postup-app/postup/internal/i18n"
	"github.com/postup-app/postup/internal/middleware"
)

type Renderer struct {
	fs fs.FS
}

func NewRenderer(files fs.FS) *Renderer {
	return &Renderer{fs: files}
}

func getLang(r *http.Request) string {
	c, err := r.Cookie("lang")
	if err != nil || c.Value == "" {
		return "uk"
	}
	for _, l := range i18n.Available() {
		if l == c.Value {
			return c.Value
		}
	}
	return "uk"
}

func getTheme(r *http.Request) string {
	c, err := r.Cookie("theme")
	if err != nil || (c.Value != "light" && c.Value != "dark") {
		return "dark"
	}
	return c.Value
}

func (re *Renderer) Render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	lang := getLang(r)
	theme := getTheme(r)

	if data == nil {
		data = map[string]any{}
	}
	data["CurrentUser"] = middleware.GetUser(r)
	data["CurrentLang"] = lang
	data["CurrentTheme"] = theme

	funcMap := template.FuncMap{
		"t": func(key string) string { return i18n.T(lang, key) },
		"dict": func(values ...any) map[string]any {
			m := make(map[string]any, len(values)/2)
			for i := 0; i+1 < len(values); i += 2 {
				if key, ok := values[i].(string); ok {
					m[key] = values[i+1]
				}
			}
			return m
		},
	}

	files := []string{"templates/layout.html", "templates/pages/" + page}
	if partials, _ := fs.Glob(re.fs, "templates/partials/*.html"); len(partials) > 0 {
		files = append(files, partials...)
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(re.fs, files...)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("parse template %s: %v", page, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

// RenderPage renders a standalone page template without the layout wrapper.
func (re *Renderer) RenderPage(w http.ResponseWriter, page string, data any) {
	tmpl, err := template.ParseFS(re.fs, "templates/pages/"+page)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		log.Printf("parse page %s: %v", page, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render page %s: %v", page, err)
	}
}
