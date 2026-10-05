package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"app/pkg/config"
	"app/pkg/jobs"
	"app/pkg/paths"
	"app/pkg/review"
	"app/pkg/tui"

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

func printUsage() {
	fmt.Println("Usage: digest [flags] [command] [command flags]")
	fmt.Println("\nCommands:")
	fmt.Println("  tui            Launch the interactive terminal dashboard (default)")
	fmt.Println("  repo-sync      Run background git repository synchronization")
	fmt.Println("  janitor        Scan and clean up quarantined/temp files")
	fmt.Println("  branch-reaper  Scan and prune stale local git branches")
	fmt.Println("  pr-review      Fresh-clone a pull request and run a Claude review (--url <pr url>)")
	fmt.Println("\nCommand flags:")
	fmt.Println("  --root <dir>   Root directory to scan; repeatable")
	fmt.Println("                 (default: ~/Projects for repo-sync and branch-reaper, current directory for janitor)")
	fmt.Println("  --dry-run      Report what would change without applying it")
	fmt.Println("  --review-root  Janitor: review clone root to reap merged/closed PRs (default: review_root)")
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

	cfg, configErr := config.LoadOrCreate(*configPath)
	if configErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", configErr)
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
		if err := tui.Run(cfg, configErr); err != nil {
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
		reviewRoot := fs.String("review-root", cfg.ReviewRootDir(), "Review clone root; clones of merged or closed PRs are removed")
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
		needsAction, err := jobs.RunJanitor(targetRoots, patterns, *reviewRoot, *dryRun)
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
		err = review.Run(context.Background(), ref, cfg.ReviewRootDir(), cfg.ReviewCommandTemplate(), log.New(os.Stderr))
		exitForJob("pr-review", false, err)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmdName)
		printUsage()
		os.Exit(1)
	}
}
