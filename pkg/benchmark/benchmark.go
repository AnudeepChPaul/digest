package benchmark

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/achandrapaul/digest/pkg/system"
)

const (
	MarkdownFile   = "docs/benchmark/benchmark.md"
	scenarioPrefix = "BenchmarkScenario"
	historyStart   = "<!-- history:start -->"
	historyEnd     = "<!-- history:end -->"
)

var cpuSuffix = regexp.MustCompile(`-\d+$`)

type Result struct {
	Name    string
	Metrics map[string]float64
}

type Run struct {
	Date    time.Time
	Version string
	Commit  string
	CPU     string
	Results []Result
}

func (r Run) metric(name, unit string) float64 {
	for _, result := range r.Results {
		if result.Name == name {
			return result.Metrics[unit]
		}
	}
	return 0
}

func ParseGoBench(output string) ([]Result, string) {
	var results []Result
	var cpu string
	for _, line := range strings.Split(output, "\n") {
		if value, found := strings.CutPrefix(line, "cpu: "); found {
			cpu = strings.TrimSpace(value)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasPrefix(fields[0], scenarioPrefix) {
			continue
		}
		if _, err := strconv.Atoi(fields[1]); err != nil {
			continue
		}
		name := cpuSuffix.ReplaceAllString(strings.TrimPrefix(fields[0], scenarioPrefix), "")
		metrics := map[string]float64{}
		for index := 2; index+1 < len(fields); index += 2 {
			value, err := strconv.ParseFloat(fields[index], 64)
			if err != nil {
				continue
			}
			metrics[fields[index+1]] = value
		}
		results = append(results, Result{Name: name, Metrics: metrics})
	}
	return results, cpu
}

func Record(root string, run Run) error {
	path := filepath.Join(root, filepath.FromSlash(MarkdownFile))
	existing, err := system.Read(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := system.MkdirAllWithMode(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return system.WriteWithMode(path, []byte(Markdown(run, string(existing))), 0o644)
}
