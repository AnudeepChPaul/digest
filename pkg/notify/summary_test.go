package notify

import (
	"strings"
	"testing"
	"time"

	"app/pkg/habit"
)

func summaryAt(day, hour, minute int) time.Time {
	return time.Date(2026, time.October, day, hour, minute, 0, 0, time.Local)
}

func TestSummariesSendOncePerSlotOnWorkDays(t *testing.T) {
	root := t.TempDir()
	var sent []Notification
	previous := Send
	Send = func(notification Notification) error {
		sent = append(sent, notification)
		return nil
	}
	t.Cleanup(func() { Send = previous })
	schedule := SummarySchedule{Morning: "09:30", Evening: "18:00", Terminal: "com.example.term", IsWorkDay: func(day time.Weekday) bool { return day != time.Saturday && day != time.Sunday }}
	facts := habit.Facts{Pending: 4, OldestDays: 8, ClosedToday: 1}
	run := func(moment time.Time) {
		t.Helper()
		if err := RunSummaries(root, schedule, facts, moment); err != nil {
			t.Fatal(err)
		}
	}
	run(summaryAt(7, 9, 29))
	if len(sent) != 0 {
		t.Fatalf("before 09:30 nothing should send: %+v", sent)
	}
	run(summaryAt(7, 9, 31))
	run(summaryAt(7, 9, 45))
	if len(sent) != 1 || sent[0].Group != "digest-summary" || sent[0].Execute != "open -b 'com.example.term'" || !strings.Contains(sent[0].Title, "morning") {
		t.Fatalf("morning should send once: %+v", sent)
	}
	run(summaryAt(7, 18, 5))
	run(summaryAt(7, 19, 0))
	if len(sent) != 2 || !strings.Contains(sent[1].Message, "1 closed today, 4 to go") {
		t.Fatalf("evening should send once: %+v", sent)
	}
	run(summaryAt(10, 10, 0))
	if len(sent) != 2 {
		t.Errorf("Saturday is not a work day: %+v", sent)
	}
	run(summaryAt(12, 19, 0))
	if len(sent) != 3 || !strings.Contains(sent[2].Title, "evening") {
		t.Errorf("waking after the evening time should skip the morning and send the evening: %+v", sent)
	}
}

func TestSummariesSkipEmptyTimesAndQuietEvenings(t *testing.T) {
	root := t.TempDir()
	var sent []Notification
	previous := Send
	Send = func(notification Notification) error {
		sent = append(sent, notification)
		return nil
	}
	t.Cleanup(func() { Send = previous })
	schedule := SummarySchedule{Evening: "18:00", IsWorkDay: func(time.Weekday) bool { return true }}
	if err := RunSummaries(root, schedule, habit.Facts{}, summaryAt(7, 20, 0)); err != nil || len(sent) != 0 {
		t.Errorf("an empty morning time and a quiet evening send nothing: err %v sent %+v", err, sent)
	}
	if err := RunSummaries(root, schedule, habit.Facts{Pending: 1}, summaryAt(7, 20, 30)); err != nil || len(sent) != 1 || sent[0].Execute != "" {
		t.Errorf("evening with a pending note should send, without a click command: %+v", sent)
	}
}

func TestSummaryStateIsNotAReminderEntry(t *testing.T) {
	root := t.TempDir()
	if err := saveSummaryState(root, summaryState{MorningSent: "2026-10-07"}); err != nil {
		t.Fatal(err)
	}
	if entries, err := List(root); err != nil || len(entries) != 0 {
		t.Errorf("entries = %+v, %v; the summary state must not be listed", entries, err)
	}
}
