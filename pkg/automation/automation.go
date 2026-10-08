package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/aitool"
	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/paths"
	"github.com/AnudeepChPaul/digest/pkg/store"

	"gopkg.in/yaml.v3"
)

type Phase string

const (
	PhaseDraft  Phase = "draft"
	PhaseCreate Phase = "create"
)

const (
	KindTicket = "ticket"
	KindDoc    = "doc"
)

type Result struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	URL  string `json:"url"`
}

var ErrNeedsReauth = errors.New("needs re-auth")

var (
	commandTimeout   = 10 * time.Minute
	checkTimeout     = time.Minute
	commandWaitDelay = 2 * time.Second
)

var authFailurePattern = regexp.MustCompile(`(?i)\b401\b|unauthori[sz]ed|mcp__\w+__authenticate|authentication (required|failed)|(re-?)?authenticate to|invalid (api )?token|token (has )?(expired|is invalid|was revoked)|not (logged|signed) in`)

var draftKeyOrder = []string{"project", "issue_type", "summary", "priority", "components", "labels"}

func Match(specs []config.AutomationSpec, note *model.Note) (config.AutomationSpec, bool) {
	matched := MatchAll(specs, note)
	if len(matched) == 0 {
		return config.AutomationSpec{}, false
	}
	return matched[0], true
}

func MatchAll(specs []config.AutomationSpec, note *model.Note) []config.AutomationSpec {
	if note == nil {
		return nil
	}
	content := strings.ToLower(note.Summary + "\n" + note.Body)
	var matched []config.AutomationSpec
	for _, spec := range specs {
		if slices.ContainsFunc(spec.Match, func(phrase string) bool {
			phrase = strings.ToLower(strings.TrimSpace(phrase))
			return phrase != "" && strings.Contains(content, phrase)
		}) {
			matched = append(matched, spec)
		}
	}
	return matched
}

func Find(specs []config.AutomationSpec, name string) (config.AutomationSpec, bool) {
	for _, spec := range specs {
		if spec.Name == name {
			return spec, true
		}
	}
	return config.AutomationSpec{}, false
}

var runCommand = func(ctx context.Context, command, input string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.WaitDelay = commandWaitDelay
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return string(output), fmt.Errorf("%w: %s", err, message)
		}
	}
	return string(output), err
}

func runBounded(ctx context.Context, timeout time.Duration, command, input string) (string, error) {
	boundedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := runCommand(boundedCtx, command, input)
	if errors.Is(boundedCtx.Err(), context.DeadlineExceeded) {
		return output, fmt.Errorf("timed out after %s", timeout)
	}
	return output, err
}

var tokenChecks = aitool.TokenChecks

func needsReauth(spec config.AutomationSpec) error {
	return reauthError(spec, spec.ReauthHint)
}

func reauthError(spec config.AutomationSpec, hint string) error {
	message := hint
	if message == "" {
		message = "could not sign in to its tools"
	}
	return fmt.Errorf("%s %w: %s", spec.Name, ErrNeedsReauth, message)
}

func checkPluginTokens(ctx context.Context, spec config.AutomationSpec) error {
	for _, check := range tokenChecks(spec.Plugins) {
		_, err := runBounded(ctx, checkTimeout, check.Command, "")
		if err == nil {
			continue
		}
		hint := spec.ReauthHint
		if hint == "" {
			hint = check.ReauthHint
		}
		if hint == "" {
			hint = check.Plugin + " token check failed"
		}
		return reauthError(spec, hint)
	}
	return nil
}

func automationCommand(spec config.AutomationSpec) string {
	if spec.Command != "" {
		return spec.Command
	}
	return aitool.AutomationCommand(spec.Plugins)
}

func ensureAuthenticated(ctx context.Context, spec config.AutomationSpec) error {
	return checkPluginTokens(ctx, spec)
}

func noteInput(prompt string, note *model.Note) string {
	return prompt + "\n\n## Note\n\n" + note.Summary + "\n\n" + strings.TrimSpace(note.Body) + "\n"
}

func Execute(ctx context.Context, spec config.AutomationSpec, root string, note *model.Note, phase Phase) (Result, error) {
	var input string
	switch phase {
	case PhaseDraft:
		input = noteInput(string(spec.DraftPrompt), note)
	case PhaseCreate:
		draft, err := LoadDraft(root, note.ID)
		if err != nil {
			return Result{}, fmt.Errorf("no confirmed draft: %w", err)
		}
		input = string(spec.CreatePrompt) + "\n\n## Draft\n\n" + draft
	default:
		return Result{}, fmt.Errorf("unknown phase %q", phase)
	}
	if err := ensureAuthenticated(ctx, spec); err != nil {
		return Result{}, err
	}
	output, err := runBounded(ctx, commandTimeout, automationCommand(spec), input)
	if err != nil {
		if authFailurePattern.MatchString(output + err.Error()) {
			return Result{}, needsReauth(spec)
		}
		return Result{}, fmt.Errorf("%s %s: %w", spec.Name, phase, err)
	}
	if phase == PhaseDraft {
		return Result{}, saveDraftFromOutput(spec, root, note.ID, output)
	}
	return resultFromOutput(spec, output)
}

func lastJSONObject(output string) (map[string]any, bool) {
	end := strings.LastIndex(output, "}")
	for start := strings.LastIndex(output[:max(end, 0)], "{"); start >= 0 && end > start; start = strings.LastIndex(output[:start], "{") {
		var object map[string]any
		if json.Unmarshal([]byte(output[start:end+1]), &object) == nil {
			return object, true
		}
	}
	return nil, false
}

func saveDraftFromOutput(spec config.AutomationSpec, root, noteID, output string) error {
	fields, found := lastJSONObject(output)
	if !found {
		if authFailurePattern.MatchString(output) {
			return needsReauth(spec)
		}
		if lastLine := lastOutputLine(output); lastLine != "" {
			return fmt.Errorf("%s draft: Claude returned no JSON draft: %s", spec.Name, lastLine)
		}
		return fmt.Errorf("%s draft: Claude returned no JSON draft", spec.Name)
	}
	text, err := draftYAML(fields)
	if err != nil {
		return err
	}
	return SaveDraft(root, noteID, text)
}

func lastOutputLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func resultFromOutput(spec config.AutomationSpec, output string) (Result, error) {
	fields, found := lastJSONObject(output)
	if !found {
		if authFailurePattern.MatchString(output) {
			return Result{}, needsReauth(spec)
		}
		return Result{}, fmt.Errorf("%s create: Claude returned no JSON result", spec.Name)
	}
	kind, _ := fields["kind"].(string)
	key, _ := fields["key"].(string)
	url, _ := fields["url"].(string)
	result := Result{Kind: strings.ToLower(strings.TrimSpace(kind)), Key: strings.TrimSpace(key), URL: strings.TrimSpace(url)}
	if result.Kind != KindTicket && result.Kind != KindDoc {
		return Result{}, fmt.Errorf("%s create: kind must be %q or %q, got %q", spec.Name, KindTicket, KindDoc, kind)
	}
	if result.Key == "" && result.URL == "" {
		return Result{}, fmt.Errorf("%s create: Claude returned neither a key nor a url", spec.Name)
	}
	return result, nil
}

func draftYAML(fields map[string]any) (string, error) {
	var keys []string
	for key := range fields {
		if key != "description" && !slices.Contains(draftKeyOrder, key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	ordered := append(slices.Clone(draftKeyOrder), keys...)
	ordered = append(ordered, "description")
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range ordered {
		value, present := fields[key]
		if !present {
			continue
		}
		valueNode := &yaml.Node{}
		if err := valueNode.Encode(value); err != nil {
			return "", err
		}
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, valueNode)
	}
	text, err := yaml.Marshal(mapping)
	return string(text), err
}

func ParseDraft(text string) (map[string]any, error) {
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(text), &fields); err != nil {
		return nil, fmt.Errorf("draft is not valid YAML: %w", err)
	}
	if summary, _ := fields["summary"].(string); strings.TrimSpace(summary) == "" {
		return nil, errors.New("draft needs a summary")
	}
	return fields, nil
}

func SaveDraft(root, noteID, text string) error {
	if _, err := ParseDraft(text); err != nil {
		return err
	}
	dir := StateDir(root, noteID)
	if err := os.MkdirAll(dir, paths.PrivateDirMode); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, draftFile), []byte(text), paths.PrivateFileMode)
}

func LoadDraft(root, noteID string) (string, error) {
	text, err := os.ReadFile(filepath.Join(StateDir(root, noteID), draftFile))
	return string(text), err
}

func resultSection(result Result, draftFields map[string]any) string {
	summary, _ := draftFields["summary"].(string)
	description, _ := draftFields["description"].(string)
	label := result.Key
	if label == "" {
		label = summary
	}
	link := label
	if result.URL != "" {
		link = fmt.Sprintf("[%s](%s)", label, result.URL)
	}
	heading := "Ticket"
	if result.Kind == KindDoc {
		heading = "Doc"
	}
	section := fmt.Sprintf("## %s\n\n%s", heading, link)
	if summary != "" && label != summary {
		section += "\n\n### " + summary
	}
	if description = strings.TrimSpace(description); description != "" {
		section += "\n\n" + description
	}
	return section
}

func RunJob(ctx context.Context, cfg *config.Config, noteID, automationName string, phase Phase) error {
	spec, found := Find(cfg.AutomationList(), automationName)
	if !found {
		return fmt.Errorf("no automation named %q in the config", automationName)
	}
	noteStore := store.New(cfg.NotesDir())
	note, err := noteStore.FindByID(noteID)
	if err != nil {
		return err
	}
	result, err := Execute(ctx, spec, cfg.AutomationDir(), note, phase)
	if err != nil || phase != PhaseCreate {
		return err
	}
	if result.Kind == KindTicket && result.URL == "" && result.Key != "" && cfg.JiraBaseURL != "" {
		result.URL = cfg.JiraBaseURL + result.Key
	}
	draft, err := LoadDraft(cfg.AutomationDir(), noteID)
	if err != nil {
		return fmt.Errorf("%s created but its draft could not be read: %w", result.Kind, err)
	}
	draftFields, _ := ParseDraft(draft)
	note.Body = strings.TrimSpace(strings.TrimSpace(note.Body) + "\n\n" + resultSection(result, draftFields))
	note.Automated, note.Updated = result.Kind, time.Now()
	if err := noteStore.Save(note); err != nil {
		return fmt.Errorf("%s created but the note could not be saved: %w", result.Kind, err)
	}
	if err := os.Remove(filepath.Join(StateDir(cfg.AutomationDir(), noteID), draftFile)); err != nil {
		return fmt.Errorf("%s created but its draft could not be removed: %w", result.Kind, err)
	}
	fmt.Printf("Created %s %s %s\n", result.Kind, result.Key, result.URL)
	return nil
}
