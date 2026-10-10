package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/store"
	tea "github.com/charmbracelet/bubbletea"
)

var realGitCachePath = gitCachePath

func unusableNoteStore() *store.NoteStore {
	return store.New("$DIGEST_TEST_UNSET_ROOT_VARIABLE/notes")
}

func TestHelpFromTheSearchPreviewListsItsKeys(t *testing.T) {
	m := typeQuery(t, searchTestModel(t), "flaky")
	m = press(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = press(t, m, runes("?"))
	entries := m.helpEntries()
	if m.mode != ViewHelp || len(entries) == 0 || entries[0].section != "PREVIEW" || !strings.Contains(fmt.Sprint(entries), "edit") {
		t.Fatalf("entries = %+v", entries)
	}
	lines := helpSectionLines([]helpEntry{{section: "A", key: "a", text: "one"}, {section: "B", key: "b", text: "two"}}, 40)
	if !strings.Contains(strings.Join(lines, "\n"), "\n\n") {
		t.Fatalf("sections should be separated by a blank line: %q", lines)
	}
}

func TestActivityOwnerPrefersTheExplicitOwner(t *testing.T) {
	if activityOwner(review.ActivityPR{Owner: "explicit", URL: "https://github.com/other/r/pull/1"}) != "explicit" {
		t.Fatal("an explicit owner wins")
	}
}

func TestPRNoteFinderReportsAnUnusableStore(t *testing.T) {
	finder := newPRNoteFinder(unusableNoteStore(), "", prNoteIndex{})
	if _, _, err := finder.find("o/r#1", "r", 1, "r:1"); err == nil {
		t.Fatal("an unusable store should fail the lookup")
	}
}

func TestBlankMarkdownShowsThePlaceholder(t *testing.T) {
	if got := stripANSI(renderMarkdown("  \n", 40)); got != "(No note body text)" {
		t.Fatalf("rendered = %q", got)
	}
}

func TestGitCacheDefaultsUnderTheDigestRoot(t *testing.T) {
	if got := realGitCachePath(); got != filepath.Join(digestRoot, "cache", "git-sync.json") {
		t.Fatalf("path = %q", got)
	}
}

func TestGitCacheSkipsOtherDaysAndKeepsTheChosenSort(t *testing.T) {
	m := syncTestModel(t)
	m.git.loadingGit, m.git.loadingCommits = false, false
	m.currentDate = time.Now().AddDate(0, 0, -3)
	if m.gitCacheSaveCmd() != nil {
		t.Fatal("only today is cached")
	}
	m.git.pendingSortChosen = true
	if cache := m.gitCacheSnapshot(); cache.PendingSort == nil {
		t.Fatal("a chosen sort should be cached")
	}
	m.git.prDetails = nil
	m.mergePRDetails(map[string]json.RawMessage{"u": json.RawMessage(`{}`)})
	if len(m.git.prDetails) != 1 {
		t.Fatalf("details = %v", m.git.prDetails)
	}
}

func TestPRAlertsSkipUnknownStatesButAlertNewOwnPRs(t *testing.T) {
	if _, alertable := prAlert("u", prStatus{State: "SOMETHING_ELSE"}); alertable {
		t.Fatal("an unknown state is not alertable")
	}
	alerts := changedPRAlerts(nil, map[string]prStatus{"u": {Kind: prKindMine, State: prStateApproved, Title: "mine"}})
	if len(alerts) != 1 || alerts[0].Title != "PR approved" {
		t.Fatalf("a newly seen own PR should alert: %+v", alerts)
	}
	if err := sendPRAlerts(t.TempDir(), nil); err == nil {
		t.Fatal("an unreadable alerts file should fail")
	}
}

func TestNoteLinkHelpersWithoutLinks(t *testing.T) {
	if noteLinks(nil) != nil {
		t.Fatal("no note has no links")
	}
	m := noteSaveModel(t, &model.Note{Summary: "plain", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now(), Updated: time.Now()})
	selectSummary(t, &m, "plain")
	if next, cmd := m.openNoteLinks(tea.KeyMsg{}); cmd != nil || next.(Model).mode != m.mode {
		t.Fatal("a note without links opens nothing")
	}
	m.selected = 99
	if next, cmd := m.openNoteLinks(tea.KeyMsg{}); cmd != nil || next.(Model).mode != m.mode {
		t.Fatal("no row opens nothing")
	}
}

func TestBindingsForAnUnknownModeAndAnEmptyPreview(t *testing.T) {
	m := syncTestModel(t)
	m.mode = ViewMode(9999)
	if m.activeBindings() != nil {
		t.Fatal("an unknown mode has no bindings")
	}
	m.selected = 99
	if len(m.previewBindings()) == 0 {
		t.Fatal("an empty preview still has its tail bindings")
	}
}

func TestFooterWordHelpers(t *testing.T) {
	if capitaliseWord("") != "" {
		t.Fatal("an empty word stays empty")
	}
	if first, second := splitFooterAction("  "); first != "" || second != "" {
		t.Fatalf("split = %q %q", first, second)
	}
}

func TestAppStateRebuildReportsAnUnusableStore(t *testing.T) {
	m := syncTestModel(t)
	m.appStateKnown = false
	m.store = unusableNoteStore()
	msg, ok := m.rebuildAppStateCmd()().(appStateRebuiltMsg)
	if !ok || msg.err == nil {
		t.Fatalf("msg = %#v", msg)
	}
	next, _ := m.applyRebuiltAppState(msg)
	if latestMessageText(next.(Model)) == "" {
		t.Fatal("the rebuild error should be shown")
	}
}

func TestSettingsKeyWithoutAConfigPathUsesTheDefaultConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	m := syncTestModel(t)
	m.configPath = ""
	next, _ := m.openSetup(tea.KeyMsg{})
	opened := next.(Model)
	if opened.mode != ViewSetup || opened.setup == nil || !opened.setup.form || opened.setup.configPath != config.Path("") || opened.setup.configPath == "" {
		t.Fatalf("',' without a config path should open settings on the default config, mode %v", opened.mode)
	}
}

func TestInactiveBannerWaveIgnoresTicks(t *testing.T) {
	m := syncTestModel(t)
	m.bannerWaveActive = false
	frame := m.bannerWaveFrame
	if next, _ := m.Update(bannerWaveTickMsg{}); next.(Model).bannerWaveFrame != frame {
		t.Fatal("an inactive wave should not advance")
	}
}

func TestEditorViewWithoutACache(t *testing.T) {
	m := syncTestModel(t)
	m.editorCache = nil
	if m.editorView() == "" {
		t.Fatal("the editor should render without a cache")
	}
}

func TestSyncPulseTickProducesItsMessage(t *testing.T) {
	if _, ok := tickSyncPulseCmd()().(syncPulseTickMsg); !ok {
		t.Fatal("expected a sync pulse tick")
	}
}

func TestRefreshCommitsOutsideASyncPostsProgress(t *testing.T) {
	m := syncTestModel(t)
	stubDayCommits(t)
	m.messages = nil
	m.git.loadingGit = false
	if m.refreshCommitsCmd() == nil || !m.gitSyncInProgress() {
		t.Fatal("loading commits outside a sync should post progress")
	}
}

func TestAutoSyncTickMessageWithoutAnIntervalIsHandled(t *testing.T) {
	m := syncTestModel(t)
	m.cfg.GitAutoSyncInterval = 0
	if _, cmd, handled := (gitSection{}).ApplyMessage(m, autoSyncTickMsg(time.Now())); !handled || cmd != nil {
		t.Fatal("the tick should be handled without a sync")
	}
}

func TestChangedNotesJoinTheBrowsedList(t *testing.T) {
	m := syncTestModel(t)
	m.browsedNotes = []*model.Note{}
	notesSection{}.mergeNotes(&m, []model.Note{{ID: "new", Summary: "new", Status: model.StatusArchived}})
	if len(m.browsedNotes) != 1 {
		t.Fatalf("browsed = %d", len(m.browsedNotes))
	}
}

func TestCommitsLoadedOutsideASyncSaveNothing(t *testing.T) {
	m := syncTestModel(t)
	m.git.loadingGit, m.git.loadingCommits = false, false
	if _, cmd, handled := (gitSection{}).ApplyMessage(m, commitsLoadedMsg{generation: m.git.commitsGeneration}); !handled || cmd != nil {
		t.Fatal("commits outside a sync should not save the cache")
	}
}

func TestFailedHostPRsAreNotDuplicatedWhenStillListed(t *testing.T) {
	previous := []review.QueuedPR{myPR(4, nil), myPR(5, nil)}
	kept := keepFailedHostPRs(previous, []review.QueuedPR{myPR(4, nil)}, []string{"github.com"})
	if len(kept) != 2 {
		t.Fatalf("kept = %+v", kept)
	}
}

func TestAFailedSaveRestoresTheBrowsedCopyFromDisk(t *testing.T) {
	m := syncTestModel(t)
	browsed := &model.Note{ID: "b", FilePath: "/notes/b.md", Summary: "typed"}
	m.browsedNotes = []*model.Note{browsed}
	onDisk := model.Note{ID: "b", FilePath: "/notes/b.md", Summary: "typed", Status: model.StatusDone}
	next, _ := m.applySavedNotes(notesSavedMsg{failed: []failedNoteSave{{target: browsed, attempted: *browsed, onDisk: &onDisk, err: errors.New("locked")}}})
	if got := next.(Model); got.browsedNotes[0].Status != model.StatusDone {
		t.Fatalf("browsed = %+v", got.browsedNotes[0])
	}
}
