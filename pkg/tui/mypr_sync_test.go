package tui

import (
	"errors"
	"testing"

	"app/pkg/model"
	"app/pkg/review"
)

func myOpenPR(repo string, number int, branch, ci string) review.QueuedPR {
	return review.QueuedPR{Ref: prRef(repo, number), Title: "Mine", Author: "me", HeadRef: branch, CIState: ci, Body: "desc"}
}

func TestMyPRsSectionListsPRsAndCreatesNotes(t *testing.T) {
	m := syncTestModel(t)
	m.beginGitFetch()
	next, cmd := m.Update(gitMyPRsMsg{generation: m.fetchGeneration, partOfSync: true, prs: []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}})
	m = next.(Model)
	if len(m.myPRs) != 1 || m.myPRs[0].Ref.Number != 4 {
		t.Fatalf("myPRs = %+v", m.myPRs)
	}
	if m.gitSectionsPending != gitSectionCount-1 {
		t.Errorf("my PRs section did not count towards the sync: pending=%d", m.gitSectionsPending)
	}
	var notesMsg *myPRNotesMsg
	for _, msg := range collectMsgs(cmd) {
		if notes, ok := msg.(myPRNotesMsg); ok {
			notesMsg = &notes
		}
	}
	if notesMsg == nil || !notesMsg.saved {
		t.Fatalf("no my PR notes written: %+v", notesMsg)
	}
	next, _ = m.Update(*notesMsg)
	m = next.(Model)
	if len(m.knownMyPRs) != 1 {
		t.Errorf("knownMyPRs = %+v", m.knownMyPRs)
	}
	found := false
	for _, note := range m.notes {
		found = found || (note.ID == "MyPR:o:console:4" && note.Source == model.SourceMyPR)
	}
	if !found {
		t.Errorf("note missing from model: %+v", m.notes)
	}
}

func TestMyPRsFailedFetchKeepsRows(t *testing.T) {
	m := syncTestModel(t)
	m.myPRs = []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}
	next, _ := m.Update(gitMyPRsMsg{generation: m.fetchGeneration, err: errors.New("offline"), failedHosts: []string{"github.com"}})
	m = next.(Model)
	if len(m.myPRs) != 1 || m.syncErrors[sectionMyPRs] == "" {
		t.Errorf("myPRs=%+v errors=%+v", m.myPRs, m.syncErrors)
	}
}

func TestMyPRsCachedAndAlwaysRefetchedOnStartup(t *testing.T) {
	m := syncTestModel(t)
	m.applyGitPending(gitPendingMsg{generation: m.fetchGeneration, pending: []GitPRItem{pendingItem(1)}})
	m.applyMyPRs(gitMyPRsMsg{generation: m.fetchGeneration, prs: []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}})
	saveCacheNow(t, m)
	fresh := NewModel(m.cfg, nil)
	if len(fresh.myPRs) != 1 || fresh.myPRs[0].Body != "desc" {
		t.Fatalf("myPRs from cache = %+v", fresh.myPRs)
	}
	if fresh.syncOnLoad || !fresh.loadingMyPRs {
		t.Errorf("fresh cache must still re-fetch my PRs: syncOnLoad=%v loadingMyPRs=%v", fresh.syncOnLoad, fresh.loadingMyPRs)
	}
}
