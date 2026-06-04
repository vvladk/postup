package handlers

import "net/http"

func HandleTimerShow(re *Renderer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		re.RenderPage(w, "timer.html", nil)
	}
}
