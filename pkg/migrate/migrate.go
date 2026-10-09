package migrate

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/appstate"
	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/notify"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/running"
	"github.com/AnudeepChPaul/digest/pkg/store"
	"github.com/AnudeepChPaul/digest/pkg/system"
)

var (
	ErrTUIRunning        = errors.New("quit the digest TUI before migrating")
	ErrAutomationRunning = errors.New("wait for running automations to finish before migrating")

	prLinkPattern   = regexp.MustCompile(`https?://[^\s<>()"']+/pull/\d+`)
	openedPattern   = regexp.MustCompile(`(?m)^- (\d{4}-\d{2}-\d{2} \d{2}:\d{2}) Opened$`)
	legacyMyPRID    = regexp.MustCompile(`^MyPR:([^:]+):([^:]+):(\d+)$`)
	legacyReviewID  = regexp.MustCompile(`^([^:]+):(\d+)$`)
	orphanOwnerName = "owner"
)

type Options struct {
	NotesDir      string
	Root          string
	AutomationDir string
	TUIMarker     string
	Out           io.Writer
	PRCreatedAt   func(url string) (time.Time, error)
	IsWorkDay     func(time.Weekday) bool
	Now           func() time.Time
}

type plannedNote struct {
	note       *model.Note
	originalID string
	created    time.Time
}

func Run(options Options) (int, error) {
	if running.Alive(options.TUIMarker) {
		return 0, ErrTUIRunning
	}
	if automation.AnyRunning(options.AutomationDir) {
		return 0, ErrAutomationRunning
	}
	noteStore := store.New(options.NotesDir)
	notes, err := noteStore.List()
	if err != nil {
		return 0, err
	}

	takenIDs := map[string]bool{}
	var needIDs []plannedNote
	for _, note := range notes {
		if isPRNote(note) && note.Ref == "" {
			note.Ref = legacyRef(note)
		}
		if store.IsStemID(note.ID) {
			takenIDs[note.ID] = true
			continue
		}
		needIDs = append(needIDs, plannedNote{note: note, originalID: note.ID, created: createdFor(note, options)})
	}
	slices.SortStableFunc(needIDs, func(a, b plannedNote) int {
		if order := a.created.Compare(b.created); order != 0 {
			return order
		}
		return strings.Compare(a.originalID, b.originalID)
	})
	for _, planned := range needIDs {
		candidate := planned.created
		for takenIDs[store.NoteID(candidate)] {
			candidate = candidate.Add(time.Millisecond)
		}
		planned.note.ID, planned.note.Created = store.NoteID(candidate), candidate
		takenIDs[planned.note.ID] = true
	}

	originalIDs := map[*model.Note]string{}
	for _, planned := range needIDs {
		originalIDs[planned.note] = planned.originalID
	}
	changed := 0
	for _, note := range notes {
		originalID, reIDed := originalIDs[note]
		originalName := filepath.Base(note.FilePath)
		before, err := store.Load(note.FilePath)
		if err != nil {
			return changed, err
		}
		if !reIDed && before.Ref == note.Ref && originalName == store.FileName(note) {
			continue
		}
		if err := noteStore.Save(note); err != nil {
			return changed, fmt.Errorf("migrate %s: %w", originalName, err)
		}
		changed++
		fmt.Fprintf(options.Out, "%s → %s\n", originalName, filepath.Base(note.FilePath))
		if note.Ref != before.Ref {
			fmt.Fprintf(options.Out, "  ref %s\n", note.Ref)
		}
		if reIDed && originalID != "" {
			if err := moveReminder(options.Root, originalID, note.ID); err != nil {
				return changed, err
			}
		}
	}
	fmt.Fprintf(options.Out, "%d notes migrated\n", changed)
	if _, found, err := appstate.Load(options.Root); err != nil {
		return changed, err
	} else if !found {
		if err := appstate.Save(options.Root, appstate.FromNotes(notes, options.now(), options.isWorkDay)); err != nil {
			return changed, fmt.Errorf("save %s: %w", appstate.Path(options.Root), err)
		}
	}
	if system.Protected(options.Root) {
		fmt.Fprintf(options.Out, "Locking files in %s (reviews, logs, config.yaml and *.log files stay unlocked)…\n", options.Root)
		locked, err := system.LockTree(options.Root)
		if err != nil {
			return changed, fmt.Errorf("lock files: %w", err)
		}
		fmt.Fprintf(options.Out, "%d files locked\n", locked)
	}
	return changed, nil
}

func (options Options) now() time.Time {
	if options.Now == nil {
		return time.Now()
	}
	return options.Now()
}

func (options Options) isWorkDay(day time.Weekday) bool {
	if options.IsWorkDay == nil {
		return day != time.Saturday && day != time.Sunday
	}
	return options.IsWorkDay(day)
}

func isPRNote(note *model.Note) bool {
	return note.Source == model.SourcePRReview || note.Source == model.SourceMyPR
}

func prLink(note *model.Note) string {
	links := prLinkPattern.FindAllString(note.Body, -1)
	if len(links) == 0 {
		return ""
	}
	return links[len(links)-1]
}

func legacyRef(note *model.Note) string {
	if match := legacyMyPRID.FindStringSubmatch(note.ID); match != nil {
		return match[1] + "/" + match[2] + "#" + match[3]
	}
	if ref, err := review.ParsePRURL(prLink(note)); err == nil {
		return ref.Owner + "/" + ref.Repo + "#" + strconv.Itoa(ref.Number)
	}
	if match := legacyReviewID.FindStringSubmatch(note.ID); match != nil {
		return orphanOwnerName + "/" + match[1] + "#" + match[2]
	}
	return ""
}

func createdFor(note *model.Note, options Options) time.Time {
	fallback := note.Created
	if fallback.IsZero() {
		fallback = note.Updated
	}
	if fallback.IsZero() {
		fallback = time.Now()
	}
	switch note.Source {
	case model.SourceMyPR:
		if match := openedPattern.FindStringSubmatch(note.Body); match != nil {
			if opened, err := time.ParseInLocation("2006-01-02 15:04", match[1], time.Local); err == nil {
				return opened
			}
		}
	case model.SourcePRReview:
		link := prLink(note)
		if link == "" {
			fmt.Fprintf(options.Out, "kept the created time of %s: no PR link in the note\n", note.ID)
			return fallback
		}
		created, err := options.PRCreatedAt(link)
		if err != nil {
			fmt.Fprintf(options.Out, "kept the created time of %s: %v\n", note.ID, err)
			return fallback
		}
		return created.Local()
	}
	return fallback.Local()
}

func moveReminder(root, originalID, newID string) error {
	entry, found := notify.Load(root, originalID)
	if !found {
		return nil
	}
	entry.NoteID = newID
	if err := notify.Save(root, entry); err != nil {
		return err
	}
	return notify.Remove(root, originalID)
}
