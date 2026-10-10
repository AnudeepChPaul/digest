package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"syscall"
	"time"

	"github.com/achandrapaul/digest/pkg/benchmark"
	"github.com/achandrapaul/digest/pkg/system"
)

const processStartRuns = 20

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || (args[0] != "run" && args[0] != "record") {
		return errors.New("usage: bench run|record [--sizes 1,100,1000,10000] [--benchtime 200ms] [--root DIR --version vX.Y.Z --commit HASH]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	sizes := flags.String("sizes", "1,100,1000,10000", "comma-separated note counts")
	benchtime := flags.String("benchtime", "200ms", "go test -benchtime per scenario")
	root := flags.String("root", ".", "repository root that holds "+benchmark.MarkdownFile)
	version := flags.String("version", "", "release version for the history column")
	commit := flags.String("commit", "", "commit hash for the history column")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if args[0] == "record" && *version == "" {
		return errors.New("record needs --version")
	}

	fmt.Fprintf(os.Stderr, "bench: measuring at %s notes…\n", *sizes)
	results, cpu, err := scenarioResults(*root, *sizes, *benchtime)
	if err != nil {
		return err
	}
	process, err := processStart(*root)
	if err != nil {
		return err
	}
	measured := benchmark.Run{Date: time.Now(), Version: *version, Commit: *commit, CPU: cpu, Results: append(results, process)}

	if args[0] == "run" {
		fmt.Print(benchmark.Markdown(measured, ""))
		return nil
	}
	if err := benchmark.Record(*root, measured); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "bench: recorded %s in %s\n", *version, filepath.Join(*root, benchmark.MarkdownFile))
	return nil
}

func scenarioResults(root, sizes, benchtime string) ([]benchmark.Result, string, error) {
	command := exec.Command("go", "-C", root, "test", "./pkg/tui", "-run", "^$", "-bench", "^BenchmarkScenario", "-benchmem", "-benchtime", benchtime, "-timeout", "60m")
	command.Env = append(os.Environ(), "DIGEST_BENCH_SIZES="+sizes)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		return nil, "", fmt.Errorf("go test: %w\n%s", err, output.String())
	}
	results, cpu := benchmark.ParseGoBench(output.String())
	if len(results) == 0 {
		return nil, "", fmt.Errorf("no benchmark results in:\n%s", output.String())
	}
	return results, cpu, nil
}

func processStart(root string) (benchmark.Result, error) {
	buildDir, err := system.MkdirTemp("", "digest-bench-bin-")
	if err != nil {
		return benchmark.Result{}, err
	}
	defer system.RemoveAll(buildDir)
	binary := filepath.Join(buildDir, "digest")
	build := exec.Command("go", "-C", root, "build", "-trimpath", "-o", binary, "./cmd/digest")
	if output, err := build.CombinedOutput(); err != nil {
		return benchmark.Result{}, fmt.Errorf("go build: %w\n%s", err, output)
	}
	var durations []float64
	var peakBytes float64
	for range processStartRuns {
		command := exec.Command(binary, "--help")
		started := time.Now()
		if err := command.Run(); err != nil {
			return benchmark.Result{}, fmt.Errorf("digest --help: %w", err)
		}
		durations = append(durations, float64(time.Since(started).Nanoseconds()))
		peakBytes = max(peakBytes, maxRSSBytes(command.ProcessState))
	}
	slices.Sort(durations)
	return benchmark.Result{Name: "Process/start", Metrics: map[string]float64{
		"ns/op":  durations[len(durations)/2],
		"rss-MB": peakBytes / (1 << 20),
	}}, nil
}

func maxRSSBytes(state *os.ProcessState) float64 {
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return float64(usage.Maxrss)
	}
	return float64(usage.Maxrss) * 1024
}
