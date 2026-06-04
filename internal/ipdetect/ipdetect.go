package ipdetect

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

var apiURL = "https://api.ipify.org?format=text"

func Detect() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("detect ip: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

func DetectWithFallback(manual string) string {
	if manual != "" {
		return manual
	}

	ip, err := Detect()
	if err != nil {
		log.Printf("warning: could not detect external IP: %v; falling back to 127.0.0.1", err)
		return "127.0.0.1"
	}
	return ip
}
