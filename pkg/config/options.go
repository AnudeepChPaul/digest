package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

const (
	JobBranchReaper = "branch-reaper"
	JobJanitor      = "janitor"
	JobRepoSync     = "repo-sync"
	OptionGraceDays = "grace_days"
	OptionPatterns  = "patterns"
	OptionRoots     = "roots"

	OptionNoQuarantine = "no_quarantine"

	optionEnvPrefix = "DIGEST_OPTION_"
)

func OptionEnvName(option string) string {
	upper := strings.Map(func(character rune) rune {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') {
			return character
		}
		return '_'
	}, strings.ToUpper(option))
	return optionEnvPrefix + upper
}

func formatOptionValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []string:
		return strings.Join(typed, ",")
	case []any:
		parts := make([]string, len(typed))
		for index, part := range typed {
			if part == nil {
				part = "~"
			}
			parts[index] = fmt.Sprint(part)
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(typed)
	}
}

func (j JobSpec) OptionsEnv() []string {
	var env []string
	for name, value := range j.Options {
		env = append(env, OptionEnvName(name)+"="+formatOptionValue(value))
	}
	slices.Sort(env)
	return env
}

func (j JobSpec) option(option string) string {
	wanted := OptionEnvName(option)
	for name, value := range j.Options {
		if OptionEnvName(name) == wanted {
			return formatOptionValue(value)
		}
	}
	return ""
}

func (c *Config) JobOption(jobName, option string) string {
	if fromEnv := strings.TrimSpace(os.Getenv(OptionEnvName(option))); fromEnv != "" {
		return fromEnv
	}
	if c == nil {
		return ""
	}
	for _, job := range c.Jobs {
		if job.Name == jobName {
			return strings.TrimSpace(job.option(option))
		}
	}
	return ""
}

func (c *Config) JobOptionInt(jobName, option string, fallback int) (int, error) {
	raw := c.JobOption(jobName, option)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s option %s: %q is not a whole number of days", jobName, option, raw)
	}
	return value, nil
}

func (c *Config) JobOptionBool(jobName, option string) (bool, error) {
	raw := c.JobOption(jobName, option)
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s option %s: %q is not true or false", jobName, option, raw)
	}
	return value, nil
}

func (c *Config) JobOptionList(jobName, option string, fallback []string) []string {
	var values []string
	for _, part := range strings.Split(c.JobOption(jobName, option), ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return values
}

func (c *Config) Retention() int {
	days, err := c.JobOptionInt(JobJanitor, OptionGraceDays, DefaultJanitorGraceDays)
	if err != nil {
		return DefaultJanitorGraceDays
	}
	return days
}
