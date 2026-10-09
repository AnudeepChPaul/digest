package config

import (
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
	AutomationJira       = "jira"
	AutomationConfluence = "confluence"
	AutomationGoogleDoc  = "google doc"
	AutomationPRReview   = "pr review"
)

const DefaultReviewPrompt = "/review-toolkit:review {url} --emit findings-json"

const noInventionRule = "The note is the only source of truth; never invent people, dates or numbers."

func builtInAutomations() []AutomationSpec {
	return []AutomationSpec{
		{
			Name:    AutomationJira,
			Plugins: []string{"jira-inator"},
			Match:   []string{"create a ticket", "create ticket", "create a jira", "create jira", "jira ticket"},
			DraftPrompt: Prompt(joinPromptParts(
				"Draft one Jira ticket from the note below using the jira-inator plugin's jira-ticket-creator skill. "+noInventionRule,
				`Use the Jira project key the note names (for example PROJ or PROJ-123). If the note names no project key, do not output any JSON; print exactly one line starting with "NO PROJECT:" asking the user to add a project key to the note, then stop.`,
				"Read the project's create metadata with jira-inator (create_meta -p <PROJECT>) so issue type, priority and components use allowed values.",
				"Do NOT create, update, transition or comment on any ticket.",
				"Output only one JSON object with the keys project, issue_type, summary, priority, components (list), labels (list) and description (markdown).",
			)),
			CreatePrompt: Prompt(joinPromptParts(
				"Do not change its content and do not create anything else. Please feel free to expan the content by ~3 lines if you can.",
				`Output only one JSON object: {"kind": "ticket", "key": "<ticket key>", "url": "<ticket url>"}.`,
			)),
		},
		{
			Name:    AutomationConfluence,
			Plugins: []string{"confluence-inator"},
			Match:   []string{"create a confluence", "confluence page", "confluence doc"},
			DraftPrompt: Prompt(joinPromptParts(
				"Draft one Confluence page from the note below using the confluence-inator plugin. "+noInventionRule,
				`Use the Confluence space key the note names. If the note names no space key, do not output any JSON; print exactly one line starting with "NO SPACE:" asking the user to add a space key to the note, then stop.`,
				"Do NOT create or update any page.",
				"Output only one JSON object with the keys space, parent (a parent page title or id, only if the note names one), summary (the page title) and description (the page body as markdown).",
			)),
			CreatePrompt: Prompt(joinPromptParts(
				"Create the Confluence page from the draft below with the confluence-inator plugin, using its space, parent, summary as the title and description as the body. Do not change its content and do not create anything else.",
				`Output only one JSON object: {"kind": "doc", "key": "<page id>", "url": "<page url>"}.`,
			)),
		},
		{
			Name:    AutomationGoogleDoc,
			Plugins: []string{"claude_ai_Google_Drive"},
			Match:   []string{"create a google doc", "google doc"},
			DraftPrompt: Prompt(joinPromptParts(
				"Draft one Google Doc from the note below. "+noInventionRule,
				"Do NOT create any file.",
				"Output only one JSON object with the keys summary (the document title) and description (the document body as markdown).",
			)),
			CreatePrompt: Prompt(joinPromptParts(
				"Create one Google Doc from the draft below with the Google Drive connector's create_file tool, titled with summary and containing description. Do not change its content and do not create anything else.",
				`Output only one JSON object: {"kind": "doc", "key": "<file id>", "url": "<document url>"}.`,
			)),
		},
		{
			Name:    AutomationPRReview,
			Plugins: []string{"review-toolkit"},
			Prompt:  DefaultReviewPrompt,
		},
	}
}

const DefaultRetentionDays = 14

const (
	DefaultGitAutoSyncInterval = 600
	DefaultMorning             = "09:30"
	DefaultEvening             = "18:00"
)

func DefaultJobs() []JobSpec {
	return []JobSpec{
		{Name: "branch-reaper", DryRunCommand: "digest branch-reaper --root ~/Projects --dry-run", Command: "digest branch-reaper --root ~/Projects"},
		{Name: "janitor", DryRunCommand: "digest janitor --root ~ --dry-run", Command: "digest janitor --root ~"},
	}
}

const (
	DefaultPRQuantityPerRepo = 15
	maxPRQuantityPerRepo     = 100
)

const DefaultConfigYAML = `digest_root: ~/digest
# claude is the only supported tool; digest builds every claude -p command from it
ai_tool_type: claude
# notes_today | notes_yesterday; unset or empty section selects the first item
selection_default: notes_today
# false hides every git section and stops all git and GitHub fetching
show_git: true
git_repository_roots: []
# days that count for your streak and get summaries; weeks always run Monday to Sunday
work_days: [mon, tue, wed, thu, fri]
# morning and evening summary notifications (24h HH:MM); empty turns one off
digest_notifications:
  morning: "09:30"
  evening: "18:00"
# a small key hint under the selected row after a moment of idle
show_key_hints: false
# terminal app opened when you click a notification
terminal_app: ""
# selected | always; @ action tags and PR tags always show, other tags only on the selected row unless always
show_tags: selected
pr_quantity_per_repo: 15
show_daily_commits: true
git_auto_sync_interval: 600
green_only: true
jira_base_url: ""
retention_days: 14

janitor_patterns:
  - java_error_in_*.log
  - hs_err_pid*.log
  - jbr_err_pid*.log
  - "*.hprof"
  - java[0-9].log
  - "*.pre-dump.bak"
  - GlobalProtectLogs*.tgz
jobs:
  - name: branch-reaper
    dry-run-command: "digest branch-reaper --root ~/Projects --dry-run"
    command: "digest branch-reaper --root ~/Projects"
  - name: janitor
    dry-run-command: "digest janitor --root ~ --dry-run"
    command: "digest janitor --root ~"

# empty prompts use digest's built-in defaults
brag_prompts: ""
month_brag_prompts: ""
performance_review_prompts: ""

automations:
  # Jira ticket; only jira-inator is allowed, its token is checked when installed
  - name: jira
    plugins:
      - jira-inator
    match:
      - create a ticket
      - create ticket
      - create a jira
      - create jira
      - jira ticket
    draft_prompt: ""
    create_prompt: ""
  # Confluence page; only confluence-inator is allowed, its token is checked when installed
  - name: confluence
    plugins:
      - confluence-inator
    match:
      - create a confluence
      - confluence page
      - confluence doc
    draft_prompt: ""
    create_prompt: ""
  # Google Doc; only the claude.ai Google Drive connector is allowed
  - name: google doc
    plugins:
      - claude_ai_Google_Drive
    match:
      - create a google doc
      - google doc
    draft_prompt: ""
    create_prompt: ""
  # PR review; never offered on @, run by digest for pending PRs
  - name: pr review
    plugins:
      - review-toolkit
    prompt: ""
`

func DefaultConfig() *Config {
	var cfg Config
	if err := yaml.Unmarshal([]byte(DefaultConfigYAML), &cfg); err != nil {
		panic("default config template is invalid: " + err.Error())
	}
	return &cfg
}
