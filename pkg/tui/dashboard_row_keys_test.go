package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
	"github.com/achandrapaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

func manyTodayNotesModel(t *testing.T, count int) Model {
	t.Helper()
	m := selectionTestModel(t)
	for index := range count {
		m.notes = append(m.notes, &model.Note{ID: fmt.Sprintf("n%02d", index), Summary: fmt.Sprintf("note %02d", index), Status: model.StatusActive, Source: model.SourceManual, Created: m.currentDate, Updated: m.currentDate})
	}
	m.contentVersion++
	return m
}

func TestDOnANoteAsksToArchiveIt(t *testing.T) {
	m := noteSelectedModel(t)
	m.notes[0].FilePath = "/notes/n1.md"
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm || len(m.deleteTargetNotes) != 1 || m.deleteTargetNotes[0].ID != "n1" || m.deleteReturnMode != ViewDashboard {
		t.Errorf("mode=%v targets=%v return=%v", m.mode, m.deleteTargetNotes, m.deleteReturnMode)
	}
}

func TestDOnAnUnsavedNoteAsksThenDropsIt(t *testing.T) {
	m := noteSelectedModel(t)
	m = press(t, m, runes("d"))
	if m.mode != ViewDeleteConfirm || len(m.deleteTargetNotes) != 1 || !strings.Contains(stripANSI(m.View()), "This note doesn't exist. Delete?") {
		t.Fatalf("mode=%v targets=%v", m.mode, m.deleteTargetNotes)
	}
	if cancelled := press(t, m, runes("n")); cancelled.mode != ViewDashboard || len(cancelled.notes) != 1 {
		t.Errorf("n should keep the note: mode=%v notes=%d", cancelled.mode, len(cancelled.notes))
	}
	next, cmd := m.Update(runes("y"))
	m = next.(Model)
	if m.mode != ViewDashboard || len(m.notes) != 0 || cmd != nil {
		t.Errorf("y should drop the note without saving: mode=%v notes=%d cmd=%v", m.mode, len(m.notes), cmd != nil)
	}
}

func TestClearingANoteInlineAsksToDeleteIt(t *testing.T) {
	m := noteSaveModel(t, &model.Note{ID: "n1", Summary: "Keep me", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now(), Updated: time.Now()})
	m = applyMsgs(t, m, func() tea.Msg { return m.loadNotesCmd() })
	selectNote(t, &m, "n1")
	clearInline := func(m Model) Model {
		m = press(t, m, runes("i"))
		m.inlineInput.SetValue("  ")
		return press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	}
	asked := clearInline(m)
	if asked.mode != ViewDeleteConfirm || !strings.Contains(stripANSI(asked.View()), "Delete this note?") {
		t.Fatalf("mode=%v", asked.mode)
	}
	kept := press(t, asked, runes("n"))
	if kept.mode != ViewDashboard || noteBySummary(t, kept, "Keep me").Summary != "Keep me" {
		t.Errorf("n should restore the text, mode=%v", kept.mode)
	}
	next, cmd := asked.Update(runes("y"))
	archived, _ := applyNoteMessages(t, next.(Model), cmd)
	if status, found := reloadedStatus(t, archived, "Keep me"); !found || status != model.StatusArchived {
		t.Errorf("y should archive the note like d: status=%v found=%v", status, found)
	}
}

func TestTJumpsBackToTodayAndResetsTheCursor(t *testing.T) {
	m := manyTodayNotesModel(t, 3)
	m = press(t, m, runes("p"))
	m = press(t, m, runes("p"))
	m.selected, m.scrollOffset = 2, 4
	next, cmd := m.Update(runes("t"))
	m = next.(Model)
	if !isSameDay(m.currentDate, time.Now()) || m.selected != 0 || m.scrollOffset != 0 {
		t.Errorf("date=%v selected=%d scroll=%d", m.currentDate, m.selected, m.scrollOffset)
	}
	if cmd == nil {
		t.Error("t should schedule a day sync and reload notes")
	}
}

func TestHalfPageKeysMoveHalfTheBodyAndClamp(t *testing.T) {
	m := manyTodayNotesModel(t, 60)
	step := max(m.dashboardBodyHeight(), 5) / 2
	if step < 1 {
		t.Fatalf("body height too small: %d", m.dashboardBodyHeight())
	}
	lastIndex := len(m.allNavItems()) - 1
	for _, downKey := range []tea.KeyMsg{{Type: tea.KeyCtrlD}} {
		m.selected = 0
		if moved := press(t, m, downKey); moved.selected != step {
			t.Errorf("%s from 0: selected=%d, want %d", downKey, moved.selected, step)
		}
		m.selected = lastIndex - 1
		if moved := press(t, m, downKey); moved.selected != lastIndex {
			t.Errorf("%s near the end: selected=%d, want %d", downKey, moved.selected, lastIndex)
		}
	}
	for _, upKey := range []tea.KeyMsg{{Type: tea.KeyCtrlU}} {
		m.selected = step + 1
		if moved := press(t, m, upKey); moved.selected != 1 {
			t.Errorf("%s from %d: selected=%d, want 1", upKey, step+1, moved.selected)
		}
		m.selected = 1
		if moved := press(t, m, upKey); moved.selected != 0 {
			t.Errorf("%s near the top: selected=%d, want 0", upKey, moved.selected)
		}
	}
}

func TestEnterOnAPendingPROpensItInTheBrowser(t *testing.T) {
	opened := stubOpenURL(t)
	m := selectionTestModel(t)
	pending := pendingItem(1)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pending}})
	selectNavItem(t, &m, "pr:"+pending.URL)
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewDashboard || !slices.Equal(*opened, []string{pending.URL}) {
		t.Errorf("mode=%v opened=%v", m.mode, *opened)
	}
}

func TestEnterAndTabOnAGitRepoOpenItsDetails(t *testing.T) {
	opened := stubOpenURL(t)
	m := gitStripTestModel(t)
	selectNavItem(t, &m, "repo:beta")
	m.git.gitPopupTab, m.git.gitPopupSelected = 2, 3
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyTab}} {
		opened := press(t, m, key)
		if opened.mode != ViewGitDetails || opened.git.gitPopupRepo == nil || opened.git.gitPopupRepo.Name != "beta" {
			t.Fatalf("%s on repo: mode=%v repo=%v", key, opened.mode, opened.git.gitPopupRepo)
		}
		if opened.git.gitPopupTab != 0 || opened.git.gitPopupSelected != 0 {
			t.Errorf("%s: git details should open on the first filter and row: tab=%d selected=%d", key, opened.git.gitPopupTab, opened.git.gitPopupSelected)
		}
	}
	if len(*opened) != 0 {
		t.Errorf("repo rows open nothing in the browser: %v", *opened)
	}
}

func TestNextDayStopsOneDayPastToday(t *testing.T) {
	m := manyTodayNotesModel(t, 1)
	tomorrow := time.Now().AddDate(0, 0, 1)
	for range 3 {
		m = press(t, m, runes("n"))
	}
	if !isSameDay(m.currentDate, tomorrow) {
		t.Errorf("n should stop at tomorrow, got %v", m.currentDate)
	}
	m = press(t, m, runes("p"))
	m = press(t, m, runes("p"))
	m = press(t, m, runes("n"))
	if !isSameDay(m.currentDate, time.Now()) {
		t.Errorf("n from yesterday should land on today, got %v", m.currentDate)
	}
}

func TestROnAPendingPRStartsTheReviewWithKeyHintsOff(t *testing.T) {
	m := prRowModel(t, false)
	if m, _ = pressKey(t, m, "r"); m.mode != ViewReviewRunConfirm {
		t.Errorf("r on a PR row with hints off: mode %v", m.mode)
	}
	repo := gitStripTestModel(t)
	selectNavItem(t, &repo, "repo:beta")
	if binding, found := repo.resolveKey(runes("r")); found {
		t.Errorf("r on a repo row is bound to %v", binding.action)
	}
}

func TestANewNoteIsDatedTheViewedDay(t *testing.T) {
	m := noteSaveModel(t)
	m = press(t, m, runes("p"))
	viewedDay := m.currentDate
	m = press(t, m, runes("a"))
	if m.mode != ViewEdit || m.currentNote == nil || !isSameDay(m.currentNote.Created, viewedDay) {
		t.Fatalf("mode=%v note=%+v viewed=%v", m.mode, m.currentNote, viewedDay)
	}
	m = typeText(t, m, "Dated note")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m, _ = applyNoteMessages(t, next.(Model), cmd)
	saved, err := store.New(m.cfg.NotesDir()).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0].Summary != "Dated note" || !isSameDay(saved[0].Created, viewedDay) {
		t.Errorf("saved notes = %+v, want one dated %v", saved, viewedDay)
	}
}

func TestWFlipsThePendingSortOrderAndSavesIt(t *testing.T) {
	m := syncTestModel(t)
	m.git.loadingGit = false
	m.git.pendingSort = sourcecontrol.Sort{}
	next, cmd := m.Update(runes("w"))
	m = next.(Model)
	if !m.git.pendingSort.Ascending {
		t.Fatal("w should switch the pending sort to ascending")
	}
	m = applyMsgs(t, m, cmd)
	cache, _ := loadGitCache()
	if cache.PendingSort == nil || !cache.PendingSort.Ascending || cache.PendingSort.ByCreated {
		t.Errorf("saved sort = %+v", cache.PendingSort)
	}
	next, cmd = m.Update(runes("w"))
	applyMsgs(t, next.(Model), cmd)
	if cache, _ = loadGitCache(); cache.PendingSort == nil || cache.PendingSort.Ascending {
		t.Errorf("second w should save descending, got %+v", cache.PendingSort)
	}
}

func TestSavingAnEmptyNewNoteShowsAnErrorInTheEditor(t *testing.T) {
	m := noteSaveModel(t)
	m = press(t, m, runes("a"))
	m.editor.SetValue("  \n ")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = next.(Model)
	if m.mode != ViewEdit || cmd != nil || len(m.notes) != 0 || m.awaitingNewNoteSave {
		t.Fatalf("mode=%v cmd=%v notes=%d", m.mode, cmd != nil, len(m.notes))
	}
	if !strings.Contains(stripANSI(m.View()), "note is empty") {
		t.Errorf("editor should say the note is empty:\n%s", stripANSI(m.View()))
	}
	m = typeText(t, m, "x")
	if strings.Contains(stripANSI(m.View()), "note is empty") {
		t.Errorf("typing should clear the notice")
	}
}

func TestSavingAnEmptiedExistingNoteShowsAnErrorAndKeepsTheNote(t *testing.T) {
	m := press(t, noteSelectedModel(t), tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ViewEdit {
		t.Fatalf("enter should open the editor, mode=%v", m.mode)
	}
	edited := m.currentNote
	m.editor.SetValue(" \n\t")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = next.(Model)
	if m.mode != ViewEdit || cmd != nil {
		t.Fatalf("an empty note should not be saved, mode=%v cmd=%v", m.mode, cmd != nil)
	}
	if edited.Summary != "Ship the trial banner" || edited.Body != "details" {
		t.Errorf("the note should keep its text, got %q / %q", edited.Summary, edited.Body)
	}
	if !strings.Contains(stripANSI(m.View()), "note is empty") {
		t.Errorf("editor should say the note is empty:\n%s", stripANSI(m.View()))
	}
}
