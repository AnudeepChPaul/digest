package tui

import (
	"errors"
	"testing"

	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
)

func myOpenPR(repo string, number int, branch, ci string) review.QueuedPR {
	return review.QueuedPR{Ref: prRef(repo, number), Title: "Mine", Author: "me", HeadRef: branch, CIState: ci, Body: "desc"}
}

func TestMyPRsSectionListsPRsAndCreatesNotes(t *testing.T) {
	m := syncTestModel(t)
	m.beginGitFetch()
	next, cmd := m.Update(gitMyPRsMsg{generation: m.git.fetchGeneration, partOfSync: true, prs: []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}})
	m = next.(Model)
	if len(m.git.myPRs) != 1 || m.git.myPRs[0].Ref.Number != 4 {
		t.Fatalf("myPRs = %+v", m.git.myPRs)
	}
	if m.git.gitSectionsPending != gitSectionCount-1 {
		t.Errorf("my PRs section did not count towards the sync: pending=%d", m.git.gitSectionsPending)
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
	if len(m.git.knownMyPRs) != 1 {
		t.Errorf("knownMyPRs = %+v", m.git.knownMyPRs)
	}
	found := false
	for _, note := range m.notes {
		found = found || (note.Ref == "o/console#4" && note.Source == model.SourceMyPR)
	}
	if !found {
		t.Errorf("note missing from model: %+v", m.notes)
	}
}

func TestMyPRsFailedFetchKeepsRows(t *testing.T) {
	m := syncTestModel(t)
	m.git.myPRs = []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}
	next, _ := m.Update(gitMyPRsMsg{generation: m.git.fetchGeneration, err: errors.New("offline"), failedHosts: []string{"github.com"}})
	m = next.(Model)
	if len(m.git.myPRs) != 1 || m.git.syncErrors[sectionMyPRs] == "" {
		t.Errorf("myPRs=%+v errors=%+v", m.git.myPRs, m.git.syncErrors)
	}
}

func TestMyPRsCachedAndAlwaysRefetchedOnStartup(t *testing.T) {
	m := syncTestModel(t)
	m.applyGitPending(gitPendingMsg{generation: m.git.fetchGeneration, pending: []GitPRItem{pendingItem(1)}})
	m.applyMyPRs(gitMyPRsMsg{generation: m.git.fetchGeneration, prs: []review.QueuedPR{myOpenPR("console", 4, "fix/a", "SUCCESS")}})
	saveCacheNow(t, m)
	fresh := NewModel(m.cfg, nil)
	if len(fresh.git.myPRs) != 1 || fresh.git.myPRs[0].Body != "desc" {
		t.Fatalf("myPRs from cache = %+v", fresh.git.myPRs)
	}
	if fresh.git.syncOnLoad || !fresh.git.loadingMyPRs {
		t.Errorf("fresh cache must still re-fetch my PRs: syncOnLoad=%v loadingMyPRs=%v", fresh.git.syncOnLoad, fresh.git.loadingMyPRs)
	}
}
