package i18n_test

import (
	"testing"

	"github.com/postup-app/postup/internal/i18n"
)

// Тест 7 — відомий ключ, мова uk
func TestT_UkNavRetros(t *testing.T) {
	got := i18n.T("uk", "nav.retros")
	if got != "Ретроспективи" {
		t.Errorf("expected 'Ретроспективи', got %q", got)
	}
}

// Тест 8 — відомий ключ, мова en
func TestT_EnNavRetros(t *testing.T) {
	got := i18n.T("en", "nav.retros")
	if got != "Retrospectives" {
		t.Errorf("expected 'Retrospectives', got %q", got)
	}
}

// Тест 9 — відомий lang, невідомий ключ → повертає key як fallback
func TestT_MissingKey(t *testing.T) {
	got := i18n.T("uk", "nonexistent.key")
	if got != "nonexistent.key" {
		t.Errorf("expected fallback key 'nonexistent.key', got %q", got)
	}
}

// Тест 10 — невідома мова → повертає key як fallback
func TestT_UnknownLang(t *testing.T) {
	got := i18n.T("xx", "nav.retros")
	if got != "nav.retros" {
		t.Errorf("expected fallback key 'nav.retros' for unknown lang, got %q", got)
	}
}
