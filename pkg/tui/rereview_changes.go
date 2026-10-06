package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"app/pkg/review"
	"app/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	changesCommitLimit = 50
	changesFileLimit   = 100
)

var compareSince = review.CompareSince

type changesSinceReview struct {
	baseSHA    string
	headSHA    string
	loading    bool
	comparison review.Comparison
	err        error
}

type changesSinceReviewMsg struct {
	url     string
	baseSHA string
	headSHA string
	result  review.Comparison
	err     error
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (m Model) reviewedSHAFor(pr *review.QueuedPR) string {
	meta, err := review.ReadMeta(review.StateDir(m.reviewRoot(), pr.Ref))
	if err != nil || meta.HeadSHA == pr.HeadSHA {
		return ""
	}
	return meta.HeadSHA
}

func (m *Model) changesSinceReviewCmd(item *GitPRItem) tea.Cmd {
	if item == nil || item.PR == nil || item.Kind != sourcecontrol.ReReviewKind || item.PR.HeadSHA == "" {
		return nil
	}
	baseSHA := m.reviewedSHAFor(item.PR)
	if baseSHA == "" {
		return nil
	}
	ref, headSHA := item.PR.Ref, item.PR.HeadSHA
	if entry, cached := m.changesSince[ref.URL]; cached && entry.baseSHA == baseSHA && entry.headSHA == headSHA {
		return nil
	}
	if m.changesSince == nil {
		m.changesSince = make(map[string]changesSinceReview)
	}
	m.changesSince[ref.URL] = changesSinceReview{baseSHA: baseSHA, headSHA: headSHA, loading: true}
	if m.mode == ViewPreview {
		m.updatePreviewViewport()
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		result, err := compareSince(ctx, ref, baseSHA, headSHA)
		return changesSinceReviewMsg{url: ref.URL, baseSHA: baseSHA, headSHA: headSHA, result: result, err: err}
	}
}

func (m Model) handleChangesSinceReview(msg changesSinceReviewMsg) (tea.Model, tea.Cmd) {
	entry, tracked := m.changesSince[msg.url]
	if !tracked || entry.baseSHA != msg.baseSHA || entry.headSHA != msg.headSHA {
		return m, nil
	}
	listed := make(map[string]bool, len(m.ghPendingPRs))
	for _, item := range m.ghPendingPRs {
		listed[item.URL] = true
	}
	for url := range m.changesSince {
		if !listed[url] && url != msg.url {
			delete(m.changesSince, url)
		}
	}
	m.changesSince[msg.url] = changesSinceReview{baseSHA: msg.baseSHA, headSHA: msg.headSHA, comparison: msg.result, err: msg.err}
	if item := m.currentPRItem(); m.mode == ViewPreview && item != nil && item.URL == msg.url {
		m.updatePreviewViewport()
	}
	return m, nil
}

func (m Model) changesSinceReviewMarkdown(item *GitPRItem) string {
	if item.PR == nil || item.Kind != sourcecontrol.ReReviewKind {
		return ""
	}
	entry, tracked := m.changesSince[item.PR.Ref.URL]
	if !tracked || entry.headSHA != item.PR.HeadSHA {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Changes since your review (%s → %s)\n\n", shortSHA(entry.baseSHA), shortSHA(entry.headSHA))
	switch {
	case entry.loading:
		b.WriteString("Loading…\n")
		return b.String()
	case errors.Is(entry.err, review.ErrReviewedCommitGone):
		b.WriteString("The reviewed commit is no longer in the PR history (force-push?). Press **r** on the Review tab to review again.\n")
		return b.String()
	case entry.err != nil:
		fmt.Fprintf(&b, "Could not load the changes: %s\n", entry.err)
		return b.String()
	}
	additions, deletions := 0, 0
	for _, file := range entry.comparison.Files {
		additions += file.Additions
		deletions += file.Deletions
	}
	fmt.Fprintf(&b, "**%s · %s · +%d / -%d**\n\n### Commits\n\n", plural(len(entry.comparison.Commits), "commit"), plural(len(entry.comparison.Files), "file"), additions, deletions)
	now := time.Now()
	for index, commit := range entry.comparison.Commits {
		if index == changesCommitLimit {
			fmt.Fprintf(&b, "- … and %d more\n", len(entry.comparison.Commits)-changesCommitLimit)
			break
		}
		fmt.Fprintf(&b, "- `%s` %s — %s, %s ago\n", shortSHA(commit.SHA), commit.Headline, commit.Author, shortAge(now.Sub(commit.Date)))
	}
	b.WriteString("\n### Files\n\n")
	for index, file := range entry.comparison.Files {
		if index == changesFileLimit {
			fmt.Fprintf(&b, "- … and %d more\n", len(entry.comparison.Files)-changesFileLimit)
			break
		}
		fmt.Fprintf(&b, "- `%s` (%s) +%d / -%d\n", file.Path, file.Status, file.Additions, file.Deletions)
	}
	return b.String()
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
