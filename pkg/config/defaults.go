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
	ghSearchPrefix = `gh api --hostname {host} -X GET search/issues --paginate -f per_page=100 -f sort=updated -f order=desc -f q='`
	ghSearchSuffix = `' --jq '.items[].html_url'`

	DefaultGHPendingPRs         = ghSearchPrefix + "is:pr is:open draft:false review-requested:@me {repos}" + ghSearchSuffix
	DefaultGHDirectRequestedPRs = ghSearchPrefix + "is:pr is:open draft:false user-review-requested:@me {repos}" + ghSearchSuffix
	DefaultGHRereviewPRs        = ghSearchPrefix + "is:pr is:open draft:false reviewed-by:@me -review-requested:@me {repos}" + ghSearchSuffix
	DefaultGHReviewedPRs        = ghSearchPrefix + "is:pr reviewed-by:@me updated:>={date} {repos}" + ghSearchSuffix
	DefaultGHPRDetails          = `gh pr view {url} --json number,title,url,state,isDraft,createdAt,updatedAt,additions,deletions,changedFiles,headRefOid,headRefName,reviewDecision,author,statusCheckRollup,commits,reviewRequests,latestReviews,comments,files`
)

func DefaultConfig() *Config {
	return &Config{
		NotesDir:             "~/digest/notes",
		GitRepositoryRoots:   []string{"~/Projects"},
		GitLookbackDays:      3,
		GitCommitsCmd:        `git log -n 50 --since="{since}" --until="{until}" --pretty=format:%h|%s`,
		GitAutoSyncInterval:  600,
		JanitorPatterns:      append([]string(nil), DefaultJanitorPatterns...),
		ReviewRoot:           DefaultReviewRoot,
		ReviewCommand:        DefaultReviewCommand,
		GreenOnly:            true,
		GHPendingPRs:         DefaultGHPendingPRs,
		GHDirectRequestedPRs: DefaultGHDirectRequestedPRs,
		GHRereviewPRs:        DefaultGHRereviewPRs,
		GHReviewedPRs:        DefaultGHReviewedPRs,
		GHPRDetails:          DefaultGHPRDetails,
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
