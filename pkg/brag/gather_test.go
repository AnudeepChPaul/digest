package brag

import (
	"context"
	"testing"
	"time"

	"app/pkg/config"
	"app/pkg/model"
)

func TestGatherUsesNotesOnlyWhenGitIsOff(t *testing.T) {
	showGit := false
	cfg := &config.Config{DigestRoot: t.TempDir(), GitRepositoryRoots: []string{t.TempDir()}, ShowGit: &showGit}
	notes := []*model.Note{{ID: "note-1", Summary: "wrote docs"}}
	week := WeekOf(localDate(2026, time.October, 7))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sources, err := Gather(ctx, cfg, week, notes, localDate(2026, time.October, 12))
	if err != nil {
		t.Fatalf("git off: Gather err = %v, want nil", err)
	}
	if len(sources.Notes) != 1 || sources.Commits != nil || sources.Opened != nil || sources.Merged != nil {
		t.Errorf("git off: sources = %+v, want notes only", sources)
	}
}
