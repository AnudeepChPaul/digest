package model

import (
	"time"
)

type Status string

const (
	StatusInbox    Status = "inbox"
	StatusActive   Status = "active"
	StatusDone     Status = "done"
	StatusArchived Status = "archived"
)

type Source string

const (
	SourceManual       Source = "manual"
	SourceRepoSync     Source = "repo-sync"
	SourceJanitor      Source = "janitor"
	SourceBranchReaper Source = "branch-reaper"
	SourceStandup      Source = "standup"
	SourceJournal      Source = "journal"
	SourcePRReview     Source = "pr-review"
)

type Note struct {
	ID      string    `yaml:"id"`
	Created time.Time `yaml:"created"`
	Updated time.Time `yaml:"updated"`
	Status  Status    `yaml:"status"`
	Source  Source    `yaml:"source"`
	Summary string    `yaml:"summary"`
	Subject string    `yaml:"subject,omitempty"`
	Repo    string    `yaml:"repo,omitempty"`

	Body     string `yaml:"-"`
	FilePath string `yaml:"-"`
}
