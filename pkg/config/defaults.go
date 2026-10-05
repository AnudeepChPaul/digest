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

const DefaultReviewRoot = "~/digest/reviews"

const DefaultReviewCommand = `claude -p "/review-toolkit:review {url} --emit findings-json --emit-to {findings}" --permission-mode auto`

const (
	DefaultPRQuantityPerRepo = 15
	maxPRQuantityPerRepo     = 100
)

func DefaultConfig() *Config {
	showDailyCommits := true
	return &Config{
		ShowDailyCommits:    &showDailyCommits,
		NotesDir:            "~/digest/notes",
		GitRepositoryRoots:  []string{"~/Projects"},
		GitLookbackDays:     3,
		GitCommitsCmd:       `git log -n 50 --since="{since}" --until="{until}" --pretty=format:%h|%s`,
		GitAutoSyncInterval: 600,
		JanitorPatterns:     append([]string(nil), DefaultJanitorPatterns...),
		ReviewRoot:          DefaultReviewRoot,
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
