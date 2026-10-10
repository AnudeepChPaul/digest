package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
)

func writeRawNote(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func skippedFiles(t *testing.T, err error) map[string]string {
	t.Helper()
	var skipped *SkippedNotesError
	if !errors.As(err, &skipped) {
		t.Fatalf("err = %v, want *SkippedNotesError", err)
	}
	reasons := map[string]string{}
	for _, note := range skipped.Notes {
		reasons[note.File] = note.Reason.Error()
	}
	return reasons
}

func corruptMonth(t *testing.T, root string) string {
	t.Helper()
	month := filepath.Join(root, "2026", "10")
	writeRawNote(t, filepath.Join(month, "no-front-matter.md"), "just text\n")
	writeRawNote(t, filepath.Join(month, "unterminated.md"), "---\nsummary: open\n")
	writeRawNote(t, filepath.Join(month, "bad-yaml.md"), "---\nsummary: [unclosed\n---\nbody\n")
	return month
}

func wantCorruptReasons(t *testing.T, err error) {
	t.Helper()
	reasons := skippedFiles(t, err)
	want := map[string]string{
		filepath.Join("2026", "10", "no-front-matter.md"): "front matter",
		filepath.Join("2026", "10", "unterminated.md"):    "front matter",
		filepath.Join("2026", "10", "bad-yaml.md"):        "yaml",
	}
	if len(reasons) != len(want) {
		t.Errorf("skipped = %v, want %v", reasons, want)
	}
	for file, reasonPart := range want {
		if !strings.Contains(reasons[file], reasonPart) {
			t.Errorf("reason for %s = %q, want it to mention %q", file, reasons[file], reasonPart)
		}
	}
	if !strings.Contains(err.Error(), filepath.Join("2026", "10", "bad-yaml.md")+": ") {
		t.Errorf("error %q should read <file>: <reason>", err)
	}
}

func TestListReturnsTheGoodNotesAndReportsCorruptOnes(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	savedNote(t, noteStore, &model.Note{Summary: "good", Status: model.StatusActive, Created: time.Now()})
	corruptMonth(t, root)
	notes, err := noteStore.List()
	if got := summaries(notes); !slices.Equal(got, []string{"good"}) {
		t.Errorf("notes = %q, want the good note", got)
	}
	wantCorruptReasons(t, err)
}

func TestListDashboardReturnsTheGoodNotesAndReportsCorruptOnes(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	savedNote(t, noteStore, &model.Note{Summary: "good", Status: model.StatusActive, Created: time.Now()})
	corruptMonth(t, root)
	notes, err := noteStore.ListDashboard(time.Now(), time.Now().AddDate(0, 0, -1))
	if got := summaries(notes); !slices.Equal(got, []string{"good"}) {
		t.Errorf("notes = %q, want the good note", got)
	}
	wantCorruptReasons(t, err)
}

func TestListReportsUnreadableFilesAndFolders(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	savedNote(t, noteStore, &model.Note{Summary: "good", Status: model.StatusActive, Created: time.Now()})
	unreadableFile := writeRawNote(t, filepath.Join(root, "2026", "10", "locked-out.md"), "---\nsummary: hidden\n---\n")
	unreadableFolder := filepath.Join(root, "2025", "01")
	writeRawNote(t, filepath.Join(unreadableFolder, "inside.md"), "---\nsummary: inside\n---\n")
	for _, path := range []string{unreadableFile, unreadableFolder} {
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(path, 0o700) })
	}
	notes, err := noteStore.List()
	if got := summaries(notes); !slices.Equal(got, []string{"good"}) {
		t.Errorf("notes = %q", got)
	}
	reasons := skippedFiles(t, err)
	for _, file := range []string{filepath.Join("2026", "10", "locked-out.md"), filepath.Join("2025", "01")} {
		if !strings.Contains(reasons[file], "permission denied") {
			t.Errorf("reason for %s = %q, want permission denied (all: %v)", file, reasons[file], reasons)
		}
	}
}

func TestFindByIDIgnoresOtherCorruptNotesButReportsThemWhenNothingMatches(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	good := savedNote(t, noteStore, &model.Note{Summary: "good", Status: model.StatusActive, Created: time.Now()})
	corruptMonth(t, root)
	if found, err := noteStore.FindByID(good.ID); err != nil || found.Summary != "good" {
		t.Errorf("FindByID = %+v, %v", found, err)
	}
	_, err := noteStore.FindByID("no-such-id")
	if err == nil || !strings.Contains(err.Error(), "note not found: no-such-id") {
		t.Errorf("err = %v, want not found", err)
	}
	wantCorruptReasons(t, err)
}

func TestFindByIDRejectsMissingAndAmbiguousPrefixes(t *testing.T) {
	noteStore := New(t.TempDir())
	created := time.Date(2026, 10, 9, 9, 0, 0, 0, time.Local)
	savedNote(t, noteStore, &model.Note{Summary: "first", Status: model.StatusActive, Created: created})
	savedNote(t, noteStore, &model.Note{Summary: "second", Status: model.StatusActive, Created: created.Add(time.Second)})
	if _, err := noteStore.FindByID("09-10-2026"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("err = %v, want ambiguous", err)
	}
	if _, err := noteStore.FindByID("nothing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want not found", err)
	}
	var skipped *SkippedNotesError
	if _, err := noteStore.FindByID("nothing"); errors.As(err, &skipped) {
		t.Errorf("a clean store should not report skipped notes: %v", err)
	}
}

func TestLoadByIDReturnsTheNewestFinishedCopy(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 3, 4, 9, 0, 0, 0, time.Local)
	id := NoteID(created)
	month := filepath.Join(root, "2026", "03")
	finishes := []time.Time{created.Add(time.Hour), created.Add(72 * time.Hour), created.Add(24 * time.Hour)}
	for index, finished := range finishes {
		name := id + "-" + strconv.FormatInt(finished.UnixMilli(), 10) + doneExtension
		writeRawNote(t, filepath.Join(month, name), "---\nid: "+id+"\nstatus: done\nsummary: copy "+strconv.Itoa(index)+"\n---\n")
	}
	archivedName := id + "-" + strconv.FormatInt(created.Add(48*time.Hour).UnixMilli(), 10) + archivedExtension
	writeRawNote(t, filepath.Join(month, archivedName), "---\nid: "+id+"\nstatus: archived\nsummary: archived copy\n---\n")
	for _, name := range []string{id + "-first-legacy.md", id + "-second-legacy.md", id + "-x.done.md"} {
		writeRawNote(t, filepath.Join(month, name), "---\nid: "+id+"\nsummary: "+name+"\n---\n")
	}
	loaded, err := New(root).LoadByID(id)
	if err != nil || loaded.Summary != "copy 1" {
		t.Errorf("LoadByID = %+v, %v; want the copy finished last", loaded, err)
	}
}

func TestLoadByIDStillPrefersTheActiveCopy(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 3, 4, 9, 0, 0, 0, time.Local)
	id := NoteID(created)
	month := filepath.Join(root, "2026", "03")
	writeRawNote(t, filepath.Join(month, id+"-"+strconv.FormatInt(created.Add(time.Hour).UnixMilli(), 10)+doneExtension), "---\nid: "+id+"\nstatus: done\nsummary: done copy\n---\n")
	writeRawNote(t, filepath.Join(month, id+noteExtension), "---\nid: "+id+"\nstatus: active\nsummary: active copy\n---\n")
	writeRawNote(t, filepath.Join(month, id+"0"+noteExtension), "---\nid: "+id+"0\nsummary: longer id\n---\n")
	if loaded, err := New(root).LoadByID(id); err != nil || loaded.Summary != "active copy" {
		t.Errorf("LoadByID = %+v, %v", loaded, err)
	}
}

func TestLoadByIDReportsACorruptCopyAndReturnsTheGoodOne(t *testing.T) {
	root := t.TempDir()
	created := time.Date(2026, 3, 4, 9, 0, 0, 0, time.Local)
	id := NoteID(created)
	month := filepath.Join(root, "2026", "03")
	writeRawNote(t, filepath.Join(month, id+noteExtension), "not a note")
	writeRawNote(t, filepath.Join(month, id+"-"+strconv.FormatInt(created.Add(time.Hour).UnixMilli(), 10)+doneExtension), "---\nid: "+id+"\nstatus: done\nsummary: good copy\n---\n")
	loaded, err := New(root).LoadByID(id)
	if loaded == nil || loaded.Summary != "good copy" {
		t.Errorf("LoadByID = %+v, want the good copy", loaded)
	}
	if reasons := skippedFiles(t, err); len(reasons) != 1 || reasons[filepath.Join("2026", "03", id+noteExtension)] == "" {
		t.Errorf("skipped = %v", reasons)
	}
}

func TestLoadByIDWithOnlyACorruptCopyIsMissingAndReported(t *testing.T) {
	root := t.TempDir()
	writeRawNote(t, filepath.Join(root, "2026", "05", "MyPR:acme:web:7.md"), "---\nsummary: [unclosed\n---\n")
	_, err := New(root).LoadByID("MyPR:acme:web:7")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want os.ErrNotExist", err)
	}
	if reasons := skippedFiles(t, err); len(reasons) != 1 {
		t.Errorf("skipped = %v", reasons)
	}
}

func TestUnknownStatusesLoadAsActive(t *testing.T) {
	month := filepath.Join(t.TempDir(), "2026", "10")
	for name, line := range map[string]string{"empty": "", "inbox": "status: inbox\n", "someday": "status: someday\n"} {
		path := writeRawNote(t, filepath.Join(month, name+".md"), "---\nsummary: "+name+"\n"+line+"---\n")
		if loaded, err := Load(path); err != nil || loaded.Status != model.StatusActive {
			t.Errorf("%s: status = %+v, %v; want active", name, loaded, err)
		}
	}
	for _, status := range []model.Status{model.StatusDone, model.StatusArchived} {
		path := writeRawNote(t, filepath.Join(month, string(status)+".md"), "---\nstatus: "+string(status)+"\n---\n")
		if loaded, err := Load(path); err != nil || loaded.Status != status {
			t.Errorf("%s: status = %+v, %v", status, loaded, err)
		}
	}
}

func TestAutomatedValuesLoadUnchanged(t *testing.T) {
	month := filepath.Join(t.TempDir(), "2026", "10")
	for _, value := range []string{"ticket", "jira", "google doc"} {
		path := writeRawNote(t, filepath.Join(month, value+".md"), "---\nsummary: x\nautomated: "+value+"\n---\n")
		if loaded, err := Load(path); err != nil || loaded.Automated != value {
			t.Errorf("automated %q loaded as %+v, %v", value, loaded, err)
		}
	}
}

func TestDeletingANoteWhoseFileIsGoneCountsAsDeleted(t *testing.T) {
	noteStore := New(t.TempDir())
	note := savedNote(t, noteStore, &model.Note{Summary: "gone", Status: model.StatusActive, Created: time.Now()})
	if err := os.Remove(note.FilePath); err != nil {
		t.Fatal(err)
	}
	if err := noteStore.Delete(note); err != nil {
		t.Errorf("delete of a missing file = %v, want nil", err)
	}
}

func TestDeleteRefusesNotesWithoutAFileOrOutsideTheRoot(t *testing.T) {
	noteStore := New(t.TempDir())
	if err := noteStore.Delete(&model.Note{Summary: "unsaved"}); err == nil || !strings.Contains(err.Error(), "no file") {
		t.Errorf("err = %v, want no file", err)
	}
	outside := writeRawNote(t, filepath.Join(t.TempDir(), "outside.md"), "---\n---\n")
	if err := noteStore.Delete(&model.Note{FilePath: outside}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("err = %v, want outside", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside file should be kept: %v", err)
	}
}

func TestDeleteReportsAFileThatCannotBeRemoved(t *testing.T) {
	noteStore := New(t.TempDir())
	note := savedNote(t, noteStore, &model.Note{Summary: "stuck", Status: model.StatusActive, Created: time.Now()})
	monthDir := filepath.Dir(note.FilePath)
	if err := os.Chmod(monthDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(monthDir, 0o700) })
	if err := noteStore.Delete(note); err == nil || !strings.Contains(err.Error(), "failed to delete note file") {
		t.Errorf("err = %v, want a delete failure", err)
	}
}

func TestSaveRefusesPathsOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	for _, path := range []string{filepath.Join(t.TempDir(), "x.md"), root, filepath.Join(root, "..", "x.md")} {
		if err := noteStore.Save(&model.Note{Summary: "x", FilePath: path}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("Save(%s) = %v, want outside", path, err)
		}
	}
}

func TestSaveKeepsTheNameOfANoteWithALegacyID(t *testing.T) {
	root := t.TempDir()
	path := writeRawNote(t, filepath.Join(root, "2026", "09", "01M3P3RDCCFE87NP9MRHHGRVKS-old.md"), "---\nid: 01M3P3RDCCFE87NP9MRHHGRVKS\nsummary: legacy\n---\n")
	note, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	note.Status, note.Body = model.StatusDone, "edited"
	if err := New(root).Save(note); err != nil || note.FilePath != path {
		t.Errorf("Save = %v, path %s; want it kept at %s", err, note.FilePath, path)
	}
}

func TestSaveReportsAWriteFailure(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	if err := os.WriteFile(filepath.Join(root, "2026"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	note := &model.Note{Summary: "x", Status: model.StatusActive, Created: time.Date(2026, 3, 4, 9, 0, 0, 0, time.Local)}
	if err := noteStore.Save(note); err == nil || !strings.Contains(err.Error(), "failed to write note file") {
		t.Errorf("err = %v, want a write failure", err)
	}
}

func TestSaveFillsMissingTimesAndWritesTheBody(t *testing.T) {
	noteStore := New(t.TempDir())
	before := time.Now()
	note := savedNote(t, noteStore, &model.Note{Summary: "fresh", Body: "  body text  \n", Status: model.StatusActive})
	if note.Created.Before(before) || note.Updated.Before(before) {
		t.Errorf("times = %v %v, want now", note.Created, note.Updated)
	}
	if loaded, err := Load(note.FilePath); err != nil || loaded.Body != "body text" {
		t.Errorf("loaded = %+v, %v", loaded, err)
	}
}

func TestStemIDsAreDateThenMillis(t *testing.T) {
	cases := map[string]bool{
		"09-10-2026-1712":   true,
		"09-10-2026":        false,
		"31-02-2026-1":      false,
		"09-10-2026x1":      false,
		"09-10-2026-abc":    false,
		"MyPR:acme:web:7":   false,
		NoteID(time.Now()):  true,
		"01M3P3RDCCFE87NP9": false,
	}
	for id, want := range cases {
		if got := IsStemID(id); got != want {
			t.Errorf("IsStemID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestAStoreWithAnUnresolvableRootRefusesToWork(t *testing.T) {
	noteStore := New("$DIGEST_UNSET_NOTES_ROOT/notes")
	wantRootError := func(operation string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "DIGEST_UNSET_NOTES_ROOT") {
			t.Errorf("%s err = %v, want the unset variable named", operation, err)
		}
	}
	_, err := noteStore.List()
	wantRootError("List", err)
	_, err = noteStore.LoadByID(NoteID(time.Now()))
	wantRootError("LoadByID", err)
	_, err = noteStore.FindByID("x")
	wantRootError("FindByID", err)
	wantRootError("Save", noteStore.Save(&model.Note{Summary: "x", Status: model.StatusActive}))
	wantRootError("Delete", noteStore.Delete(&model.Note{Summary: "x", FilePath: "/tmp/x.md"}))
	_, err = noteStore.ListDashboard(time.Now(), time.Now().AddDate(0, 0, -1))
	wantRootError("ListDashboard", err)
	_, err = noteStore.ListDoneSince(time.Now())
	wantRootError("ListDoneSince", err)
}

func TestNewNotesNeverReuseTheIDOfAnActiveNote(t *testing.T) {
	noteStore := New(t.TempDir())
	created := time.Now()
	first := savedNote(t, noteStore, &model.Note{Summary: "a", Status: model.StatusActive, Created: created})
	second := savedNote(t, noteStore, &model.Note{Summary: "b", Status: model.StatusActive, Created: created})
	if second.ID == first.ID || second.FilePath == first.FilePath {
		t.Errorf("second note reused %q", first.ID)
	}
}

func TestLoadReportsAMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "gone.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want os.ErrNotExist", err)
	}
}

func TestListOnAMissingRootIsEmpty(t *testing.T) {
	notes, err := New(filepath.Join(t.TempDir(), "missing")).List()
	if err != nil || len(notes) != 0 {
		t.Errorf("notes = %v, err = %v", notes, err)
	}
}

func TestListSkipsHiddenFoldersAndOtherFiles(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	savedNote(t, noteStore, &model.Note{Summary: "visible", Status: model.StatusActive, Created: time.Now()})
	writeRawNote(t, filepath.Join(root, ".state", "hidden.md"), "not a note")
	writeRawNote(t, filepath.Join(root, "2026", "10", "notes.txt"), "not a note")
	notes, err := noteStore.List()
	if got := summaries(notes); err != nil || !slices.Equal(got, []string{"visible"}) {
		t.Errorf("notes = %q, err = %v", got, err)
	}
}

func TestRelativeNotePathsAreOutsideAnAbsoluteRoot(t *testing.T) {
	if err := New(t.TempDir()).Save(&model.Note{Summary: "x", FilePath: "x.md"}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("err = %v, want outside", err)
	}
}

func TestARelativeRootKeepsNotesInsideIt(t *testing.T) {
	t.Chdir(t.TempDir())
	note := savedNote(t, New("notes"), &model.Note{Summary: "x", Status: model.StatusActive, Created: time.Now()})
	if !strings.HasPrefix(note.FilePath, "notes"+string(filepath.Separator)) {
		t.Errorf("path = %s, want it under notes/", note.FilePath)
	}
}

func TestDashboardLoadsUnnamedFinishedFilesAndSkipsHiddenFolders(t *testing.T) {
	root := t.TempDir()
	month := filepath.Join(root, "2026", "01")
	writeRawNote(t, filepath.Join(month, "legacy.done.md"), "---\nstatus: done\nsummary: legacy done\n---\n")
	writeRawNote(t, filepath.Join(month, "legacy.archived.md"), "---\nstatus: archived\nsummary: legacy archived\n---\n")
	writeRawNote(t, filepath.Join(root, ".state", "hidden.md"), "---\nsummary: hidden\n---\n")
	notes, err := New(root).ListDashboard(time.Now(), time.Now().AddDate(0, 0, -1))
	if got := summaries(notes); err != nil || !slices.Equal(got, []string{"legacy archived", "legacy done"}) {
		t.Errorf("notes = %q, err = %v", got, err)
	}
}

func sealFolder(t *testing.T, folder string) {
	t.Helper()
	writeRawNote(t, filepath.Join(folder, "inside.md"), "---\nsummary: inside\n---\n")
	if err := os.Chmod(folder, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(folder, 0o700) })
}

func TestListDashboardReportsAnUnreadableFolder(t *testing.T) {
	root := t.TempDir()
	sealFolder(t, filepath.Join(root, "2025", "01"))
	_, err := New(root).ListDashboard(time.Now(), time.Now().AddDate(0, 0, -1))
	if reason := skippedFiles(t, err)[filepath.Join("2025", "01")]; !strings.Contains(reason, "permission denied") {
		t.Errorf("reason = %q, want permission denied", reason)
	}
}

func TestListDoneSinceReturnsNotesFinishedSinceAndReportsCorruptOnes(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	since := day(-7, 0)
	add := func(summary string, status model.Status, updated time.Time) {
		savedNote(t, noteStore, &model.Note{Summary: summary, Status: status, Source: model.SourceManual, Created: updated.Add(-time.Hour), Updated: updated})
	}
	add("done this week", model.StatusDone, day(-2, 9))
	add("done long ago", model.StatusDone, day(-30, 9))
	add("still active", model.StatusActive, day(-1, 9))
	add("archived this week", model.StatusArchived, day(-1, 9))
	month := filepath.Join(root, "2026", "01")
	writeRawNote(t, filepath.Join(month, "legacy-recent.done.md"), "---\nstatus: done\nsummary: legacy recent\nupdated: "+day(-1, 9).Format(time.RFC3339)+"\n---\n")
	writeRawNote(t, filepath.Join(month, "legacy-old.done.md"), "---\nstatus: done\nsummary: legacy old\nupdated: "+day(-20, 9).Format(time.RFC3339)+"\n---\n")
	writeRawNote(t, filepath.Join(month, "reopened.done.md"), "---\nstatus: active\nsummary: reopened\n---\n")
	writeRawNote(t, filepath.Join(month, "broken.done.md"), "---\nsummary: [unclosed\n---\n")
	writeRawNote(t, filepath.Join(root, ".state", "hidden.done.md"), "---\nstatus: done\nsummary: hidden\n---\n")
	sealFolder(t, filepath.Join(root, "2025", "02"))
	notes, err := noteStore.ListDoneSince(since)
	if got := summaries(notes); !slices.Equal(got, []string{"done this week", "legacy recent"}) {
		t.Errorf("notes = %q", got)
	}
	reasons := skippedFiles(t, err)
	if len(reasons) != 2 || !strings.Contains(reasons[filepath.Join("2026", "01", "broken.done.md")], "yaml") || !strings.Contains(reasons[filepath.Join("2025", "02")], "permission denied") {
		t.Errorf("skipped = %v", reasons)
	}
}

func TestListDoneSinceOnAMissingRootIsEmpty(t *testing.T) {
	notes, err := New(filepath.Join(t.TempDir(), "missing")).ListDoneSince(time.Now())
	if err != nil || len(notes) != 0 {
		t.Errorf("notes = %v, err = %v", notes, err)
	}
}

func TestLoadNamesTheFileItCannotParse(t *testing.T) {
	path := writeRawNote(t, filepath.Join(t.TempDir(), "broken.md"), "plain text")
	if _, err := Load(path); err == nil || !strings.HasPrefix(err.Error(), path+": ") {
		t.Errorf("err = %v, want it to start with the path", err)
	}
}
