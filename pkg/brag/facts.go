package brag

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/sourcecontrol"
)

const noActivityFacts = "- No recorded activity this week."

type Sources struct {
	Notes   []*model.Note
	Opened  []review.QueuedPR
	Merged  []review.QueuedPR
	Commits map[string][]sourcecontrol.PRItem
}

var bragNoteSources = map[model.Source]bool{"": true, model.SourceManual: true, model.SourceStandup: true, model.SourceJournal: true}

var reviewHistoryLine = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}) (.+)$`)

type reviewEntry struct {
	at      time.Time
	summary string
}

func reviewEntries(note *model.Note) []reviewEntry {
	entries := []reviewEntry{{at: note.Updated, summary: note.Summary}}
	for _, line := range strings.Split(note.Body, "\n") {
		match := reviewHistoryLine.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		if at, err := time.ParseInLocation("2006-01-02 15:04", match[1], time.Local); err == nil {
			entries = append(entries, reviewEntry{at: at, summary: match[2]})
		}
	}
	return entries
}

func noteFact(note *model.Note) string {
	lines := []string{"- " + note.Summary}
	for _, line := range strings.Split(note.Body, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, "  "+trimmed)
		}
	}
	return strings.Join(lines, "\n")
}

func BuildFacts(week Week, sources Sources) string {
	var completed, started []string
	var reviews []reviewEntry
	for _, note := range sources.Notes {
		if note.Source == model.SourcePRReview {
			for _, entry := range reviewEntries(note) {
				if week.Contains(entry.at) {
					reviews = append(reviews, entry)
				}
			}
			continue
		}
		if !bragNoteSources[note.Source] {
			continue
		}
		finished := note.Status == model.StatusDone || note.Status == model.StatusArchived
		switch {
		case finished && week.Contains(note.Updated):
			completed = append(completed, noteFact(note))
		case !finished && week.Contains(note.Created):
			started = append(started, noteFact(note))
		}
	}
	sort.SliceStable(reviews, func(i, j int) bool { return reviews[i].at.Before(reviews[j].at) })

	var sections []string
	addSection := func(heading string, lines []string) {
		if len(lines) > 0 {
			sections = append(sections, "### "+heading+"\n"+strings.Join(lines, "\n"))
		}
	}
	addSection("Completed", completed)
	addSection("Started", started)
	var reviewLines []string
	for _, entry := range reviews {
		reviewLines = append(reviewLines, fmt.Sprintf("- %s (%s)", entry.summary, entry.at.Format("Mon 02 Jan 15:04")))
	}
	addSection("PR reviews", reviewLines)
	addSection("PRs opened", pullRequestLines(sources.Opened))
	addSection("PRs merged", pullRequestLines(sources.Merged))
	if commitSection := commitLines(sources.Commits); commitSection != "" {
		sections = append(sections, "### Commits\n"+commitSection)
	}
	if len(sections) == 0 {
		return noActivityFacts
	}
	return strings.Join(sections, "\n\n")
}

func pullRequestLines(prs []review.QueuedPR) []string {
	var lines []string
	for _, pr := range prs {
		lines = append(lines, fmt.Sprintf("- %s#%d %s (%s)", pr.Ref.Repo, pr.Ref.Number, pr.Title, pr.State))
	}
	return lines
}

func commitLines(commitsByRepo map[string][]sourcecontrol.PRItem) string {
	repos := make([]string, 0, len(commitsByRepo))
	for repo, commits := range commitsByRepo {
		if len(commits) > 0 {
			repos = append(repos, repo)
		}
	}
	sort.Strings(repos)
	var blocks []string
	for _, repo := range repos {
		lines := []string{"#### " + repo}
		for _, commit := range commitsByRepo[repo] {
			lines = append(lines, "- "+strings.TrimPrefix(commit.Title, "commit "))
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func Gather(ctx context.Context, cfg *config.Config, week Week, notes []*model.Note, now time.Time) (Sources, error) {
	until := week.End().Add(-time.Second)
	if now.Before(until) {
		until = now
	}
	if !cfg.GitEnabled() {
		return Sources{Notes: notes}, nil
	}
	sources := Sources{Notes: notes, Commits: sourcecontrol.FetchLocalCommitsBetween(ctx, cfg, week.Start, until)}
	var err error
	sources.Opened, sources.Merged, err = sourcecontrol.NewEngine(cfg).AuthoredPRs(ctx, week.Start, week.End())
	return sources, err
}
