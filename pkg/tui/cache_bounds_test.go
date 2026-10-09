package tui

import "testing"

func TestMarkdownRendererCacheStaysSmall(t *testing.T) {
	for width := 40; width < 120; width++ {
		markdownRendererFor(width)
	}
	markdownRenderersMu.Lock()
	cached := len(markdownRenderers)
	markdownRenderersMu.Unlock()
	if cached > maxMarkdownRenderers {
		t.Errorf("%d renderers cached after a resize drag", cached)
	}
	current := markdownRendererFor(80)
	if markdownRendererFor(80) != current {
		t.Error("the current width should stay cached")
	}
}

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
