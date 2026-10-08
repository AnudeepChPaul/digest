package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/automation"
	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/doctor"
	"github.com/AnudeepChPaul/digest/pkg/habit"
	"github.com/AnudeepChPaul/digest/pkg/install"
	"github.com/AnudeepChPaul/digest/pkg/jobs"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/notify"
	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/store"
	"github.com/AnudeepChPaul/digest/pkg/tui"

	"github.com/charmbracelet/log"
)

type rootFlags []string

func (r *rootFlags) String() string {
	return strings.Join(*r, ",")
}

func (r *rootFlags) Set(value string) error {
	*r = append(*r, paths.Expand(value))
	return nil
}

func rootsOrDefault(roots rootFlags, fallback string) []string {
	if len(roots) > 0 {
		return roots
	}
	return []string{paths.Expand(fallback)}
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

func printUsage() {
	fmt.Println("Usage: digest [flags] [command] [command flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  tui            Launch the interactive terminal dashboard (default)")
	fmt.Println("  repo-sync      Run background git repository synchronization")
	fmt.Println("  janitor        Scan and clean up quarantined/temp files")
	fmt.Println("  branch-reaper  Scan and prune stale local git branches")
	fmt.Println("  pr-review      Fresh-clone a pull request and run a Claude review (--url <pr url>)")
	fmt.Println("  brag           Generate a brag (--week 2026-W40 | --month 2026-10 | --year 2026) [--regenerate]")
	fmt.Println("  automation     Draft or create a note's ticket (--note <id> --name <automation> --phase draft|create)")
	fmt.Println("  notify-due     Send due @notify reminders (run every minute by launchd)")
	fmt.Println("  install        Install missing tools, notifications and a shortcut, asking before each")
	fmt.Println("  install notifications    Install the launchd agent that runs notify-due every minute")
	fmt.Println("  uninstall notifications  Remove that launchd agent")
	fmt.Println("  doctor         Check that every tool digest needs is installed")
	fmt.Println("  setup          Walk through the friendly setup again (git, work days, summaries, hints, notifications)")
	fmt.Println("  export config  Write the default config.yaml into the config directory (backs up an existing one)")
	fmt.Println("\nCommand flags:")
	fmt.Println("  --root <dir>   Root directory to scan; repeatable")
	fmt.Println("                 (default: ~/Projects for repo-sync and branch-reaper, current directory for janitor)")
	fmt.Println("  --dry-run      Report what would change without applying it")
	fmt.Println("  --review-root  Janitor: review clone root to reap merged/closed PRs (default: <digest_root>/reviews)")
	fmt.Println("\nExit codes for job commands: 0 clean, 1 user action needed or error")
	fmt.Println("\nFlags:")
	flag.PrintDefaults()
}

func main() {
	configPath := flag.String("config", "", "Path to custom YAML configuration file")
	helpFlag := flag.Bool("help", false, "Show command usage")
	flag.Parse()

	if *helpFlag {
		printUsage()
		os.Exit(0)
	}

	firstRun := !config.Exists(*configPath)
	dryRun := isDryRun(flag.Args())
	loadConfig := config.LoadOrCreate
	if dryRun {
		loadConfig = config.LoadOrDefault
	}
	cfg, configErr := loadConfig(*configPath)
	if configErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", configErr)
	}
	if !dryRun {
		if err := cfg.TightenPermissions(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	args := flag.Args()
	cmdName := "tui"
	var subArgs []string
	if len(args) > 0 {
		cmdName = args[0]
		subArgs = args[1:]
	}

	switch cmdName {
	case "tui":
		startupErr := configErr
		if !firstRun {
			if rootsErr := jobs.CheckGitRoots(cfg.GitEnabled(), cfg.GitRepositoryRoots); rootsErr != nil {
				gitOff := false
				cfg.ShowGit = &gitOff
				startupErr = errors.Join(configErr, rootsErr)
			}
		}
		if err := tui.Run(cfg, startupErr, config.Path(*configPath), firstRun); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
		}

	case "repo-sync":
		fs := flag.NewFlagSet("repo-sync", flag.ExitOnError)
		var roots rootFlags
		fs.Var(&roots, "root", "Root directory for repo sync (repeatable, default ~/Projects)")
		dryRun := fs.Bool("dry-run", false, "Perform a dry run without applying changes")
		_ = fs.Parse(subArgs)

		targetRoots := rootsOrDefault(roots, "~/Projects")
		fmt.Printf("Running repo-sync against %s (dry-run: %v)...\n", strings.Join(targetRoots, ", "), *dryRun)
		needsAction, err := jobs.RunRepoSync(targetRoots, *dryRun)
		exitForJob("repo-sync", needsAction, err)

	case "janitor":
		fs := flag.NewFlagSet("janitor", flag.ExitOnError)
		var roots rootFlags
		fs.Var(&roots, "root", "Root directory to scan/clean (repeatable, default current directory)")
		dryRun := fs.Bool("dry-run", false, "Perform a dry run without moving files")
		reviewRoot := fs.String("review-root", cfg.ReviewRootDir(), "Review clone root; clones of merged or closed PRs, or idle for 7 days, are removed")
		_ = fs.Parse(subArgs)

		currentDir, err := os.Getwd()
		if err != nil {
			currentDir = "."
		}
		targetRoots := rootsOrDefault(roots, currentDir)
		patterns := cfg.JanitorPatterns
		if len(patterns) == 0 {
			patterns = config.DefaultJanitorPatterns
		}

		fmt.Printf("Running janitor job against %s (dry-run: %v)...\n", strings.Join(targetRoots, ", "), *dryRun)
		needsAction, err := jobs.RunJanitor(targetRoots, patterns, *reviewRoot, cfg.QuarantineDir(), cfg.Retention(), *dryRun)
		exitForJob("janitor", needsAction, err)

	case "branch-reaper":
		fs := flag.NewFlagSet("branch-reaper", flag.ExitOnError)
		var roots rootFlags
		fs.Var(&roots, "root", "Root directory to check repositories (repeatable, default ~/Projects)")
		dryRun := fs.Bool("dry-run", false, "Perform a dry run without deleting branches")
		_ = fs.Parse(subArgs)

		targetRoots := rootsOrDefault(roots, "~/Projects")
		fmt.Printf("Running branch-reaper job against %s (dry-run: %v)...\n", strings.Join(targetRoots, ", "), *dryRun)
		needsAction, err := jobs.RunBranchReaper(targetRoots, *dryRun)
		exitForJob("branch-reaper", needsAction, err)

	case "pr-review":
		fs := flag.NewFlagSet("pr-review", flag.ExitOnError)
		prURL := fs.String("url", "", "Pull request URL to review")
		_ = fs.Parse(subArgs)

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
		fs := flag.NewFlagSet("brag", flag.ExitOnError)
		week := fs.String("week", "", "ISO week to brag about, e.g. 2026-W40")
		month := fs.String("month", "", "Month to brag about from saved weekly brags, e.g. 2026-10")
		year := fs.String("year", "", "Year for the performance review from saved monthly brags, e.g. 2026")
		regenerate := fs.Bool("regenerate", false, "Rewrite only the summary from the saved facts")
		_ = fs.Parse(subArgs)

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
		fs := flag.NewFlagSet("automation", flag.ExitOnError)
		noteID := fs.String("note", "", "Note id")
		name := fs.String("name", "", "Automation name from the config")
		phase := fs.String("phase", string(automation.PhaseDraft), "draft or create")
		_ = fs.Parse(subArgs)

		signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		err := automation.RunJob(signalCtx, cfg, *noteID, *name, automation.Phase(*phase))
		stopSignals()
		if errors.Is(err, automation.ErrNeedsReauth) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(automation.ExitNeedsReauth)
		}
		exitForJob("automation", false, err)

	case "notify-due":
		now := time.Now()
		reminderErr := notify.RunDue(cfg.Root(), now)
		notes, summaryErr := store.New(cfg.NotesDir()).List()
		if summaryErr == nil {
			schedule := notify.SummarySchedule{Morning: cfg.DigestNotifications.Morning, Evening: cfg.DigestNotifications.Evening, Terminal: cfg.TerminalApp, IsWorkDay: cfg.IsWorkDay}
			summaryErr = notify.RunSummaries(cfg.Root(), schedule, habit.Gather(notes, now, cfg.IsWorkDay), now)
		}
		exitForJob("notify-due", false, errors.Join(reminderErr, summaryErr))

	case "install", "uninstall":
		if cmdName == "install" && len(subArgs) == 0 {
			exitForJob("install", false, runInstall(cfg))
		} else {
			exitForJob(cmdName+" notifications", false, runNotificationsCommand(cfg, cmdName, subArgs))
		}

	case "setup":
		if err := tui.Run(cfg, configErr, config.Path(*configPath), true); err != nil {
			fmt.Fprintf(os.Stderr, "Error running setup: %v\n", err)
			os.Exit(1)
		}

	case "export":
		if len(subArgs) != 1 || subArgs[0] != "config" {
			fmt.Fprintln(os.Stderr, "Usage: digest export config")
			os.Exit(1)
		}
		written, backup, err := config.Export(*configPath)
		if backup != "" {
			fmt.Println("Backed up " + backup)
		}
		if err == nil {
			fmt.Println("Wrote " + written)
		}
		exitForJob("export config", false, err)

	case "doctor":
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
		printUsage()
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
