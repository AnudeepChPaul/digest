package automation

import (
	"testing"

	"github.com/achandrapaul/digest/pkg/config"
)

func TestKnownKindsAreBuiltInSourcesAndAutomationTypes(t *testing.T) {
	cfg := &config.Config{Automations: []config.AutomationSpec{{Name: "jira", Type: "ticket"}, {Name: "standup", Type: "Event"}}}
	for _, kind := range []string{"manual", "my-pr", "pr-review", "PR-REVIEW", "ticket", "TICKET", "event", "reviewed"} {
		if !IsKnownKind(cfg, kind) {
			t.Errorf("%q should be a known kind", kind)
		}
	}
	for _, kind := range []string{"", " ", "jira", "standup", "doc", "repo-sync", "slack"} {
		if IsKnownKind(cfg, kind) {
			t.Errorf("%q should not be a known kind", kind)
		}
	}
	for _, kind := range []string{"ticket", "confluence", "doc", "event", "reviewed", "manual"} {
		if !IsKnownKind(nil, kind) {
			t.Errorf("without a config the built-in type %q should be known", kind)
		}
	}
	if IsKnownKind(nil, config.AutomationJira) {
		t.Error("automation names are not kinds")
	}
}
