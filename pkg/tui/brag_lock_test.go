//go:build darwin

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/system"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/unix"
)

func editBragWithFlagFailure(t *testing.T, failWhen func(flags int) bool) (Model, brag.Week) {
	t.Helper()
	m, _ := bragTestModel(t, wednesday())
	system.Protect(m.cfg.Root())
	t.Cleanup(func() {
		system.Protect("")
		_ = system.UnlockTree(m.cfg.Root())
	})
	week40 := brag.WeekOf(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	if err := (&brag.Brag{Period: week40, Facts: "- collected", Summary: "- Did things"}).Save(m.cfg.BragDir()); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, runes("b"))
	m = selectBragRow(t, m, "Week 40 · 28 Sep – 04 Oct")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	original := system.ChangeFileFlags
	system.ChangeFileFlags = func(path string, flags int) error {
		if failWhen(flags) {
			return unix.EPERM
		}
		return original(path, flags)
	}
	t.Cleanup(func() { system.ChangeFileFlags = original })
	m.editor.SetValue("## Facts\n\n- collected\n- typed point\n\n## Summary\n\n- Did things")
	return press(t, m, tea.KeyMsg{Type: tea.KeyCtrlO}), week40
}

func TestBragEditLockFailureSavesAndWarns(t *testing.T) {
	m, week40 := editBragWithFlagFailure(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE != 0 })
	if saved, err := brag.Load(m.cfg.BragDir(), week40); err != nil || !strings.Contains(saved.Facts, "- typed point") {
		t.Fatalf("brag should be saved: %+v %v", saved, err)
	}
	if m.mode != ViewBragView || m.bragNotice != "Saved but not locked" {
		t.Errorf("mode = %v notice = %q", m.mode, m.bragNotice)
	}
}

func TestBragEditUnlockFailureKeepsTheEditor(t *testing.T) {
	m, week40 := editBragWithFlagFailure(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE == 0 })
	if saved, _ := brag.Load(m.cfg.BragDir(), week40); saved == nil || strings.Contains(saved.Facts, "- typed point") {
		t.Fatalf("nothing should be written: %+v", saved)
	}
	if m.mode != ViewBragEdit || !strings.Contains(m.editor.Value(), "- typed point") || !strings.Contains(m.bragNotice, "unlock") {
		t.Errorf("editor should stay open with the text: mode %v notice %q", m.mode, m.bragNotice)
	}
}
