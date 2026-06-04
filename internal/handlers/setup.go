package handlers

import "net/http"

func HandleSetup(re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		re.Render(w, r, "setup.html", nil)
	}
}
