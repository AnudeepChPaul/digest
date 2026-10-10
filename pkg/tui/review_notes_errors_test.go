package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
	"github.com/achandrapaul/digest/pkg/store"
	"golang.org/x/sys/unix"
)

func readOnlyNoteStore(t *testing.T) *store.NoteStore {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o700) })
	return store.New(root)
}

func TestReviewNotesStopOnASaveFailure(t *testing.T) {
	msg, ok := reviewNotesCmd(readOnlyNoteStore(t), []review.ActivityPR{changesRecord("", time.Now())})().(notesChangedMsg)
	if !ok || msg.err == nil || errors.Is(msg.err, store.ErrLockNote) || len(msg.notes) != 0 {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestReviewNotesKeepGoingWhenLockingFails(t *testing.T) {
	m := lockedNoteModel(t)
	failNoteFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE != 0 })
	msg, ok := reviewNotesCmd(m.store, []review.ActivityPR{changesRecord("", time.Now())})().(notesChangedMsg)
	if !ok || !errors.Is(msg.err, store.ErrLockNote) || len(msg.notes) != 1 {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestReviewNotesReportAKnownNoteWhoseFileIsGone(t *testing.T) {
	noteStore := store.New(t.TempDir())
	ref := prNoteRef("", "console", 7)
	gone := &model.Note{Ref: ref, Summary: "Reviewed: Commented:console:7 Fix", Source: model.SourcePRReview, Status: model.StatusActive, Updated: time.Now().Add(-time.Hour)}
	if err := noteStore.Save(gone); err != nil {
		t.Fatal(err)
	}
	if err := noteStore.Delete(gone); err != nil {
		t.Fatal(err)
	}
	known := indexPRNotes([]*model.Note{gone}, "")
	msg, ok := knownReviewNotesCmd(noteStore, known, []review.ActivityPR{changesRecord("", time.Now())})().(notesChangedMsg)
	if !ok || msg.err == nil || len(msg.notes) != 0 {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestReviewNotesGiveALegacyNoteItsRef(t *testing.T) {
	noteStore := store.New(t.TempDir())
	legacy := &model.Note{ID: reviewNoteID("console", 7), Summary: "Reviewed: Commented:console:7 Fix", Source: model.SourcePRReview, Status: model.StatusActive, Updated: time.Now().Add(-time.Hour)}
	if err := noteStore.Save(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(legacy.FilePath, filepath.Join(filepath.Dir(legacy.FilePath), legacy.ID+".md")); err != nil {
		t.Fatal(err)
	}
	msg, ok := reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("", time.Now())})().(notesChangedMsg)
	if !ok || msg.err != nil || len(msg.notes) != 1 || msg.notes[0].Ref == "" {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestRepeatedReviewAddsOnlyNewComments(t *testing.T) {
	noteStore := store.New(t.TempDir())
	reviewedAt := time.Now()
	reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("", reviewedAt)})()
	if msg := reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("", reviewedAt.Add(10*time.Second))})(); msg != nil {
		t.Fatalf("a repeat without comments should change nothing, got %#v", msg)
	}
	msg, ok := reviewNotesCmd(noteStore, []review.ActivityPR{changesRecord("Rename it", reviewedAt.Add(20*time.Second))})().(notesChangedMsg)
	if !ok || len(msg.notes) != 1 || !strings.Contains(msg.notes[0].Body, "Rename it") {
		t.Fatalf("msg = %#v", msg)
	}
	if strings.Count(msg.notes[0].Body, requestedChangesHeading) != 1 {
		t.Fatalf("one comments block expected:\n%s", msg.notes[0].Body)
	}
}

func TestIsRepeatOfLatestNeedsTheSameSummary(t *testing.T) {
	note := &model.Note{Summary: "a", Updated: time.Now()}
	if isRepeatOfLatest(note, "b", time.Now()) {
		t.Fatal("a different summary is not a repeat")
	}
}

func approvedNoteIn(t *testing.T, noteStore *store.NoteStore) {
	t.Helper()
	approvedAt := time.Now().Add(-48 * time.Hour)
	note := &model.Note{Ref: prNoteRef("", "console", 6), Created: approvedAt, Updated: approvedAt, Status: model.StatusDone, Source: model.SourcePRReview, Summary: "Approved:console:6 Fix"}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
}

func TestReopenApprovedNotesWithNothingPendingDoesNothing(t *testing.T) {
	if reopenApprovedNotesCmd(store.New(t.TempDir()), nil, time.Now()) != nil {
		t.Fatal("no pending PRs should give no command")
	}
}

func TestReopenApprovedNotesStopsOnASaveFailure(t *testing.T) {
	root := t.TempDir()
	noteStore := store.New(root)
	approvedNoteIn(t, noteStore)
	notes, _ := noteStore.List()
	monthDir := filepath.Dir(notes[0].FilePath)
	if err := os.Chmod(monthDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(monthDir, 0o700) })
	msg, ok := reopenApprovedNotesCmd(noteStore, []GitPRItem{{Number: 6, Repository: "console"}}, time.Now())().(notesChangedMsg)
	if !ok || msg.err == nil || errors.Is(msg.err, store.ErrLockNote) {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestReopenApprovedNotesKeepGoingWhenLockingFails(t *testing.T) {
	m := lockedNoteModel(t)
	approvedNoteIn(t, m.store)
	failNoteFlagChanges(t, func(flags int) bool { return flags&unix.UF_IMMUTABLE != 0 })
	pending := []GitPRItem{{Number: 6, Repository: "console", PR: &review.QueuedPR{}}}
	msg, ok := reopenApprovedNotesCmd(m.store, pending, time.Now())().(notesChangedMsg)
	if !ok || !errors.Is(msg.err, store.ErrLockNote) || len(msg.notes) != 1 || msg.notes[0].Status != model.StatusActive {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestOpenCloneCommandReportsACloneInProgress(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-test,1,0")
	m := reviewTestModel(t)
	writeLivePID(t, filepath.Join(reviewStateDir(m), "review.pid"))
	m.sessionCtx = nil
	_, _, cmd := m.openClone(m.currentPRItem())
	ready, ok := cmd().(reviewCloneReadyMsg)
	if !ok || !errors.Is(ready.err, sourcecontrol.ErrCloneInProgress) {
		t.Fatalf("msg = %+v", ready)
	}
}

func TestReviewConfirmCountsSelectedComments(t *testing.T) {
	m := reviewTestModel(t)
	m.reviewEvent = review.EventRequestChanges
	m.reviewSelected = map[int]bool{0: true, 1: true}
	text := stripANSI(m.renderReviewConfirm(80))
	if !strings.Contains(text, "CONFIRM REQUEST CHANGES") || !strings.Contains(text, "Includes 2 selected comment(s).") {
		t.Fatalf("confirm:\n%s", text)
	}
}

func selectNavKind(t *testing.T, m *Model, kind NavItemKind) {
	t.Helper()
	for index, item := range m.allNavItems() {
		if item.Kind == kind {
			m.selected = index
			return
		}
	}
	t.Fatalf("no nav item of kind %v", kind)
}

func TestPreviewStampTracksRunLogs(t *testing.T) {
	m := reviewTestModel(t)
	ref := review.PRRef{Host: "github.com", Owner: "o", Repo: "api", Number: 9, URL: "https://github.com/o/api/pull/9"}
	m.reviewRuns = []review.ReviewRun{{Meta: review.Meta{Ref: ref}, Status: review.RunFailed}}
	writeReviewStateFile(t, review.StateDir(m.reviewRoot(), ref), review.LogFile, "boom")
	m.bragRuns = []brag.Run{{Meta: brag.RunMeta{ID: "week"}, Status: brag.RunFailed}}
	m.automationRuns = map[string]automation.Run{"note": {Meta: automation.RunMeta{NoteID: "note"}, Status: automation.RunFailed}}
	m.mode = ViewPreview
	for _, kind := range []NavItemKind{KindReviewRun, KindBragRun, KindAutomationRun} {
		selectNavKind(t, &m, kind)
		if stamp := m.previewStamp(); stamp.runStatus == 0 {
			t.Fatalf("kind %v stamp = %+v", kind, stamp)
		}
	}
	selectNavKind(t, &m, KindReviewRun)
	if stamp := m.previewStamp(); stamp.logSize != 4 {
		t.Fatalf("review run stamp should track the log size, got %+v", stamp)
	}
}

func TestPreviewStampIsEmptyForNoteRows(t *testing.T) {
	m := noteSaveModel(t, dashboardNotes(time.Now())...)
	m.mode = ViewPreview
	selectNavKind(t, &m, KindCarriedNote)
	if m.previewStamp() != (previewStamp{}) {
		t.Fatal("a note row has no preview stamp")
	}
}
