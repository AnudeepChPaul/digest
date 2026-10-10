package notify

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/habit"
	"github.com/achandrapaul/digest/pkg/system"

	"gopkg.in/yaml.v3"
)

const summaryGroup = "digest-summary"

type SummarySchedule struct {
	Morning   string
	Evening   string
	Terminal  string
	IsWorkDay func(time.Weekday) bool
}

type summaryState struct {
	MorningSent string `yaml:"morning_sent,omitempty"`
	EveningSent string `yaml:"evening_sent,omitempty"`
}

var summaryTitles = map[habit.Slot]string{habit.SlotMorning: "Good morning ☀️", habit.SlotEvening: "Good evening 🌙"}

func summaryStatePath(root string) string {
	return filepath.Join(Dir(root), ".summary.yaml")
}

func loadSummaryState(root string) summaryState {
	var state summaryState
	if data, err := system.Read(summaryStatePath(root)); err == nil {
		_ = yaml.Unmarshal(data, &state)
	}
	return state
}

func saveSummaryState(root string, state summaryState) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return system.Write(summaryStatePath(root), data)
}

func slotTime(now time.Time, clock string) (time.Time, bool) {
	if clock == "" {
		return time.Time{}, false
	}
	offset, err := config.ParseClock(clock)
	if err != nil {
		return time.Time{}, false
	}
	year, month, day := now.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, now.Location()).Add(offset), true
}

func RunSummaries(root string, schedule SummarySchedule, facts habit.Facts, now time.Time) error {
	if schedule.IsWorkDay != nil && !schedule.IsWorkDay(now.Weekday()) {
		return nil
	}
	today := now.Format("2006-01-02")
	state := loadSummaryState(root)
	morningAt, hasMorning := slotTime(now, schedule.Morning)
	eveningAt, hasEvening := slotTime(now, schedule.Evening)
	slot, sentToday := habit.SlotMorning, &state.MorningSent
	switch {
	case hasEvening && !now.Before(eveningAt):
		slot, sentToday = habit.SlotEvening, &state.EveningSent
	case hasMorning && !now.Before(morningAt):
	default:
		return nil
	}
	if *sentToday == today {
		return nil
	}
	line := habit.PickLine(habit.SummaryLines(facts, slot), now, slot)
	if line == "" {
		return nil
	}
	notification := Notification{Title: summaryTitles[slot], Message: line, Group: summaryGroup}
	if schedule.Terminal != "" {
		notification.Execute = "open -b " + shellQuote(schedule.Terminal)
	}
	if err := Send(notification); err != nil {
		return errors.Join(errors.New("digest summary"), err)
	}
	*sentToday = today
	return saveSummaryState(root, state)
}
