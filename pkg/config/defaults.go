package config

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var DefaultJanitorPatterns = []string{
	"java_error_in_*.log",
	"hs_err_pid*.log",
	"jbr_err_pid*.log",
	"*.hprof",
	"java[0-9].log",
	"*.pre-dump.bak",
	"GlobalProtectLogs*.tgz",
}

const DefaultDigestRoot = "~/digest"

const DefaultBragCommand = `claude -p --tools "" --setting-sources "" --strict-mcp-config --permission-prompts none --disable-slash-commands`

func joinPromptParts(parts ...string) string {
	return strings.Join(parts, "\n\n")
}

var DefaultWeekBragPrompt = joinPromptParts(
	"Below are facts about my work for one ISO week. They are the only source of truth; never invent work, metrics or people.",
	"If required, you can always refer to PR description for more knowledge.",
	"Write a concise, strong impact-focused brag summary as grouped bullet points under a '## Summary' heading. Do not repeat the facts verbatim. Output only markdown.",
)

var DefaultMonthBragPrompt = joinPromptParts(
	"Below are my weekly brags (summaries and facts) for one month. They are the only source of truth; never invent work, metrics or people.",
	"Write a concise monthly brag under a '## Summary' heading, with 3-6 themed groups of bullets covering outcomes, my role and evidence (repos, PRs, dates). Output only markdown.",
)

var DefaultPerformanceReviewPrompt = joinPromptParts(
	`Never ever use the word "brag" anywhere.`,
	"Below are monthly brag summaries for one year. Treat them as the only source of truth; never invent work, metrics or people.",
	`Write markdown with these sections:
## Summary — ~8 sentences on overall impact this year.
## Key accomplishments (the what) — ~8 items, each: outcome, my role, scope, measurable impact, evidence (PRs/repos/dates from the input).
## How I worked (the how) — behaviours with examples: ownership, customer focus, collaboration/code review, quality & operational excellence, written communication.
## Technical depth & craft — notable design, reliability, tooling or migration work.
## Helping others — reviews, unblocking, mentoring.
## Growth areas — 2-3 honest areas with a concrete plan.
## Goals for next year — 3 goals tied to the above.`,
	"Use first person, past tense, concise bullets. Output only markdown.",
)

const (
	AutomationJira           = "jira"
	AutomationConfluence     = "confluence"
	AutomationGoogleDoc      = "google doc"
	AutomationGoogleCalendar = "google calendar"
	AutomationPRReview       = "pr review"
)

var builtInAutomationSpecs = func() []AutomationSpec {
	var template struct {
		Automations []AutomationSpec `yaml:"automations"`
	}
	if err := yaml.Unmarshal([]byte(DefaultConfigYAML), &template); err != nil {
		panic("default config template is invalid: " + err.Error())
	}
	return template.Automations
}()

func builtInAutomations() []AutomationSpec {
	specs := make([]AutomationSpec, len(builtInAutomationSpecs))
	for index, spec := range builtInAutomationSpecs {
		spec.Plugins, spec.Match = slices.Clone(spec.Plugins), slices.Clone(spec.Match)
		specs[index] = spec
	}
	return specs
}

const (
	DefaultJanitorGraceDays      = 14
	DefaultBranchReaperGraceDays = 7
)

const (
	DefaultGitAutoSyncInterval = 600
	DefaultMorning             = "09:30"
	DefaultEvening             = "18:00"
)

func builtInJob(name string, options map[string]any) JobSpec {
	return JobSpec{Name: name, Command: "digest " + name, DryRunCommand: "digest " + name + " --dry-run", Options: options}
}

func DefaultJobs() []JobSpec {
	return []JobSpec{
		builtInJob(JobBranchReaper, map[string]any{OptionGraceDays: DefaultBranchReaperGraceDays, OptionRoots: []string{"~/Projects"}}),
		builtInJob(JobJanitor, map[string]any{OptionGraceDays: DefaultJanitorGraceDays, OptionPatterns: slices.Clone(DefaultJanitorPatterns), OptionRoots: []string{"~"}}),
		builtInJob(JobRepoSync, map[string]any{OptionRoots: []string{"~/Projects"}}),
	}
}

func BuiltInJobName(name string) (string, bool) {
	canonical := strings.Map(func(character rune) rune {
		if character == ' ' || character == '_' {
			return '-'
		}
		return character
	}, strings.ToLower(strings.TrimSpace(name)))
	switch canonical {
	case JobBranchReaper, JobJanitor, JobRepoSync:
		return canonical, true
	}
	return "", false
}

func mergeJobs(configured []JobSpec) ([]JobSpec, error) {
	jobs := DefaultJobs()
	for _, job := range configured {
		name, builtIn := BuiltInJobName(job.Name)
		if !builtIn {
			jobs = append(jobs, job)
			continue
		}
		if job.Command != "" || job.DryRunCommand != "" {
			return nil, fmt.Errorf("job %s: command and dry-run-command are built in; only options can be set; run digest migrate", job.Name)
		}
		target := &jobs[slices.IndexFunc(jobs, func(candidate JobSpec) bool { return candidate.Name == name })]
		for option, value := range job.Options {
			for existing := range target.Options {
				if OptionEnvName(existing) == OptionEnvName(option) {
					delete(target.Options, existing)
				}
			}
			target.Options[option] = value
		}
	}
	return jobs, nil
}

const (
	DefaultPRQuantityPerRepo = 15
	maxPRQuantityPerRepo     = 100
)

func DefaultConfig() *Config {
	var cfg Config
	if err := yaml.Unmarshal([]byte(DefaultConfigYAML), &cfg); err != nil {
		panic("default config template is invalid: " + err.Error())
	}
	return &cfg
}
