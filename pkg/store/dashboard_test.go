package store

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
)

func day(offset int, hour int) time.Time {
	today := time.Now()
	return time.Date(today.Year(), today.Month(), today.Day(), hour, 0, 0, 0, time.Local).AddDate(0, 0, offset)
}

func summaries(notes []*model.Note) []string {
	var names []string
	for _, note := range notes {
		names = append(names, note.Summary)
	}
	slices.Sort(names)
	return names
}

func TestDashboardLoaderReadsActiveNotesAndOnlyThePreviousDaysEarlierDoneNotes(t *testing.T) {
	noteStore := New(t.TempDir())
	add := func(summary string, status model.Status, source model.Source, created, updated time.Time) {
		savedNote(t, noteStore, &model.Note{Summary: summary, Status: status, Source: source, Created: created, Updated: updated})
	}
	add("old active", model.StatusActive, model.SourceManual, day(-40, 9), day(-40, 9))
	add("inbox", model.StatusActive, model.SourceManual, day(-30, 9), day(-30, 9))
	add("done today", model.StatusDone, model.SourceManual, day(-10, 9), day(0, 9))
	add("pr done yesterday", model.StatusDone, model.SourcePRReview, day(-1, 8), day(-1, 9))
	add("manual done two days ago", model.StatusDone, model.SourceManual, day(-2, 8), day(-2, 9))
	add("done three days ago", model.StatusDone, model.SourceManual, day(-3, 8), day(-3, 9))
	add("done five days ago", model.StatusDone, model.SourceManual, day(-5, 8), day(-5, 9))
	add("archived", model.StatusArchived, model.SourceManual, day(-1, 8), day(-1, 9))

	notes, err := noteStore.ListDashboard(day(0, 12), day(-3, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"done three days ago", "done today", "inbox", "old active"}
	if got := summaries(notes); !slices.Equal(got, want) {
		t.Errorf("loaded %q, want %q", got, want)
	}
}

func TestDashboardLoaderAlwaysReadsFilesWithOtherNames(t *testing.T) {
	root := t.TempDir()
	noteStore := New(root)
	legacy := filepath.Join(root, "2026", "09", "01M3P3RDCCFE87NP9MRHHGRVKS-old-note.md")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: 01M3P3RDCCFE87NP9MRHHGRVKS\nstatus: done\nsummary: legacy\n---\n"
	if err := os.WriteFile(legacy, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	notes, err := noteStore.ListDashboard(time.Now(), time.Now().AddDate(0, 0, -1))
	if err != nil || len(notes) != 1 || notes[0].Summary != "legacy" {
		t.Errorf("notes = %v, err = %v", summaries(notes), err)
	}
}

func TestDashboardLoaderOnAMissingRootIsEmpty(t *testing.T) {
	notes, err := New(filepath.Join(t.TempDir(), "missing")).ListDashboard(time.Now(), time.Now().AddDate(0, 0, -1))
	if err != nil || len(notes) != 0 {
		t.Errorf("notes = %v, err = %v", notes, err)
	}
}
