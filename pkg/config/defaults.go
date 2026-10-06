package config

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

const DefaultWeekBragPrompt = `Below are facts about my work for one ISO week. They are the only source of truth; never invent work, metrics or people.
Write a concise, impact-focused brag summary as grouped bullet points under a '## Summary' heading. Do not repeat the facts verbatim. Output only markdown.`

const DefaultMonthBragPrompt = `Below are my weekly brags (summaries and facts) for one month. They are the only source of truth; never invent work, metrics or people.
Write a concise monthly brag under a '## Summary' heading: 3-6 themed groups of bullets with outcomes, my role and evidence (repos, PRs, dates). Output only markdown.`

const DefaultPerformanceReviewPrompt = `You are helping a software engineer write their annual performance self-review.

Below are monthly brag summaries for one year. Treat them as the only source of truth; never invent work, metrics or people.

Write markdown with these sections:
## Summary — 3-4 sentences on overall impact this year.
## Key accomplishments (the what) — 4-6 items, each: outcome, my role, scope, measurable impact, evidence (PRs/repos/dates from the input).
## How I worked (the how) — behaviours with examples: ownership, customer focus, collaboration/code review, quality & operational excellence, written communication.
## Technical depth & craft — notable design, reliability, tooling or migration work.
## Helping others — reviews, unblocking, mentoring.
## Growth areas — 2-3 honest areas with a concrete plan.
## Goals for next year — 3 goals tied to the above.

Use first person, past tense, concise bullets. Output only markdown.`

const DefaultReviewCommand = `claude -p "/review-toolkit:review {url} --emit findings-json --emit-to {findings}" --permission-mode auto`

const DefaultRetentionDays = 14

const (
	DefaultPRQuantityPerRepo = 15
	maxPRQuantityPerRepo     = 100
)

func DefaultConfig() *Config {
	showDailyCommits := true
	return &Config{
		ShowDailyCommits:    &showDailyCommits,
		DigestRoot:          DefaultDigestRoot,
		GitRepositoryRoots:  []string{"~/Projects"},
		GitLookbackDays:     3,
		GitCommitsCmd:       `git log -n 50 --author="$(git config user.email)" --since="{since}" --until="{until}" --pretty="format:%h|%s"`,
		GitAutoSyncInterval: 600,
		JanitorPatterns:     append([]string(nil), DefaultJanitorPatterns...),
		ReviewCommand:       DefaultReviewCommand,
		GreenOnly:           true,
		PRQuantityPerRepo:   DefaultPRQuantityPerRepo,
		Jobs: []JobSpec{
			{
				Name:          "branch-reaper",
				DryRunCommand: "digest branch-reaper --root ~/Projects --dry-run",
				Command:       "digest branch-reaper --root ~/Projects",
			},
			{
				Name:          "janitor",
				DryRunCommand: "digest janitor --root ~ --dry-run",
				Command:       "digest janitor --root ~",
			},
		},
	}
}
