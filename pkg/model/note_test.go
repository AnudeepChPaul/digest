package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/achandrapaul/digest/pkg/model"

	"gopkg.in/yaml.v3"
)

func TestNoteFrontMatterRoundTripsAndSkipsFileOnlyFields(t *testing.T) {
	created := time.Date(2026, 10, 9, 8, 30, 0, 0, time.UTC)
	note := model.Note{
		ID: "09-10-2026-1", Created: created, Updated: created.Add(time.Hour), Status: model.StatusDone,
		Source: model.SourcePRReview, Summary: "review", Subject: "subject", Repo: "acme/web", Ref: "7",
		Automated: model.AutomationTicket, Body: "body", FilePath: "/notes/x.md",
	}
	encoded, err := yaml.Marshal(note)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.Note
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	want := note
	want.Body, want.FilePath = "", ""
	if !reflect.DeepEqual(decoded, want) {
		t.Errorf("decoded = %+v, want %+v", decoded, want)
	}
	var keys map[string]any
	if err := yaml.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "created", "updated", "status", "source", "summary", "subject", "repo", "ref", "automated"} {
		if _, found := keys[key]; !found {
			t.Errorf("front matter is missing %q: %s", key, encoded)
		}
	}
	if len(keys) != 10 {
		t.Errorf("front matter keys = %v", keys)
	}
}

func TestEmptyOptionalFieldsAreLeftOut(t *testing.T) {
	encoded, err := yaml.Marshal(model.Note{Summary: "bare"})
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]any
	if err := yaml.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"subject", "repo", "ref", "automated"} {
		if _, found := keys[key]; found {
			t.Errorf("empty %q should be omitted: %s", key, encoded)
		}
	}
}

func TestOnlyActiveDoneAndArchivedAreKnownStatuses(t *testing.T) {
	for _, status := range []model.Status{model.StatusActive, model.StatusDone, model.StatusArchived} {
		if !status.Known() {
			t.Errorf("%q should be known", status)
		}
	}
	for _, status := range []model.Status{"", "inbox", "someday"} {
		if status.Known() {
			t.Errorf("%q should not be known", status)
		}
	}
}

func TestSourcesStayFreeText(t *testing.T) {
	var note model.Note
	if err := yaml.Unmarshal([]byte("source: carrier-pigeon\n"), &note); err != nil || note.Source != "carrier-pigeon" {
		t.Errorf("source = %q, %v", note.Source, err)
	}
}
