package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

const myPRUpdatesHeading = "\n## Updates\n"

var myPRNotesMu sync.Mutex

type myPRSeen struct {
	Ref         review.PRRef         `json:"ref"`
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	CIState     string               `json:"ci_state,omitempty"`
	IsDraft     bool                 `json:"is_draft,omitempty"`
	Reviews     map[string]time.Time `json:"reviews,omitempty"`
	LastReplyAt time.Time            `json:"last_reply_at"`
}

type myPRUpdate struct {
	at   time.Time
	text string
}

type myPRNotesMsg struct {
	known   []review.PRRef
	changed []model.Note
	saved   bool
	missing error
	err     error
}

func myPRNoteID(ref review.PRRef) string {
	return fmt.Sprintf("MyPR:%s:%s:%d", ref.Owner, ref.Repo, ref.Number)
}

func myPRNoteSummary(ref review.PRRef, title string) string {
	return fmt.Sprintf("NewPR: %s:%s:%d: %s", ref.Owner, ref.Repo, ref.Number, title)
}

func myPRsSeenPath(root string) string {
	return filepath.Join(root, ".state", "my-prs-seen.json")
}

func loadMyPRsSeen(path string) (map[string]myPRSeen, error) {
	seen := map[string]myPRSeen{}
	encoded, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return seen, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(encoded, &seen); err != nil {
		return map[string]myPRSeen{}, nil
	}
	return seen, nil
}

func saveMyPRsSeen(path string, seen map[string]myPRSeen) error {
	if err := os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode); err != nil {
		return err
	}
	encoded, err := json.Marshal(seen)
	if err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".my-prs-seen-*.json")
	if err != nil {
		return err
	}
	if _, err := tempFile.Write(encoded); err != nil {
		tempFile.Close()
		os.Remove(tempFile.Name())
		return err
	}
	if err := tempFile.Close(); err != nil {
		os.Remove(tempFile.Name())
		return err
	}
	if err := os.Rename(tempFile.Name(), path); err != nil {
		os.Remove(tempFile.Name())
		return err
	}
	return nil
}

func knownMyPRRefs(seen map[string]myPRSeen) []review.PRRef {
	refs := make([]review.PRRef, 0, len(seen))
	for _, entry := range seen {
		refs = append(refs, entry.Ref)
	}
	slices.SortFunc(refs, func(a, b review.PRRef) int { return strings.Compare(a.URL, b.URL) })
	return refs
}

func seenFromPR(pr review.QueuedPR) myPRSeen {
	entry := myPRSeen{Ref: pr.Ref, Title: pr.Title, Body: normalizedDescription(pr.Body), CIState: pr.CIState, IsDraft: pr.IsDraft, LastReplyAt: pr.LastReplyAt}
	for _, prReview := range pr.Reviews {
		if entry.Reviews == nil {
			entry.Reviews = map[string]time.Time{}
		}
		if prReview.SubmittedAt.After(entry.Reviews[prReview.Author]) {
			entry.Reviews[prReview.Author] = prReview.SubmittedAt
		}
	}
	return entry
}

func normalizedDescription(body string) string {
	return strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
}

var ciUpdateText = map[string]string{"SUCCESS": "CI passing", "FAILURE": "CI failing", "PENDING": "CI running"}

var reviewUpdateText = map[string]string{
	"APPROVED":          "Approved by @%s",
	"CHANGES_REQUESTED": "Changes requested by @%s",
	"COMMENTED":         "Reviewed with comments by @%s",
	"DISMISSED":         "Review dismissed for @%s",
}

func myPRUpdates(previous *myPRSeen, pr review.QueuedPR, now time.Time) []myPRUpdate {
	var updates []myPRUpdate
	if previous == nil {
		updates = append(updates, myPRUpdate{at: pr.CreatedAt, text: "Opened"})
	} else {
		if pr.Title != previous.Title {
			updates = append(updates, myPRUpdate{at: now, text: fmt.Sprintf("Title changed to %q", pr.Title)})
		}
		if normalizedDescription(pr.Body) != previous.Body {
			updates = append(updates, myPRUpdate{at: now, text: "Description updated"})
		}
	}
	for _, prReview := range pr.Reviews {
		format, known := reviewUpdateText[prReview.State]
		if !known || prReview.Author == pr.Author || review.IsBot(prReview.Author) {
			continue
		}
		if previous != nil && !prReview.SubmittedAt.After(previous.Reviews[prReview.Author]) {
			continue
		}
		updates = append(updates, myPRUpdate{at: prReview.SubmittedAt, text: fmt.Sprintf(format, prReview.Author)})
	}
	if previous != nil && pr.LastReplyAt.After(previous.LastReplyAt) {
		updates = append(updates, myPRUpdate{at: pr.LastReplyAt, text: "New comment"})
	}
	if previous != nil && previous.IsDraft && !pr.IsDraft {
		updates = append(updates, myPRUpdate{at: now, text: "Marked ready for review"})
	}
	if text, known := ciUpdateText[pr.CIState]; known && (previous == nil || previous.CIState != pr.CIState) {
		updates = append(updates, myPRUpdate{at: now, text: text})
	}
	slices.SortStableFunc(updates, func(a, b myPRUpdate) int { return b.at.Compare(a.at) })
	return updates
}

func splitMyPRBody(body string) (head, updates string) {
	index := strings.LastIndex(body, myPRUpdatesHeading)
	if index < 0 {
		return strings.TrimRight(body, "\n"), ""
	}
	return strings.TrimRight(body[:index], "\n"), strings.TrimSpace(body[index+len(myPRUpdatesHeading):])
}

func myPRBodyHead(pr review.QueuedPR) string {
	description := normalizedDescription(pr.Body)
	if description == "" {
		description = "_No description._"
	}
	return pr.Ref.URL + "\n\n## Description\n\n" + description
}

func composeMyPRBody(head, existingUpdates string, updates []myPRUpdate) string {
	var body strings.Builder
	body.WriteString(head + "\n" + myPRUpdatesHeading + "\n")
	for _, update := range updates {
		fmt.Fprintf(&body, "- %s %s\n", update.at.Local().Format("2006-01-02 15:04"), update.text)
	}
	body.WriteString(existingUpdates)
	return strings.TrimSpace(body.String())
}

func myPRNotesCmd(noteStore *store.NoteStore, seenPath string, prs []review.QueuedPR, closed map[string]string, failedHosts []string, now time.Time) tea.Cmd {
	return knownMyPRNotesCmd(noteStore, prNoteIndex{}, seenPath, prs, closed, failedHosts, now)
}

func (m Model) myPRNotesCmd(prs []review.QueuedPR, closed map[string]string, failedHosts []string, now time.Time) tea.Cmd {
	known := m.prNoteSnapshot(model.SourceMyPR)
	return knownMyPRNotesCmd(m.store, known, myPRsSeenPath(m.cfg.Root()), prs, closed, failedHosts, now)
}

func knownMyPRNotesCmd(noteStore *store.NoteStore, known prNoteIndex, seenPath string, prs []review.QueuedPR, closed map[string]string, failedHosts []string, now time.Time) tea.Cmd {
	return func() tea.Msg {
		myPRNotesMu.Lock()
		defer myPRNotesMu.Unlock()
		seen, err := loadMyPRsSeen(seenPath)
		if err != nil {
			return myPRNotesMsg{err: err}
		}
		finder := newPRNoteFinder(noteStore, model.SourceMyPR, known)
		var changed []model.Note
		var missing []error
		seenChanged := false
		save := func(note *model.Note) error {
			if err := noteStore.Save(note); err != nil {
				return err
			}
			changed = append(changed, *note)
			return nil
		}
		for _, pr := range prs {
			previous, wasSeen := seen[pr.Ref.URL]
			current := seenFromPR(pr)
			if !wasSeen || !jsonEqual(previous, current) {
				seen[pr.Ref.URL], seenChanged = current, true
			}
			ref := prNoteRef(pr.Ref.Owner, pr.Ref.Repo, pr.Ref.Number)
			note, exists, err := finder.find(ref, pr.Ref.Repo, pr.Ref.Number, myPRNoteID(pr.Ref))
			if err != nil {
				missing = append(missing, err)
				continue
			}
			switch {
			case !wasSeen && !exists:
				created := pr.CreatedAt.Local()
				if pr.CreatedAt.IsZero() {
					created = now
				}
				note = &model.Note{Ref: ref, Created: created, Updated: now, Status: model.StatusActive, Source: model.SourceMyPR, Repo: pr.Ref.Repo, Summary: myPRNoteSummary(pr.Ref, pr.Title)}
				note.Body = composeMyPRBody(myPRBodyHead(pr), "", myPRUpdates(nil, pr, now))
			case !exists:
				continue
			default:
				var previousSeen *myPRSeen
				if wasSeen {
					previousSeen = &previous
				}
				_, existingUpdates := splitMyPRBody(note.Body)
				body := composeMyPRBody(myPRBodyHead(pr), existingUpdates, myPRUpdates(previousSeen, pr, now))
				summary := myPRNoteSummary(pr.Ref, pr.Title)
				if body == note.Body && summary == note.Summary {
					continue
				}
				note.Body, note.Summary, note.Updated = body, summary, now
			}
			if note.Ref == "" {
				note.Ref = ref
			}
			if err := save(note); err != nil {
				return myPRNotesMsg{known: knownMyPRRefs(seen), changed: changed, saved: len(changed) > 0, missing: errors.Join(missing...), err: err}
			}
			finder.remember(note)
		}
		for url, state := range closed {
			entry, wasSeen := seen[url]
			if !wasSeen {
				continue
			}
			delete(seen, url)
			seenChanged = true
			note, exists, err := finder.find(prNoteRef(entry.Ref.Owner, entry.Ref.Repo, entry.Ref.Number), entry.Ref.Repo, entry.Ref.Number, myPRNoteID(entry.Ref))
			if err != nil {
				missing = append(missing, err)
				continue
			}
			if !exists {
				continue
			}
			head, existingUpdates := splitMyPRBody(note.Body)
			text := "Closed"
			if state == "MERGED" {
				text = "Merged"
			}
			note.Body = composeMyPRBody(head, existingUpdates, []myPRUpdate{{at: now, text: text}})
			note.Status, note.Updated = model.StatusDone, now
			if err := save(note); err != nil {
				return myPRNotesMsg{known: knownMyPRRefs(seen), changed: changed, saved: len(changed) > 0, missing: errors.Join(missing...), err: err}
			}
		}
		if seenChanged {
			if err := saveMyPRsSeen(seenPath, seen); err != nil {
				return myPRNotesMsg{known: knownMyPRRefs(seen), changed: changed, saved: len(changed) > 0, missing: errors.Join(missing...), err: err}
			}
		}
		return myPRNotesMsg{known: knownMyPRRefs(seen), changed: changed, saved: len(changed) > 0, missing: errors.Join(missing...)}
	}
}

func jsonEqual(first, second myPRSeen) bool {
	firstJSON, firstErr := json.Marshal(first)
	secondJSON, secondErr := json.Marshal(second)
	return firstErr == nil && secondErr == nil && string(firstJSON) == string(secondJSON)
}
