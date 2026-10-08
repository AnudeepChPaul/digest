package migrate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/notify"
	"github.com/AnudeepChPaul/digest/pkg/store"
)

type fixture struct {
	root     string
	notesDir string
	output   bytes.Buffer
	lookups  map[string]time.Time
}

func newFixture(t *testing.T) *fixture {
	root := t.TempDir()
	return &fixture{root: root, notesDir: filepath.Join(root, "notes"), lookups: map[string]time.Time{}}
}

func (f *fixture) writeNote(t *testing.T, relative, frontmatter, body string) string {
	t.Helper()
	path := filepath.Join(f.notesDir, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\n"+frontmatter+"---\n"+body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (f *fixture) options() Options {
	return Options{
		NotesDir:      f.notesDir,
		Root:          f.root,
		AutomationDir: filepath.Join(f.root, "automations"),
		TUIMarker:     filepath.Join(f.root, ".state", "tui.pid"),
		Out:           &f.output,
		PRCreatedAt: func(url string) (time.Time, error) {
			if created, found := f.lookups[url]; found {
				return created, nil
			}
			return time.Time{}, errors.New("not found")
		},
	}
}

func (f *fixture) notesByRef(t *testing.T) (map[string]*model.Note, []*model.Note) {
	t.Helper()
	notes, err := store.New(f.notesDir).List()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]*model.Note{}
	for _, note := range notes {
		key := note.Ref
		if key == "" {
			key = note.Summary
		}
		byKey[key] = note
	}
	return byKey, notes
}

func TestMigrateRenamesReIDsAndFillsRefs(t *testing.T) {
	f := newFixture(t)
	activeCreated := time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local)
	activeID := store.NoteID(activeCreated)
	f.writeNote(t, "2026/10/"+activeID+".md", "id: "+activeID+"\ncreated: "+activeCreated.Format(time.RFC3339Nano)+"\nupdated: "+activeCreated.Format(time.RFC3339Nano)+"\nstatus: active\nsource: manual\nsummary: active\n", "")
	doneUpdated := time.Date(2026, 10, 7, 18, 0, 0, 0, time.Local)
	doneID := store.NoteID(activeCreated.Add(time.Minute))
	f.writeNote(t, "2026/10/"+doneID+".md", "id: "+doneID+"\ncreated: "+activeCreated.Add(time.Minute).Format(time.RFC3339Nano)+"\nupdated: "+doneUpdated.Format(time.RFC3339Nano)+"\nstatus: done\nsource: manual\nsummary: done\n", "")
	ulidCreated := time.Date(2026, 9, 28, 23, 30, 0, 0, time.UTC)
	f.writeNote(t, "2026/09/01M3P3RDCCFE87NP9MRHHGRVKS-check-pods.md", "id: 01M3P3RDCCFE87NP9MRHHGRVKS\ncreated: "+ulidCreated.Format(time.RFC3339Nano)+"\nupdated: "+ulidCreated.Format(time.RFC3339Nano)+"\nstatus: active\nsource: manual\nsummary: ulid one\n", "")
	f.writeNote(t, "2026/09/01M3P3RDCCFE87NP9MRHHGRVKT-twin.md", "id: 01M3P3RDCCFE87NP9MRHHGRVKT\ncreated: "+ulidCreated.Format(time.RFC3339Nano)+"\nupdated: "+ulidCreated.Format(time.RFC3339Nano)+"\nstatus: archived\nsource: manual\nsummary: ulid twin\n", "")
	f.writeNote(t, "2026/10/no-id.md", "created: "+activeCreated.Add(2*time.Minute).Format(time.RFC3339Nano)+"\nstatus: inbox\nsummary: without id\n", "")
	syncTime := time.Date(2026, 10, 8, 20, 22, 26, 0, time.Local)
	f.writeNote(t, "2026/10/08-10-2026-1791471146280.md", "id: MyPR:flex:flex-console:449\ncreated: "+syncTime.Format(time.RFC3339Nano)+"\nupdated: "+syncTime.Format(time.RFC3339Nano)+"\nstatus: active\nsource: my-pr\nsummary: mine\n",
		"https://github.com/flex/flex-console/pull/449\n\n## Updates\n\n- 2026-10-08 20:22 CI passing\n- 2026-10-05 11:15 Opened")
	reviewCreated := time.Date(2026, 9, 30, 10, 0, 0, 0, time.Local)
	f.lookups["https://github.com/o/console/pull/7"] = reviewCreated
	f.writeNote(t, "2026/10/07-10-2026-1791394547610.md", "id: console:7\ncreated: "+syncTime.Format(time.RFC3339Nano)+"\nupdated: "+syncTime.Format(time.RFC3339Nano)+"\nstatus: done\nsource: pr-review\nsummary: reviewed\n", "https://github.com/o/console/pull/7")
	f.writeNote(t, "2026/10/07-10-2026-1791394547611.md", "id: web:3\ncreated: "+syncTime.Format(time.RFC3339Nano)+"\nupdated: "+syncTime.Format(time.RFC3339Nano)+"\nstatus: active\nsource: pr-review\nsummary: orphan\n", "no link here")
	if err := notify.Save(f.root, notify.Entry{NoteID: "01M3P3RDCCFE87NP9MRHHGRVKS", Summary: "ulid one", Interval: "1h"}); err != nil {
		t.Fatal(err)
	}

	changed, err := Run(f.options())
	if err != nil {
		t.Fatal(err)
	}
	if changed != 7 {
		t.Errorf("changed = %d, want 7\n%s", changed, f.output.String())
	}
	byKey, notes := f.notesByRef(t)
	if len(notes) != 8 {
		t.Fatalf("notes = %d, want 8", len(notes))
	}
	for _, note := range notes {
		if !strings.HasPrefix(filepath.Base(note.FilePath), note.ID) || filepath.Base(note.FilePath) != store.FileName(note) {
			t.Errorf("%q: file %q does not follow its id", note.Summary, note.FilePath)
		}
	}
	if note := byKey["active"]; note.ID != activeID {
		t.Errorf("timestamp id changed: %q", note.ID)
	}
	if note := byKey["done"]; filepath.Base(note.FilePath) != doneID+"-"+strconv.FormatInt(doneUpdated.UnixMilli(), 10)+".done.md" {
		t.Errorf("done file = %q", note.FilePath)
	}
	ulidOne, ulidTwin := byKey["ulid one"], byKey["ulid twin"]
	if ulidOne.ID != store.NoteID(ulidCreated) || ulidTwin.ID == ulidOne.ID || !strings.HasPrefix(ulidOne.ID, ulidCreated.Local().Format("02-01-2006")) {
		t.Errorf("ulid ids = %q, %q", ulidOne.ID, ulidTwin.ID)
	}
	if entry, found := notify.Load(f.root, ulidOne.ID); !found || entry.NoteID != ulidOne.ID {
		t.Errorf("reminder should move to the new id: %+v %v", entry, found)
	}
	if _, found := notify.Load(f.root, "01M3P3RDCCFE87NP9MRHHGRVKS"); found {
		t.Error("old reminder file should be gone")
	}
	if note := byKey["without id"]; note == nil || note.ID == "" {
		t.Errorf("note without id should get one: %+v", note)
	}
	opened := time.Date(2026, 10, 5, 11, 15, 0, 0, time.Local)
	if note := byKey["flex/flex-console#449"]; note == nil || !note.Created.Equal(opened) || note.ID != store.NoteID(opened) {
		t.Errorf("my-PR note should take the Opened time: %+v", note)
	}
	if note := byKey["o/console#7"]; note == nil || !note.Created.Equal(reviewCreated) || note.ID != store.NoteID(reviewCreated) {
		t.Errorf("review note should take the PR created time from gh: %+v", note)
	}
	if note := byKey["owner/web#3"]; note == nil || !note.Created.Equal(syncTime) {
		t.Errorf("orphan review note should keep its created time: %+v", note)
	}
	if !strings.Contains(f.output.String(), "web:3") {
		t.Errorf("output should mention the failed lookup:\n%s", f.output.String())
	}

	f.output.Reset()
	if again, err := Run(f.options()); err != nil || again != 0 {
		t.Errorf("second run changed %d notes, err %v\n%s", again, err, f.output.String())
	}
}

func TestMigrateRefusesWhileTheTUIOrAnAutomationRuns(t *testing.T) {
	f := newFixture(t)
	options := f.options()
	if err := os.MkdirAll(filepath.Dir(options.TUIMarker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.TUIMarker, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(options); !errors.Is(err, ErrTUIRunning) {
		t.Errorf("err = %v, want ErrTUIRunning", err)
	}
	os.Remove(options.TUIMarker)
	runDir := filepath.Join(options.AutomationDir, "06-10-2026-1791290202215")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "automation.pid"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(options); !errors.Is(err, ErrAutomationRunning) {
		t.Errorf("err = %v, want ErrAutomationRunning", err)
	}
}
