package ipdetect

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "1.2.3.4")
	}))
	defer srv.Close()

	old := apiURL
	apiURL = srv.URL
	defer func() { apiURL = old }()

	ip, err := Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Errorf("want %q, got %q", "1.2.3.4", ip)
	}
}

func TestDetectWithFallback_unavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	old := apiURL
	apiURL = srv.URL
	defer func() { apiURL = old }()

	ip := DetectWithFallback("")
	if ip != "127.0.0.1" {
		t.Errorf("want %q, got %q", "127.0.0.1", ip)
	}
}
