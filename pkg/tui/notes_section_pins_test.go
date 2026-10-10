package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
)

func TestArchiveGroupsNotesByDayAndClampsTheSelection(t *testing.T) {
	m := openArchiveModel(t)
	now := time.Now()
	m.browsedNotes = append(m.browsedNotes, &model.Note{Summary: "archived last week", Status: model.StatusArchived, Source: model.SourceManual, Created: now.AddDate(0, 0, -9), Updated: now.AddDate(0, 0, -8)})
	m.archivedSelected = -3
	m.refreshArchivedViewport()
	if m.archivedSelected != 0 {
		t.Fatalf("selected = %d", m.archivedSelected)
	}
	content := stripANSI(m.renderArchivedContent(100, 0))
	if !strings.Contains(content, now.AddDate(0, 0, -8).Format("Monday 02 Jan 2006")) || !strings.Contains(content, "\n\n") {
		t.Fatalf("archive should group by day:\n%s", content)
	}
}

func TestArchiveOnATinyTerminalKeepsAMinimumHeight(t *testing.T) {
	m := noteSaveModel(t, archivedNotes(time.Now())...)
	m.height = 8
	next, _ := m.openArchive(tea.KeyMsg{})
	m = next.(Model)
	if m.archivedViewport.Height != 4 {
		t.Fatalf("height = %d", m.archivedViewport.Height)
	}
	if view := stripANSI(m.renderArchivedModal(80)); !strings.Contains(view, "ARCHIVED NOTES") {
		t.Fatalf("view:\n%s", view)
	}
}

func TestArchiveSelectionStartsAMapWhenMissing(t *testing.T) {
	m := openArchiveModel(t)
	m.archivedSelectedMap = nil
	next, _ := m.toggleArchiveSelection(tea.KeyMsg{})
	if !next.(Model).archivedSelectedMap[0] {
		t.Fatal("the first note should be selected")
	}
}

func TestBrowsedNotesRefreshTheSearchPreviewAndBragList(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky")
	m.showSearchPreviewAt(0)
	next, _ := m.Update(browsedNotesMsg{notes: nil})
	if got := next.(Model); got.browsedNotes == nil || len(got.browsedNotes) != 0 {
		t.Fatalf("browsed = %v", got.browsedNotes)
	}
	bragModel, _ := bragTestModel(t, wednesday())
	bragModel = press(t, bragModel, runes("b"))
	bragModel.bragSelected = 999
	next, _ = bragModel.Update(browsedNotesMsg{notes: []*model.Note{}})
	if got := next.(Model); got.bragSelected != len(got.bragRows())-1 {
		t.Fatalf("selected = %d of %d", got.bragSelected, len(got.bragRows()))
	}
	_ = brag.Week{}
}

func TestUnusableStoreResultsOnlyReportTheError(t *testing.T) {
	m := syncTestModel(t)
	failure := errors.New("disk gone")
	for _, msg := range []tea.Msg{notesReloadedMsg{err: failure}, closedThisWeekMsg{count: 5, err: failure}} {
		next, cmd := m.Update(msg)
		if cmd != nil || next.(Model).closedThisWeekElsewhere == 5 {
			t.Fatalf("%T should only report the error", msg)
		}
	}
	next, _ := m.Update(loadNotesMsg{err: failure})
	if len(next.(Model).notes) != len(m.notes) {
		t.Fatal("an unusable load should keep the notes")
	}
}

func TestBackgroundSaveErrorsShowAsMessages(t *testing.T) {
	m := syncTestModel(t)
	for _, testCase := range []struct {
		msg  tea.Msg
		want string
	}{
		{notifyEntriesMsg{err: errors.New("notify broke")}, "notify broke"},
		{appStateSavedMsg{err: errors.New("app state broke")}, "app state broke"},
	} {
		next, _ := m.Update(testCase.msg)
		if got := latestMessageText(next.(Model)); got != testCase.want {
			t.Fatalf("%T message = %q, want %q", testCase.msg, got, testCase.want)
		}
		archive := openArchiveModel(t)
		next, _ = archive.Update(testCase.msg)
		if got := next.(Model); got.mode != ViewArchived || !strings.Contains(stripANSI(got.View()), testCase.want) {
			t.Fatalf("%T should also show in the archive footer:\n%s", testCase.msg, stripANSI(got.View()))
		}
	}
}

func TestLoadedNotesReselectTheRequestedNoteAndRefreshOpenViews(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	target := m.notes[len(m.notes)-1]
	m.selectAfterReload = target.ID
	m.mode = ViewArchived
	next, _ := m.Update(loadNotesMsg{notes: m.notes})
	got := next.(Model)
	if item, _ := got.selectedNavItem(); item.Note != target || got.selectAfterReload != "" {
		t.Fatalf("selected %+v", item.Note)
	}
	search := typeQuery(t, searchTestModel(t), "flaky")
	search.showSearchPreviewAt(0)
	if next, _ := search.Update(loadNotesMsg{notes: search.notes}); next.(Model).mode != ViewPreview || !next.(Model).searchPreviewing {
		t.Fatal("the search preview should stay open")
	}
}

func TestChangedNotesRefreshOpenViewsAndMergeBrowsedCopies(t *testing.T) {
	m := openArchiveModel(t)
	browsed := m.browsedNotes[0]
	changed := *browsed
	changed.Summary = "renamed in the background"
	next, _ := m.Update(notesChangedMsg{notes: []model.Note{changed}})
	got := next.(Model)
	if got.browsedNotes[0].Summary != "renamed in the background" {
		t.Fatalf("browsed = %q", got.browsedNotes[0].Summary)
	}
	for _, note := range got.notes {
		if note.Status == model.StatusArchived {
			t.Fatal("archived notes never join the dashboard")
		}
	}
	search := typeQuery(t, searchTestModel(t), "flaky")
	search.showSearchPreviewAt(0)
	extra := model.Note{ID: "new", Summary: "flaky newcomer", Status: model.StatusActive, Source: model.SourceManual, Updated: time.Now()}
	next, _ = search.Update(notesChangedMsg{notes: []model.Note{extra}})
	if got := next.(Model); got.mode != ViewPreview || !got.searchPreviewing {
		t.Fatal("the search preview should stay open")
	}
}

func TestNoteActionsOnNonNoteRowsDoNothing(t *testing.T) {
	m := gitStripTestModel(t)
	selectNavKind(t, &m, KindGitRepo)
	for name, action := range map[string]func(tea.KeyMsg) (tea.Model, tea.Cmd){
		"inline": m.inlineEditSelected,
		"done":   m.toggleSelectedDone,
	} {
		if next, cmd := action(tea.KeyMsg{}); cmd != nil || next.(Model).mode != m.mode {
			t.Fatalf("%s should do nothing on a repo row", name)
		}
	}
	m.currentNote = nil
	next, cmd := m.saveInlineEdit(tea.KeyMsg{})
	if next.(Model).mode != ViewDashboard || cmd == nil {
		t.Fatal("saving without a note should reload the notes")
	}
}

func TestRowCachesAreClearedWhenFull(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	note := m.notes[0]
	for index := 0; index < maxCachedNoteRows; index++ {
		m.noteRows.tags[noteTagKey{created: time.Unix(int64(index), 0)}] = [2]string{}
		m.noteRows.rows[noteRowKey{width: -index - 1}] = ""
	}
	m.noteTagCells(note)
	m.renderRow(note, false, 80)
	if len(m.noteRows.tags) != 1 || len(m.noteRows.rows) != 1 {
		t.Fatalf("tags %d rows %d", len(m.noteRows.tags), len(m.noteRows.rows))
	}
	m.noteRows = nil
	if age, source := m.noteTagCells(note); source == "" && age == "" {
		t.Fatal("cells should render without a cache")
	}
}

func TestDayTitleNamesTheWeekdayForAnotherDay(t *testing.T) {
	m := syncTestModel(t)
	m.currentDate = time.Date(2026, 10, 5, 9, 0, 0, 0, time.Local)
	if got := m.dayTitleText(m.currentDate, "T O D A Y"); got != "M O N D A Y  ·  05 OCT" {
		t.Fatalf("title = %q", got)
	}
}

func TestPreviousDayIsYesterdayOnlyFromToday(t *testing.T) {
	m := syncTestModel(t)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	m.currentDate = now
	if got := m.previousDayTitleFor(yesterday); !strings.HasPrefix(got, "Y E S T E R D A Y  ·  ") {
		t.Errorf("from today, the day before = %q", got)
	}
	m.currentDate = yesterday
	dayBefore := now.AddDate(0, 0, -2)
	want := letterSpaced(strings.ToUpper(dayBefore.Format("Monday"))) + "  ·  " + strings.ToUpper(dayBefore.Format("02 Jan"))
	if got := m.previousDayTitleFor(dayBefore); got != want {
		t.Errorf("from yesterday, the day before = %q, want %q", got, want)
	}
	if got := m.dayTitleText(m.currentDate, "T O D A Y"); !strings.HasPrefix(got, letterSpaced(strings.ToUpper(yesterday.Format("Monday")))+"  ·  ") {
		t.Errorf("viewed past day = %q", got)
	}
}
