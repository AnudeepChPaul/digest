//go:build darwin

package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/appstate"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/store"
	"github.com/AnudeepChPaul/digest/pkg/system"

	"golang.org/x/sys/unix"
)

func TestMigrateLocksNotesAndWritesTheAppState(t *testing.T) {
	f := newFixture(t)
	system.Protect(f.root, filepath.Join(f.root, "reviews"))
	t.Cleanup(func() {
		system.Protect("")
		if err := system.UnlockTree(f.root); err != nil {
			t.Error(err)
		}
	})
	reviewFile := filepath.Join(f.root, "reviews", ".state", "pr", "findings.json")
	if err := os.MkdirAll(filepath.Dir(reviewFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reviewFile, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	firstCreated := time.Date(2026, 9, 28, 9, 0, 0, 0, time.Local)
	legacyPath := f.writeNote(t, "2026/09/01M3P3RDCCFE87NP9MRHHGRVKS-legacy.md", "id: 01M3P3RDCCFE87NP9MRHHGRVKS\ncreated: "+firstCreated.Format(time.RFC3339Nano)+"\nupdated: "+firstCreated.Format(time.RFC3339Nano)+"\nstatus: active\nsource: manual\nsummary: legacy\n", "")
	for index, doneDay := range []int{7, 8} {
		created := time.Date(2026, 10, doneDay, 9, 0, 0, 0, time.Local)
		updated := created.Add(8 * time.Hour)
		id := store.NoteID(created)
		f.writeNote(t, "2026/10/"+store.FileName(&model.Note{ID: id, Status: model.StatusDone, Updated: updated}), "id: "+id+"\ncreated: "+created.Format(time.RFC3339Nano)+"\nupdated: "+updated.Format(time.RFC3339Nano)+"\nstatus: done\nsource: manual\nsummary: done "+string(rune('a'+index))+"\n", "")
	}
	if err := unix.Chflags(legacyPath, unix.UF_IMMUTABLE); err != nil {
		t.Fatal(err)
	}
	options := f.options()
	options.Now = func() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.Local) }
	options.IsWorkDay = func(day time.Weekday) bool { return day != time.Saturday && day != time.Sunday }

	if _, err := Run(options); err != nil {
		t.Fatalf("migrate should handle an already locked note: %v\n%s", err, f.output.String())
	}
	_, notes := f.notesByRef(t)
	if len(notes) != 3 {
		t.Fatalf("notes = %d, want 3", len(notes))
	}
	for _, note := range notes {
		var stat unix.Stat_t
		if err := unix.Lstat(note.FilePath, &stat); err != nil || stat.Flags&unix.UF_IMMUTABLE == 0 {
			t.Errorf("%s should be locked: %v %#x", filepath.Base(note.FilePath), err, stat.Flags)
		}
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("the locked legacy file should be renamed: %v", err)
	}
	var reviewStat unix.Stat_t
	if err := unix.Lstat(reviewFile, &reviewStat); err != nil || reviewStat.Flags&unix.UF_IMMUTABLE != 0 {
		t.Errorf("reviews/ should stay unlocked: %v %#x", err, reviewStat.Flags)
	}
	lockingAt := strings.Index(f.output.String(), "Locking files in "+f.root)
	lockedAt := strings.Index(f.output.String(), "4 files locked")
	if lockingAt < 0 || lockedAt < lockingAt {
		t.Errorf("output should announce locking before reporting the locked notes:\n%s", f.output.String())
	}
	state, found, err := appstate.Load(f.root)
	if err != nil || !found {
		t.Fatalf("app state should be written: %v %v", found, err)
	}
	if !state.FirstNoteCreated.Equal(firstCreated) || state.Streak != 2 || state.LastDoneDay != "2026-10-08" {
		t.Errorf("app state = %+v", state)
	}

	f.output.Reset()
	if again, err := Run(options); err != nil || again != 0 {
		t.Errorf("second run changed %d notes, err %v\n%s", again, err, f.output.String())
	}
	if again, _, _ := appstate.Load(f.root); again != state {
		t.Errorf("second run should keep the same state: %+v vs %+v", again, state)
	}
	edited := appstate.State{FirstNoteCreated: firstCreated.AddDate(-1, 0, 0), Streak: 9, LastDoneDay: "2026-10-09"}
	if err := appstate.Save(f.root, edited); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(options); err != nil {
		t.Fatal(err)
	}
	if kept, _, _ := appstate.Load(f.root); kept != edited {
		t.Errorf("an existing state file should be left untouched: %+v", kept)
	}
}
