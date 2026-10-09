package tui

import (
	"strings"
	"testing"
	"time"
)

func helpText(entries []helpEntry, key string) (string, bool) {
	for _, entry := range entries {
		if entry.key == key {
			return entry.text, true
		}
	}
	return "", false
}

func TestDashboardShortcutsListOneActionPerRow(t *testing.T) {
	entries := syncTestModel(t).helpEntries()
	for key, want := range map[string]string{"p": "previous day", "n": "next day", "j|↓": "move down", "k|↑": "move up", "t": "today", "a": "new note"} {
		if text, found := helpText(entries, key); !found || text != want {
			t.Errorf("%s = %q (found %v), want %q", key, text, found, want)
		}
	}
	for _, entry := range entries {
		if entry.key == "p|n" || entry.key == "j|k" || entry.key == "ctrl+d|u" {
			t.Errorf("combined entry left: %+v", entry)
		}
	}
}

func TestDashboardShortcutsExplainRowKeysPerRowKind(t *testing.T) {
	entries := syncTestModel(t).helpEntries()
	for key, wants := range map[string][]string{
		"enter": {"note: edit", "PR: open in browser", "job: preview"},
		"space": {"note: mark done", "done note: mark active"},
		"d":     {"note: archive", "PR: reject", "job: dry run"},
		"r":     {"PR: review", "job: run job"},
	} {
		text, found := helpText(entries, key)
		for _, want := range wants {
			if !found || !strings.Contains(text, want) {
				t.Errorf("%s = %q, want it to mention %q", key, text, want)
			}
		}
	}
}

func TestQuestionMarkInAPreviewShowsThatPreviewsKeys(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	selectSummary(t, &m, "active")
	next, _ := m.openPreview(m.allNavItems()[m.selected])
	m = next.(Model)
	m = press(t, m, runes("?"))
	if m.mode != ViewHelp {
		t.Fatalf("? in a preview should open the shortcuts, mode = %v", m.mode)
	}
	view := stripANSI(m.View())
	for _, want := range []string{"next item", "previous item", "close"} {
		if !strings.Contains(view, want) {
			t.Errorf("preview shortcuts lack %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "previous day") {
		t.Errorf("preview shortcuts should not list dashboard keys:\n%s", view)
	}
	if m = press(t, m, runes("?")); m.mode != ViewPreview {
		t.Errorf("closing the shortcuts should return to the preview, mode = %v", m.mode)
	}
}
