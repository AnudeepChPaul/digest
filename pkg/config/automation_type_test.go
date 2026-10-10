package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltInAutomationsComeFromTheEmbeddedTemplate(t *testing.T) {
	want := map[string]string{
		AutomationJira:           "ticket",
		AutomationConfluence:     "confluence",
		AutomationGoogleDoc:      "doc",
		AutomationGoogleCalendar: "event",
		AutomationPRReview:       "reviewed",
	}
	for _, spec := range builtInAutomations() {
		if spec.Type != want[spec.Name] {
			t.Errorf("%s automation_type = %q, want %q", spec.Name, spec.Type, want[spec.Name])
		}
		for _, phrase := range spec.Match {
			if strings.Contains(phrase, "create") {
				t.Errorf("%s match %q should not contain create", spec.Name, phrase)
			}
		}
		if spec.Name == AutomationPRReview {
			if spec.Prompt == "" {
				t.Error("pr review should carry its prompt in the template")
			}
			continue
		}
		if spec.DraftPrompt == "" || spec.CreatePrompt == "" {
			t.Errorf("%s should carry both prompts in the template", spec.Name)
		}
		if !strings.Contains(string(spec.CreatePrompt), `"kind": "`+spec.Type+`"`) {
			t.Errorf("%s create prompt should return kind %q", spec.Name, spec.Type)
		}
	}
	calendar, _ := findAutomation(builtInAutomations(), AutomationGoogleCalendar)
	if strings.Join(calendar.Plugins, ",") != "claude_ai_Google_Calendar" || strings.Join(calendar.Match, ",") != "calendar event,event,schedule a meeting,add to calendar" {
		t.Errorf("google calendar = %+v", calendar)
	}
	if !strings.Contains(string(calendar.DraftPrompt), "NO TIME:") {
		t.Error("the calendar draft should stop when the note has no time")
	}
}

func TestBuiltInAutomationsAreCopies(t *testing.T) {
	first := builtInAutomations()
	first[0].Match[0] = "changed"
	if builtInAutomations()[0].Match[0] == "changed" {
		t.Error("callers must not change the built-in specs")
	}
}

func loadAutomations(t *testing.T, automations string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("automations:\n"+automations), 0644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadFillsBuiltInTypesAndRequiresCustomOnes(t *testing.T) {
	cfg, err := loadAutomations(t, "  - name: jira\n    draft_prompt: \"\"\n")
	if err != nil {
		t.Fatalf("a built-in entry without a type should load: %v", err)
	}
	if jira, _ := findAutomation(cfg.AutomationList(), AutomationJira); jira.Type != "ticket" {
		t.Errorf("jira type = %q", jira.Type)
	}
	_, err = loadAutomations(t, "  - name: docs\n    draft_prompt: draft it\n    create_prompt: make it\n")
	if err == nil || !strings.Contains(err.Error(), "automation docs: automation_type is required") {
		t.Errorf("err = %v", err)
	}
	if _, err = loadAutomations(t, "  - name: docs\n    automation_type: doc\n    draft_prompt: draft it\n    create_prompt: make it\n"); err != nil {
		t.Errorf("a typed custom automation should load: %v", err)
	}
}

func TestLoadRequiresDraftAndCreatePrompts(t *testing.T) {
	for _, entry := range []string{
		"  - name: docs\n    automation_type: doc\n    create_prompt: make it\n",
		"  - name: docs\n    automation_type: doc\n    draft_prompt: draft it\n",
	} {
		if _, err := loadAutomations(t, entry); err == nil || !strings.Contains(err.Error(), "automation docs: draft_prompt and create_prompt are required") {
			t.Errorf("%q: err = %v", entry, err)
		}
	}
	if _, err := loadAutomations(t, "  - name: pr review\n    prompt: review it\n"); err != nil {
		t.Errorf("pr review needs no draft or create prompt: %v", err)
	}
}

const customisedConfig = `# my root
digest_root: /mine
jobs:
  - name: repo-sync
    options:
      roots: [~/mine]
automations:
  - name: jira
    automation_type: ticket
    match:
      - create a ticket
    draft_prompt: ""
    create_prompt: ""
  - name: mine
    automation_type: note
    draft_prompt: draft it
    create_prompt: make it
`

func TestInitAutomationsReplacesOnlyTheAutomationsSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(customisedConfig), 0600); err != nil {
		t.Fatal(err)
	}
	written, backup, err := InitAutomations(path)
	if err != nil || written != path || backup != path+".bak" {
		t.Fatalf("init automations = %q %q %v", written, backup, err)
	}
	if saved, _ := os.ReadFile(backup); string(saved) != customisedConfig {
		t.Errorf("backup = %q", saved)
	}
	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "# my root\ndigest_root: /mine\n") || !strings.Contains(string(content), "roots: [~/mine]") {
		t.Errorf("init --automations should keep the rest of the config:\n%s", content)
	}
	if strings.Contains(string(content), "name: mine") || strings.Contains(string(content), "create a ticket") {
		t.Errorf("the automations section should be the template's:\n%s", content)
	}
	if !strings.Contains(string(content), "# every automation except pr review drafts first") {
		t.Errorf("the template's automations comment should come along:\n%s", content)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := builtInAutomations()
	got := cfg.AutomationList()
	if len(got) != len(want) {
		t.Fatalf("automations = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].Name != want[index].Name || got[index].DraftPrompt != want[index].DraftPrompt || got[index].CreatePrompt != want[index].CreatePrompt || got[index].Prompt != want[index].Prompt {
			t.Errorf("%s should carry the template's full prompts", want[index].Name)
		}
	}
}

func TestInitAutomationsAddsTheSectionWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("digest_root: /mine\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InitAutomations(path); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.DigestRoot != "/mine" || len(cfg.Automations) != len(builtInAutomations()) {
		t.Errorf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestInitAutomationsWritesTheTemplateWithoutAConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "digest", "config.yaml")
	written, backup, err := InitAutomations(path)
	if err != nil || written != path || backup != "" {
		t.Fatalf("init automations = %q %q %v", written, backup, err)
	}
	if content, _ := os.ReadFile(path); string(content) != DefaultConfigYAML {
		t.Errorf("without a config it should write the template")
	}
}
