package model

import (
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDone     Status = "done"
	StatusArchived Status = "archived"
)

func (status Status) Known() bool {
	return status == StatusActive || status == StatusDone || status == StatusArchived
}

const AutomationTicket = "ticket"

type Source string

const (
	SourceManual       Source = "manual"
	SourceRepoSync     Source = "repo-sync"
	SourceJanitor      Source = "janitor"
	SourceBranchReaper Source = "branch-reaper"
	SourceStandup      Source = "standup"
	SourceJournal      Source = "journal"
	SourcePRReview     Source = "pr-review"
	SourceMyPR         Source = "my-pr"
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
	Ref     string    `yaml:"ref,omitempty"`

	Automated string `yaml:"automated,omitempty"`

	Body     string `yaml:"-"`
	FilePath string `yaml:"-"`
}
