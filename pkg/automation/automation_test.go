package automation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/aitool"
	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/store"
)

func fakeTypedSpec(automationType, command string) config.AutomationSpec {
	spec := fakeSpec(command)
	spec.Type = automationType
	return spec
}

func fakeSpec(command string) config.AutomationSpec {
	return config.AutomationSpec{
		Name:         "PROJ",
		Type:         KindTicket,
		Match:        []string{"create a ticket", "create jira"},
		Command:      command,
		DraftPrompt:  "DRAFT PROMPT",
		CreatePrompt: "CREATE PROMPT",
		ReauthHint:   "store a new token",
	}
}

func testNote() *model.Note {
	return &model.Note{ID: "note-1", Summary: "Create jira for flaky deploys", Body: "deploys fail on Mondays"}
}

func TestMatchReadsTheWholeNote(t *testing.T) {
	specs := []config.AutomationSpec{fakeSpec("cat"), {Name: "EMPTY", Match: []string{"", "  "}}}
	for _, testCase := range []struct {
		summary, body string
		want          bool
	}{
		{"Create a ticket for the outage", "", true},
		{"  create JIRA: flaky deploy", "", true},
		{"Please create a ticket", "", true},
		{"Flaky deploys", "We should CREATE A TICKET for this", true},
		{"Review PR", "nothing here", false},
		{"", "", false},
	} {
		spec, matched := Match(specs, &model.Note{Summary: testCase.summary, Body: testCase.body})
		if matched != testCase.want || (matched && spec.Name != "PROJ") {
			t.Errorf("Match(%q, %q) = %v %q, want %v", testCase.summary, testCase.body, matched, spec.Name, testCase.want)
		}
	}
	if _, matched := Match(specs, nil); matched {
		t.Error("nil note matched")
	}
}

func TestDraftPhaseSavesAnOrderedDraft(t *testing.T) {
	root := t.TempDir()
	spec := fakeSpec(`cat > "$INPUT_COPY"; echo 'Here is the draft:'; printf '%s\n' '{"description":"## Problem\nDeploys fail","labels":["deploy"],"summary":"Flaky deploys","project":"PROJ","issue_type":"Task","team":"Platform"}'`)
	inputCopy := filepath.Join(root, "input.txt")
	t.Setenv("INPUT_COPY", inputCopy)
	if _, err := Execute(context.Background(), spec, root, testNote(), PhaseDraft); err != nil {
		t.Fatal(err)
	}
	draft, err := LoadDraft(root, "note-1")
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"project:", "issue_type:", "summary:", "labels:", "team:", "description:"}
	last := -1
	for _, key := range order {
		index := strings.Index(draft, key)
		if index <= last {
			t.Fatalf("key %s out of order in:\n%s", key, draft)
		}
		last = index
	}
	input, _ := os.ReadFile(inputCopy)
	for _, want := range []string{"DRAFT PROMPT", "Create jira for flaky deploys", "deploys fail on Mondays"} {
		if !strings.Contains(string(input), want) {
			t.Errorf("claude input misses %q:\n%s", want, input)
		}
	}
}

func TestDraftWithoutSummaryFails(t *testing.T) {
	root := t.TempDir()
	_, err := Execute(context.Background(), fakeSpec(`echo '{"project":"PROJ"}'`), root, testNote(), PhaseDraft)
	if err == nil || !strings.Contains(err.Error(), "summary") {
		t.Errorf("err = %v", err)
	}
	if _, err := Execute(context.Background(), fakeSpec(`echo 'no json here'`), root, testNote(), PhaseDraft); err == nil {
		t.Error("output without JSON should fail")
	}
	_, err = Execute(context.Background(), fakeSpec(`echo 'Looking at the note'; echo; echo 'NO PROJECT: add a Jira project key to the note'; echo`), root, testNote(), PhaseDraft)
	if err == nil || !strings.Contains(err.Error(), "NO PROJECT: add a Jira project key to the note") || strings.Contains(err.Error(), "Looking at the note") {
		t.Errorf("err = %v", err)
	}
}

func TestAuthErrorsInClaudeOutputNeedReauth(t *testing.T) {
	for _, command := range []string{
		`echo 'Request failed: 401 Unauthorized'`,
		`echo 'Call mcp__atlassian__authenticate first'`,
		`echo 'error: token expired' >&2; exit 1`,
	} {
		_, err := Execute(context.Background(), fakeSpec(command), t.TempDir(), testNote(), PhaseDraft)
		if !errors.Is(err, ErrNeedsReauth) {
			t.Errorf("%s: err = %v", command, err)
		}
	}
}

func TestCreatePhaseReturnsTheResult(t *testing.T) {
	root := t.TempDir()
	if err := SaveDraft(root, "note-1", "summary: Flaky deploys\nproject: PROJ\n"); err != nil {
		t.Fatal(err)
	}
	inputCopy := filepath.Join(root, "input.txt")
	t.Setenv("INPUT_COPY", inputCopy)
	spec := fakeSpec(`cat > "$INPUT_COPY"; echo 'Created it.'; echo '{"kind":"Ticket","key":"PROJ-77","url":"https://jira/browse/PROJ-77"}'`)
	result, err := Execute(context.Background(), spec, root, testNote(), PhaseCreate)
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{Kind: KindTicket, Key: "PROJ-77", URL: "https://jira/browse/PROJ-77"}) {
		t.Errorf("result = %+v", result)
	}
	input, _ := os.ReadFile(inputCopy)
	if !strings.Contains(string(input), "CREATE PROMPT") || !strings.Contains(string(input), "summary: Flaky deploys") {
		t.Errorf("create input:\n%s", input)
	}
	doc, err := Execute(context.Background(), fakeTypedSpec("doc", `echo '{"kind":"doc","url":"https://docs/page"}'`), root, testNote(), PhaseCreate)
	if err != nil || doc != (Result{Kind: "doc", URL: "https://docs/page"}) {
		t.Errorf("doc = %+v %v", doc, err)
	}
	for _, output := range []string{`{"key":"PROJ-78"}`, `{"kind":"page","key":"PROJ-78"}`, `{"kind":"doc","key":"PROJ-78"}`, `{"kind":"ticket"}`, "Created PROJ-78 for you"} {
		if result, err := Execute(context.Background(), fakeSpec("echo '"+output+"'"), root, testNote(), PhaseCreate); err == nil {
			t.Errorf("%s accepted as %+v", output, result)
		}
	}
}

func TestCreateWithoutDraftFails(t *testing.T) {
	if _, err := Execute(context.Background(), fakeSpec(`echo '{"key":"A-1"}'`), t.TempDir(), testNote(), PhaseCreate); err == nil {
		t.Error("create without a draft should fail")
	}
}

func TestCommandTimesOut(t *testing.T) {
	previous := commandTimeout
	commandTimeout = 200 * time.Millisecond
	t.Cleanup(func() { commandTimeout = previous })
	started := time.Now()
	_, err := Execute(context.Background(), fakeSpec("sleep 5 & wait"), t.TempDir(), testNote(), PhaseDraft)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v", err)
	}
	if time.Since(started) > 4*time.Second {
		t.Errorf("timeout did not kill the command group: %s", time.Since(started))
	}
}

func TestSaveDraftValidates(t *testing.T) {
	root := t.TempDir()
	for _, invalid := range []string{"summary: [unclosed", "project: PROJ\n", "- a list\n"} {
		if err := SaveDraft(root, "note-1", invalid); err == nil {
			t.Errorf("SaveDraft(%q) accepted", invalid)
		}
	}
}

func writeRunState(t *testing.T, root, noteID string, phase Phase, exitCode string) {
	t.Helper()
	dir := StateDir(root, noteID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := writeMeta(dir, RunMeta{NoteID: noteID, Automation: "PROJ", Phase: phase}); err != nil {
		t.Fatal(err)
	}
	if exitCode != "" {
		if err := os.WriteFile(filepath.Join(dir, runExitFile), []byte(exitCode), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunStatusFromExitCodes(t *testing.T) {
	root := t.TempDir()
	writeRunState(t, root, "drafted", PhaseDraft, "0")
	writeRunState(t, root, "created", PhaseCreate, "0")
	writeRunState(t, root, "reauth", PhaseCreate, "3")
	writeRunState(t, root, "failed", PhaseDraft, "1")
	runs := ListRuns(root)
	for noteID, want := range map[string]RunStatus{"drafted": RunDraftReady, "created": RunCreated, "reauth": RunNeedsReauth, "failed": RunFailed} {
		if runs[noteID].Status != want || runs[noteID].Meta.NoteID != noteID {
			t.Errorf("%s = %+v, want %v", noteID, runs[noteID], want)
		}
	}
	if err := Dismiss(root, "failed"); err != nil {
		t.Fatal(err)
	}
	if _, kept := ListRuns(root)["failed"]; kept {
		t.Error("dismissed run still listed")
	}
}

func TestBackgroundRunRecordsTheExitCode(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "fake-digest")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\"\nexit 3\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return script, nil }
	t.Cleanup(func() { executablePath = previous })
	if err := StartBackground(root, "note-1", "PROJ", PhaseCreate); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for Status(root, "note-1").Status == RunRunning && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	run := Status(root, "note-1")
	if run.Status != RunNeedsReauth || run.Meta.Phase != PhaseCreate || run.Meta.Automation != "PROJ" {
		t.Errorf("run = %+v", run)
	}
	logText, _ := os.ReadFile(LogPath(root, "note-1"))
	if !strings.Contains(string(logText), "automation --note note-1 --name PROJ --phase create") {
		t.Errorf("log = %q", logText)
	}
	if err := StartBackground(root, "note-1", "PROJ", PhaseDraft); err != nil {
		t.Fatal(err)
	}
}

func TestDraftRunReplacesTheOldDraftAndCreateKeepsIt(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "fake-digest")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return script, nil }
	t.Cleanup(func() { executablePath = previous })
	waitForRun := func() Run {
		deadline := time.Now().Add(5 * time.Second)
		for Status(root, "note-1").Status == RunRunning && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		return Status(root, "note-1")
	}
	if err := SaveDraft(root, "note-1", "summary: Old\n"); err != nil {
		t.Fatal(err)
	}
	if run := Status(root, "note-1"); !run.HasDraft {
		t.Errorf("saved draft not reported: %+v", run)
	}
	if err := StartBackground(root, "note-1", "PROJ", PhaseCreate); err != nil {
		t.Fatal(err)
	}
	if run := waitForRun(); run.Status != RunFailed || !run.HasDraft {
		t.Errorf("failed create lost the draft: %+v", run)
	}
	if err := StartBackground(root, "note-1", "PROJ", PhaseDraft); err != nil {
		t.Fatal(err)
	}
	if run := waitForRun(); run.HasDraft {
		t.Errorf("new draft run kept the old draft: %+v", run)
	}
}

func TestRunJobRecordsTheResultOnTheNote(t *testing.T) {
	root := t.TempDir()
	spec := fakeSpec(`echo '{"kind":"ticket","key":"PROJ-9"}'`)
	cfg := &config.Config{DigestRoot: root, JiraBaseURL: "https://jira.example/browse/", Automations: []config.AutomationSpec{spec}}
	noteStore := store.New(cfg.NotesDir())
	note := &model.Note{Summary: "Create a ticket for logs", Body: "Logs vanish at midnight", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	if err := SaveDraft(cfg.AutomationDir(), note.ID, "summary: Lost logs\ndescription: Logs vanish after rotation\n"); err != nil {
		t.Fatal(err)
	}
	if err := RunJob(context.Background(), cfg, note.ID, "PROJ", PhaseCreate); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Load(note.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Automated != KindTicket {
		t.Errorf("automated = %q", saved.Automated)
	}
	for _, want := range []string{"Logs vanish at midnight", "## Ticket", "[PROJ-9](https://jira.example/browse/PROJ-9)", "Lost logs", "Logs vanish after rotation"} {
		if !strings.Contains(saved.Body, want) {
			t.Errorf("body misses %q:\n%s", want, saved.Body)
		}
	}
	if strings.Index(saved.Body, "Logs vanish at midnight") > strings.Index(saved.Body, "## Ticket") {
		t.Errorf("result not appended after the original body:\n%s", saved.Body)
	}
	if _, err := LoadDraft(cfg.AutomationDir(), note.ID); !os.IsNotExist(err) {
		t.Errorf("draft kept after success: %v", err)
	}
	if err := RunJob(context.Background(), cfg, note.ID, "MISSING", PhaseCreate); err == nil {
		t.Error("unknown automation should fail")
	}
}

func TestCreateRejectsAKindThatIsNotTheAutomationType(t *testing.T) {
	root := t.TempDir()
	if err := SaveDraft(root, "note-1", "summary: Standup\n"); err != nil {
		t.Fatal(err)
	}
	spec := fakeTypedSpec("event", `echo '{"kind":"doc","url":"https://calendar/e1"}'`)
	if _, err := Execute(context.Background(), spec, root, testNote(), PhaseCreate); err == nil || !strings.Contains(err.Error(), `kind must be "event", got "doc"`) {
		t.Errorf("err = %v", err)
	}
	untyped := fakeTypedSpec("", `echo '{"kind":"","url":"https://calendar/e1"}'`)
	if _, err := Execute(context.Background(), untyped, root, testNote(), PhaseCreate); err == nil {
		t.Error("an empty kind should never be accepted")
	}
}

func TestRunJobAppendsAnEventSection(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{DigestRoot: root, Automations: []config.AutomationSpec{fakeTypedSpec("event", `echo '{"kind":"Event","key":"e1","url":"https://calendar/e1"}'`)}}
	noteStore := store.New(cfg.NotesDir())
	note := &model.Note{Summary: "Schedule a meeting with platform", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	if err := SaveDraft(cfg.AutomationDir(), note.ID, "summary: Platform sync\n"); err != nil {
		t.Fatal(err)
	}
	if err := RunJob(context.Background(), cfg, note.ID, "PROJ", PhaseCreate); err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Load(note.FilePath)
	if saved.Automated != "event" || !strings.Contains(saved.Body, "## Event\n\n[e1](https://calendar/e1)") || !strings.Contains(saved.Body, "### Platform sync") {
		t.Errorf("note = %+v", saved)
	}
}

func TestRunJobAppendsADocSection(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{DigestRoot: root, Automations: []config.AutomationSpec{fakeTypedSpec("doc", `echo '{"kind":"doc","url":"https://docs/page"}'`)}}
	noteStore := store.New(cfg.NotesDir())
	note := &model.Note{Summary: "Create a ticket for docs", Status: model.StatusActive, Source: model.SourceManual, Created: time.Now()}
	if err := noteStore.Save(note); err != nil {
		t.Fatal(err)
	}
	if err := SaveDraft(cfg.AutomationDir(), note.ID, "summary: Runbook\n"); err != nil {
		t.Fatal(err)
	}
	if err := RunJob(context.Background(), cfg, note.ID, "PROJ", PhaseCreate); err != nil {
		t.Fatal(err)
	}
	saved, _ := store.Load(note.FilePath)
	if saved.Automated != "doc" || !strings.Contains(saved.Body, "## Doc") || !strings.Contains(saved.Body, "[Runbook](https://docs/page)") {
		t.Errorf("note = %+v", saved)
	}
}

func TestStopEndsTheRunAndKeepsTheDraft(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "fake-digest")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	previous := executablePath
	executablePath = func() (string, error) { return script, nil }
	t.Cleanup(func() { executablePath = previous })
	if err := SaveDraft(root, "note-1", "summary: Flaky deploys\n"); err != nil {
		t.Fatal(err)
	}
	if err := StartBackground(root, "note-1", "PROJ", PhaseCreate); err != nil {
		t.Fatal(err)
	}
	if Status(root, "note-1").Status != RunRunning {
		t.Fatal("run did not start")
	}
	if err := Stop(root, "note-1"); err != nil {
		t.Fatal(err)
	}
	run := Status(root, "note-1")
	if run.Status != RunFailed || !run.HasDraft {
		t.Errorf("run after stop = %+v", run)
	}
	if err := Stop(root, "note-1"); err != nil {
		t.Errorf("stopping a stopped run: %v", err)
	}
}

func TestCommandIsBuiltFromTheEntryPlugins(t *testing.T) {
	var ranCommands []string
	previousRun, previousChecks := runCommand, tokenChecks
	runCommand = func(ctx context.Context, command, input string) (string, error) {
		ranCommands = append(ranCommands, command)
		return `{"summary": "Flaky deploys"}`, nil
	}
	tokenChecks = func(plugins []string) []aitool.TokenCheck { return nil }
	t.Cleanup(func() { runCommand, tokenChecks = previousRun, previousChecks })
	spec := config.AutomationSpec{Name: "jira", Plugins: []string{"claude_ai_Example"}, DraftPrompt: "DRAFT"}
	if _, err := Execute(context.Background(), spec, t.TempDir(), testNote(), PhaseDraft); err != nil {
		t.Fatal(err)
	}
	if len(ranCommands) != 1 || !strings.HasPrefix(ranCommands[0], "claude -p --model sonnet") || !strings.Contains(ranCommands[0], "--allowedTools 'mcp__claude_ai_Example__*'") {
		t.Errorf("commands = %v", ranCommands)
	}
}

func TestInstalledPluginTokenCheckRunsFirst(t *testing.T) {
	var ranCommands []string
	previousRun, previousChecks := runCommand, tokenChecks
	runCommand = func(ctx context.Context, command, input string) (string, error) {
		ranCommands = append(ranCommands, command)
		if command == "check jira token" {
			return "401 Unauthorized", errors.New("exit status 1")
		}
		return `{"summary": "x"}`, nil
	}
	tokenChecks = func(plugins []string) []aitool.TokenCheck {
		if strings.Join(plugins, ",") != "jira-inator" {
			t.Errorf("plugins = %v", plugins)
		}
		return []aitool.TokenCheck{{Plugin: "jira-inator", Command: "check jira token", ReauthHint: "make a new jira token"}}
	}
	t.Cleanup(func() { runCommand, tokenChecks = previousRun, previousChecks })
	spec := config.AutomationSpec{Name: "jira", Plugins: []string{"jira-inator"}, DraftPrompt: "DRAFT"}
	_, err := Execute(context.Background(), spec, t.TempDir(), testNote(), PhaseDraft)
	if !errors.Is(err, ErrNeedsReauth) || !strings.Contains(err.Error(), "make a new jira token") {
		t.Errorf("err = %v", err)
	}
	if len(ranCommands) != 1 {
		t.Errorf("claude should not run after a failed check: %v", ranCommands)
	}
	tokenChecks = func([]string) []aitool.TokenCheck { return nil }
	ranCommands = nil
	if _, err := Execute(context.Background(), spec, t.TempDir(), testNote(), PhaseDraft); err != nil || len(ranCommands) != 1 {
		t.Errorf("no installed plugin should skip the check: err %v commands %v", err, ranCommands)
	}
}

func TestTokenCheckErrorHidesCommandOutput(t *testing.T) {
	previousRun, previousChecks := runCommand, tokenChecks
	runCommand = func(ctx context.Context, command, input string) (string, error) {
		return "Authorization: Bearer secret-token-value", errors.New("exit status 1")
	}
	tokenChecks = func([]string) []aitool.TokenCheck {
		return []aitool.TokenCheck{{Plugin: "jira-inator", Command: "check jira token"}}
	}
	t.Cleanup(func() { runCommand, tokenChecks = previousRun, previousChecks })
	spec := config.AutomationSpec{Name: "jira", Plugins: []string{"jira-inator"}, DraftPrompt: "DRAFT"}
	_, err := Execute(context.Background(), spec, t.TempDir(), testNote(), PhaseDraft)
	if !errors.Is(err, ErrNeedsReauth) || strings.Contains(err.Error(), "secret-token-value") || !strings.Contains(err.Error(), "jira-inator") {
		t.Errorf("err = %v", err)
	}
}

func TestReauthErrorWithoutHintHidesCommandOutput(t *testing.T) {
	err := needsReauth(config.AutomationSpec{Name: "jira"})
	if !errors.Is(err, ErrNeedsReauth) || strings.Contains(err.Error(), "secret-value") || !strings.Contains(err.Error(), "jira") {
		t.Errorf("err = %v", err)
	}
}
