package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var ErrReviewedCommitGone = errors.New("reviewed commit is no longer in the PR history")

const compareJQ = `{commits: [.commits[] | {sha, message: .commit.message, author: (.author.login // .commit.author.name), date: .commit.author.date}], files: [(.files // [])[] | {filename, status, additions, deletions}]}`

var runGHAPI = func(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", append([]string{"api"}, args...)...)
	cmd.WaitDelay = commandWaitDelay
	return cmd.CombinedOutput()
}

type CompareCommit struct {
	SHA      string
	Headline string
	Author   string
	Date     time.Time
}

type CompareFile struct {
	Path      string
	Status    string
	Additions int
	Deletions int
}

type Comparison struct {
	Commits []CompareCommit
	Files   []CompareFile
}

func CompareSince(ctx context.Context, ref PRRef, baseSHA, headSHA string) (Comparison, error) {
	output, err := runGHAPI(ctx, "--hostname", ref.Host, fmt.Sprintf("repos/%s/%s/compare/%s...%s", ref.Owner, ref.Repo, baseSHA, headSHA), "--jq", compareJQ)
	if err != nil {
		if strings.Contains(string(output), "HTTP 404") {
			return Comparison{}, ErrReviewedCommitGone
		}
		return Comparison{}, fmt.Errorf("compare %s...%s: %w: %s", baseSHA, headSHA, err, strings.TrimSpace(string(output)))
	}
	var decoded struct {
		Commits []struct {
			SHA     string    `json:"sha"`
			Message string    `json:"message"`
			Author  string    `json:"author"`
			Date    time.Time `json:"date"`
		} `json:"commits"`
		Files []struct {
			Filename  string `json:"filename"`
			Status    string `json:"status"`
			Additions int    `json:"additions"`
			Deletions int    `json:"deletions"`
		} `json:"files"`
	}
	if err := json.Unmarshal(output, &decoded); err != nil {
		return Comparison{}, fmt.Errorf("decode compare: %w", err)
	}
	plainTextFields(&decoded)
	comparison := Comparison{Commits: make([]CompareCommit, 0, len(decoded.Commits)), Files: make([]CompareFile, 0, len(decoded.Files))}
	for _, commit := range decoded.Commits {
		headline, _, _ := strings.Cut(commit.Message, "\n")
		comparison.Commits = append(comparison.Commits, CompareCommit{SHA: commit.SHA, Headline: headline, Author: commit.Author, Date: commit.Date})
	}
	for _, file := range decoded.Files {
		comparison.Files = append(comparison.Files, CompareFile{Path: file.Filename, Status: file.Status, Additions: file.Additions, Deletions: file.Deletions})
	}
	return comparison, nil
}
