package tui

import "testing"

func TestRelatedHistoryKeepsOnlyListedPRs(t *testing.T) {
	m := reviewTestModel(t)
	listed := m.git.ghPendingPRs[0].URL
	m.contextCache["https://github.com/o/gone/pull/1"] = "old history"
	m = update(m, relatedHistoryMsg{url: listed, history: "fresh"})
	if _, kept := m.contextCache["https://github.com/o/gone/pull/1"]; kept {
		t.Error("history for a PR no longer listed should be dropped")
	}
	if m.contextCache[listed] != "fresh" {
		t.Errorf("listed PR history = %q", m.contextCache[listed])
	}
}
