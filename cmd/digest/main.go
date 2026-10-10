package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/achandrapaul/digest/pkg/automation"
	"github.com/achandrapaul/digest/pkg/brag"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/doctor"
	"github.com/achandrapaul/digest/pkg/habit"
	"github.com/achandrapaul/digest/pkg/install"
	"github.com/achandrapaul/digest/pkg/jobs"
	"github.com/achandrapaul/digest/pkg/migrate"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/notify"
	"github.com/achandrapaul/digest/pkg/review"
	"github.com/achandrapaul/digest/pkg/store"
	"github.com/achandrapaul/digest/pkg/system"
	"github.com/achandrapaul/digest/pkg/tui"

	"github.com/charmbracelet/log"
)

type rootFlags []string

func (r *rootFlags) String() string {
	return strings.Join(*r, ",")
}

func (r *rootFlags) Set(value string) error {
	*r = append(*r, value)
	return nil
}

func jobRoots(cfg *config.Config, jobName string, roots rootFlags) []string {
	if len(roots) > 0 {
		return roots
	}
	var fallback []string
	for _, job := range config.DefaultJobs() {
		if job.Name == jobName {
			fallback, _ = job.Options[config.OptionRoots].([]string)
		}
	}
	return cfg.JobOptionList(jobName, config.OptionRoots, fallback)
}

func janitorSettings(cfg *config.Config) ([]string, int, error) {
	patterns := cfg.JobOptionList(config.JobJanitor, config.OptionPatterns, config.DefaultJanitorPatterns)
	graceDays, err := cfg.JobOptionInt(config.JobJanitor, config.OptionGraceDays, config.DefaultJanitorGraceDays)
	return patterns, graceDays, err
}

func branchReaperGraceDays(cfg *config.Config) (int, error) {
	return cfg.JobOptionInt(config.JobBranchReaper, config.OptionGraceDays, config.DefaultBranchReaperGraceDays)
}

func exitForJob(jobName string, needsAction bool, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running %s: %v\n", jobName, err)
		os.Exit(1)
	}
	if needsAction {
		os.Exit(1)
	}
}

func runNotificationsCommand(cfg *config.Config, action string, args []string) error {
	if len(args) != 1 || args[0] != "notifications" {
		return fmt.Errorf("usage: digest %s notifications", action)
	}
	if action == "uninstall" {
		if err := notify.Uninstall(); err != nil {
			return err
		}
		fmt.Println("Removed " + notify.LaunchAgentPath())
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if err := notify.Install(executable, filepath.Join(cfg.LogsDir(), "notify.log")); err != nil {
		return err
	}
	fmt.Println("Installed " + notify.LaunchAgentPath())
	return nil
}

func runInstall(cfg *config.Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	installer := &install.Installer{
		Out:               os.Stdout,
		HomeDir:           home,
		Confirm:           install.ConfirmFromStdin(bufio.NewReader(os.Stdin)),
		ReadKey:           install.ReadRawKey,
		LookPath:          exec.LookPath,
		RunCommand:        install.RunInTerminal,
		TmuxServerRunning: install.TmuxServerRunning,
		InstallNotifications: func() error {
			return runNotificationsCommand(cfg, "install", []string{"notifications"})
		},
		MissingTools: install.MissingTools(doctor.Check(cfg, exec.LookPath)),
	}
	return installer.Run()
}

func janitorNoQuarantine(cfg *config.Config, flagValue bool) (bool, error) {
	if flagValue {
		return true, nil
	}
	return cfg.JobOptionBool(config.JobJanitor, config.OptionNoQuarantine)
}

func printUsage(out io.Writer) {
	usage := []string{
		"Usage: digest [flags] [command] [command flags]",
		"\nCommands:",
		"  tui            Launch the interactive terminal dashboard (default)",
		"  repo-sync      Run background git repository synchronization",
		"  janitor        Scan and clean up quarantined/temp files",
		"  branch-reaper  Scan and prune stale local git branches",
		"  pr-review      Fresh-clone a pull request and run a Claude review (--url <pr url>)",
		"  brag           Generate a brag (--week 2026-W40 | --month 2026-10 | --year 2026) [--regenerate]",
		"  automation     Draft or create a note's ticket (--note <id> --name <automation> --phase draft|create)",
		"  notify-due     Send due @notify reminders (run every minute by launchd)",
		"  migrate        Rename note files by status and give older and PR notes timestamp ids (quit the TUI first)",
		"  install        Install missing tools, notifications and a shortcut, asking before each",
		"  install notifications    Install the launchd agent that runs notify-due every minute",
		"  uninstall notifications  Remove that launchd agent",
		"  doctor         Check that every tool digest needs is installed",
		"  setup          Walk through the friendly setup again (git, work days, summaries, hints, notifications)",
		"  init           Write the default config.yaml into the config directory; --force backs up an existing one and rewrites it, --automations backs it up and replaces only its automations",
		"\nCommand flags:",
		"  --root <dir>   Root directory to scan; repeatable",
		"                 (default: ~/Projects for repo-sync and branch-reaper, current directory for janitor)",
		"  --dry-run      Report what would change without applying it (job commands and migrate)",
		"  --review-root  Janitor: review clone root to reap merged/closed PRs (default: <digest_root>/reviews)",
		"  --no-quarantine",
		"                 Janitor: delete matching files for good instead of quarantining them",
		"                 (or the janitor job option no_quarantine: true)",
		"\nExit codes for job commands: 0 clean, 1 user action needed or error",
		"\nFlags:",
		"  --config <path>  Path to custom YAML configuration file",
		"  --dry-run        Same as passing --dry-run to the command",
		"  -h, --help       Show this usage (also: digest help)",
	}
	fmt.Fprintln(out, strings.Join(usage, "\n"))
}

type invocation struct {
	configPath  string
	showHelp    bool
	dryRun      bool
	command     string
	commandArgs []string
}

func parseInvocation(flagSet *flag.FlagSet, args []string) (invocation, error) {
	flagSet.SetOutput(io.Discard)
	configPath := flagSet.String("config", "", "Path to custom YAML configuration file")
	helpFlag := flagSet.Bool("help", false, "Show command usage")
	globalDryRun := flagSet.Bool("dry-run", false, "Same as passing --dry-run to the command")
	if err := flagSet.Parse(args); errors.Is(err, flag.ErrHelp) {
		return invocation{showHelp: true, command: "tui"}, nil
	} else if err != nil {
		return invocation{}, err
	}
	parsed := invocation{configPath: *configPath, showHelp: *helpFlag, command: "tui"}
	if remaining := flagSet.Args(); len(remaining) > 0 {
		parsed.command = remaining[0]
		parsed.commandArgs = remaining[1:]
	}
	if parsed.command == "help" {
		parsed.showHelp = true
	}
	if *globalDryRun {
		parsed.commandArgs = append([]string{"--dry-run"}, parsed.commandArgs...)
	}
	if parsed.command == "tui" || parsed.command == "setup" {
		parsed.commandArgs = withoutDryRun(parsed.commandArgs)
	}
	parsed.dryRun = isDryRun(append([]string{parsed.command}, parsed.commandArgs...))
	return parsed, nil
}

func withoutDryRun(args []string) []string {
	var kept []string
	for _, arg := range args {
		if name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "="); !strings.HasPrefix(arg, "-") || name != "dry-run" {
			kept = append(kept, arg)
		}
	}
	return kept
}

func dryRunUnsupported(command string, args []string) (string, bool) {
	switch command {
	case "pr-review", "brag", "automation", "notify-due", "init":
		return command, true
	case "install", "uninstall":
		words := []string{command}
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				words = append(words, arg)
			}
		}
		return strings.Join(words, " "), true
	}
	return "", false
}

func exitOnFlagError(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, flag.ErrHelp) {
		printUsage(os.Stdout)
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
	printUsage(os.Stderr)
	os.Exit(1)
}

func newCommandFlags(name string) *flag.FlagSet {
	flagSet := flag.NewFlagSet(name, flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	return flagSet
}

func parseCommandFlags(flagSet *flag.FlagSet, args []string) []string {
	exitOnFlagError(flagSet.Parse(args))
	return flagSet.Args()
}

func loadStartupConfig(configPath string) (*config.Config, bool, error) {
	firstRun := !config.Exists(configPath)
	cfg, err := config.LoadOrDefault(configPath)
	return cfg, firstRun, err
}

func reportConfigError(out io.Writer, command string, configErr error) {
	if configErr != nil && command != "tui" {
		fmt.Fprintf(out, "Warning: %v\n", configErr)
	}
}

func prepareTUI(cfg *config.Config, configErr error, firstRun bool) (bool, error) {
	if firstRun {
		return true, configErr
	}
	if rootsErr := jobs.CheckGitRoots(cfg.GitEnabled(), cfg.GitRepositoryRoots); rootsErr != nil {
		gitOff := false
		cfg.ShowGit = &gitOff
		return false, errors.Join(configErr, rootsErr)
	}
	return false, configErr
}

func main() {
	parsed, err := parseInvocation(flag.NewFlagSet("digest", flag.ContinueOnError), os.Args[1:])
	exitOnFlagError(err)

	if parsed.showHelp {
		printUsage(os.Stdout)
		os.Exit(0)
	}

	dryRun := parsed.dryRun
	if unsupported, refused := dryRunUnsupported(parsed.command, parsed.commandArgs); dryRun && refused {
		fmt.Fprintf(os.Stderr, "%s doesn't support dry-run\n", unsupported)
		os.Exit(1)
	}
	cfg, firstRun, configErr := loadStartupConfig(parsed.configPath)
	reportConfigError(os.Stderr, parsed.command, configErr)
	system.Protect(cfg.Root(), cfg.ReviewRootDir(), cfg.LogsDir(), config.Path(parsed.configPath))
	if !dryRun {
		if err := cfg.TightenPermissions(parsed.configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	cmdName := parsed.command
	subArgs := parsed.commandArgs

	switch cmdName {
	case "tui":
		parseCommandFlags(newCommandFlags("tui"), subArgs)
		showSetup, startupErr := prepareTUI(cfg, configErr, firstRun)
		if err := tui.Run(cfg, startupErr, config.Path(parsed.configPath), showSetup); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
		}

	case "repo-sync":
		fs := newCommandFlags("repo-sync")
		var roots rootFlags
		fs.Var(&roots, "root", "Root directory for repo sync (repeatable, default the job's roots option, else ~/Projects)")
		dryRun := fs.Bool("dry-run", false, "Perform a dry run without applying changes")
		parseCommandFlags(fs, subArgs)

		targetRoots := jobRoots(cfg, config.JobRepoSync, roots)
		fmt.Printf("Running repo-sync against %s (dry-run: %v)...\n", strings.Join(targetRoots, ", "), *dryRun)
		needsAction, err := jobs.RunRepoSync(targetRoots, *dryRun)
		exitForJob("repo-sync", needsAction, err)

	case "janitor":
		fs := newCommandFlags("janitor")
		var roots rootFlags
		fs.Var(&roots, "root", "Root directory to scan/clean (repeatable, default the job's roots option, else ~)")
		dryRun := fs.Bool("dry-run", false, "Perform a dry run without moving files")
		reviewRoot := fs.String("review-root", cfg.ReviewRootDir(), "Review clone root; clones of merged or closed PRs, or idle for 7 days, are removed")
		noQuarantineFlag := fs.Bool("no-quarantine", false, "Delete matching files for good instead of quarantining them")
		parseCommandFlags(fs, subArgs)

		targetRoots := jobRoots(cfg, config.JobJanitor, roots)
		patterns, graceDays, err := janitorSettings(cfg)
		if err != nil {
			exitForJob("janitor", false, err)
		}
		noQuarantine, err := janitorNoQuarantine(cfg, *noQuarantineFlag)
		if err != nil {
			exitForJob("janitor", false, err)
		}

		fmt.Printf("Running janitor job against %s (dry-run: %v, no-quarantine: %v)...\n", strings.Join(targetRoots, ", "), *dryRun, noQuarantine)
		needsAction, err := jobs.RunJanitorJob(&jobs.JanitorJob{
			Roots:          targetRoots,
			Patterns:       patterns,
			QuarantineRoot: cfg.QuarantineDir(),
			GraceDays:      graceDays,
			ReviewRoot:     *reviewRoot,
			NoQuarantine:   noQuarantine,
		}, *dryRun)
		exitForJob("janitor", needsAction, err)

	case "branch-reaper":
		fs := newCommandFlags("branch-reaper")
		var roots rootFlags
		fs.Var(&roots, "root", "Root directory to check repositories (repeatable, default the job's roots option, else ~/Projects)")
		dryRun := fs.Bool("dry-run", false, "Perform a dry run without deleting branches")
		parseCommandFlags(fs, subArgs)

		targetRoots := jobRoots(cfg, config.JobBranchReaper, roots)
		graceDays, err := branchReaperGraceDays(cfg)
		if err != nil {
			exitForJob("branch-reaper", false, err)
		}
		fmt.Printf("Running branch-reaper job against %s (dry-run: %v)...\n", strings.Join(targetRoots, ", "), *dryRun)
		needsAction, err := jobs.RunBranchReaper(targetRoots, graceDays, *dryRun)
		exitForJob("branch-reaper", needsAction, err)

	case "pr-review":
		fs := newCommandFlags("pr-review")
		prURL := fs.String("url", "", "Pull request URL to review")
		parseCommandFlags(fs, subArgs)

		ref, err := review.ParsePRURL(*prURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Reviewing %s in %s...\n", ref.URL, review.CloneDir(cfg.ReviewRootDir(), ref))
		signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err = review.Run(signalCtx, ref, cfg.ReviewRootDir(), cfg.ReviewCommandTemplate(), log.New(os.Stderr))
		stopSignals()
		exitForJob("pr-review", false, err)

	case "brag":
		fs := newCommandFlags("brag")
		week := fs.String("week", "", "ISO week to brag about, e.g. 2026-W40")
		month := fs.String("month", "", "Month to brag about from saved weekly brags, e.g. 2026-10")
		year := fs.String("year", "", "Year for the performance review from saved monthly brags, e.g. 2026")
		regenerate := fs.Bool("regenerate", false, "Rewrite only the summary from the saved facts")
		parseCommandFlags(fs, subArgs)

		period, err := brag.PeriodFromFlags(*week, *month, *year)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Bragging about %s into %s...\n", period.Label(), period.Path(cfg.BragDir()))
		loadNotes := func() ([]*model.Note, error) { return store.New(cfg.NotesDir()).List() }
		signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err = brag.RunJob(signalCtx, cfg, period, *regenerate, loadNotes)
		stopSignals()
		if err == nil {
			fmt.Println("Done.")
		}
		exitForJob("brag", false, err)

	case "automation":
		fs := newCommandFlags("automation")
		noteID := fs.String("note", "", "Note id")
		name := fs.String("name", "", "Automation name from the config")
		phase := fs.String("phase", string(automation.PhaseDraft), "draft or create")
		parseCommandFlags(fs, subArgs)

		signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err := automation.RunJob(signalCtx, cfg, *noteID, *name, automation.Phase(*phase))
		stopSignals()
		if errors.Is(err, automation.ErrNeedsReauth) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(automation.ExitNeedsReauth)
		}
		exitForJob("automation", false, err)

	case "notify-due":
		parseCommandFlags(newCommandFlags("notify-due"), subArgs)
		now := time.Now()
		reminderErr := notify.RunDue(cfg.Root(), now)
		notes, summaryErr := store.New(cfg.NotesDir()).List()
		if summaryErr == nil {
			schedule := notify.SummarySchedule{Morning: cfg.DigestNotifications.Morning, Evening: cfg.DigestNotifications.Evening, Terminal: cfg.TerminalApp, IsWorkDay: cfg.IsWorkDay}
			summaryErr = notify.RunSummaries(cfg.Root(), schedule, habit.Gather(notes, now, cfg.IsWorkDay), now)
		}
		exitForJob("notify-due", false, errors.Join(reminderErr, summaryErr))

	case "install", "uninstall":
		subArgs = parseCommandFlags(newCommandFlags(cmdName), subArgs)
		if cmdName == "install" && len(subArgs) == 0 {
			exitForJob("install", false, runInstall(cfg))
		} else {
			exitForJob(cmdName+" notifications", false, runNotificationsCommand(cfg, cmdName, subArgs))
		}

	case "setup":
		parseCommandFlags(newCommandFlags("setup"), subArgs)
		if err := tui.Run(cfg, configErr, config.Path(parsed.configPath), true); err != nil {
			fmt.Fprintf(os.Stderr, "Error running setup: %v\n", err)
			os.Exit(1)
		}

	case "init":
		fs := newCommandFlags("init")
		force := fs.Bool("force", false, "Back up an existing config.yaml and rewrite it")
		automationsOnly := fs.Bool("automations", false, "Back up config.yaml and replace only its automations with the defaults")
		parseCommandFlags(fs, subArgs)
		writeConfig := config.Init
		if *automationsOnly {
			writeConfig = func(path string, _ bool) (string, string, error) { return config.InitAutomations(path) }
		}
		written, backup, err := writeConfig(parsed.configPath, *force)
		if backup != "" {
			fmt.Println("Backed up " + backup)
		}
		if err == nil && *automationsOnly && backup != "" {
			fmt.Println("Wrote the default automations to " + written)
		} else if err == nil {
			fmt.Println("Wrote " + written)
		}
		exitForJob("init", false, err)

	case "migrate":
		fs := newCommandFlags("migrate")
		migrateDryRun := fs.Bool("dry-run", false, "Print what a migration would change without changing anything")
		parseCommandFlags(fs, subArgs)
		prCreatedAt := func(url string) (time.Time, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			return review.FetchPRCreatedAt(ctx, url)
		}
		_, err := migrate.Run(migrate.Options{
			NotesDir:      cfg.NotesDir(),
			Root:          cfg.Root(),
			AutomationDir: cfg.AutomationDir(),
			TUIMarker:     cfg.TUIMarkerPath(),
			ConfigPath:    config.Path(parsed.configPath),
			Out:           os.Stdout,
			PRCreatedAt:   prCreatedAt,
			IsWorkDay:     cfg.IsWorkDay,
			DryRun:        *migrateDryRun,
		})
		exitForJob("migrate", false, err)

	case "doctor":
		parseCommandFlags(newCommandFlags("doctor"), subArgs)
		results := doctor.Check(cfg, exec.LookPath)
		fmt.Println(doctor.Format(results))
		if notify.Installed() {
			fmt.Println("\n@notify launchd agent installed: " + notify.LaunchAgentPath())
		} else {
			fmt.Println("\n@notify launchd agent not installed; run: digest install notifications")
		}
		if doctor.Failed(results) {
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmdName)
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

func isDryRun(args []string) bool {
	for _, arg := range args {
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && name == "dry-run" {
			return !hasValue || value == "true" || value == "1"
		}
	}
	return false
}
