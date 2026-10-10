package habit

import (
	"fmt"
	"time"

	"github.com/achandrapaul/digest/pkg/model"
)

type Slot int

const (
	SlotMorning Slot = iota
	SlotEvening
)

type Facts struct {
	Pending        int
	CarriedOver    int
	OldestDays     int
	ClosedToday    int
	ClosedThisWeek int
	OpenMyPRs      int
	Streak         int
}

const maxStreakDays = 366

func startOfDay(moment time.Time) time.Time {
	year, month, day := moment.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, moment.Location())
}

func startOfISOWeek(moment time.Time) time.Time {
	daysSinceMonday := (int(moment.Weekday()) + 6) % 7
	return startOfDay(moment).AddDate(0, 0, -daysSinceMonday)
}

func isPending(note *model.Note) bool {
	return note.Status != model.StatusDone && note.Status != model.StatusArchived
}

func Gather(notes []*model.Note, now time.Time, isWorkDay func(time.Weekday) bool) Facts {
	today, weekStart := startOfDay(now), startOfISOWeek(now)
	var facts Facts
	for _, note := range notes {
		switch {
		case isPending(note):
			facts.Pending++
			if note.Created.Before(today) {
				facts.CarriedOver++
				facts.OldestDays = max(facts.OldestDays, int(today.Sub(startOfDay(note.Created)).Hours()/24))
			}
			if note.Source == model.SourceMyPR {
				facts.OpenMyPRs++
			}
		case note.Status == model.StatusDone:
			if !note.Updated.Before(today) {
				facts.ClosedToday++
			}
			if !note.Updated.Before(weekStart) {
				facts.ClosedThisWeek++
			}
		}
	}
	facts.Streak = Streak(notes, now, isWorkDay)
	return facts
}

func Streak(notes []*model.Note, now time.Time, isWorkDay func(time.Weekday) bool) int {
	closedDays := map[time.Time]bool{}
	for _, note := range notes {
		if note.Status == model.StatusDone {
			closedDays[startOfDay(note.Updated.In(now.Location()))] = true
		}
	}
	day := startOfDay(now)
	if !closedDays[day] {
		day = day.AddDate(0, 0, -1)
	}
	streak := 0
	for range maxStreakDays {
		switch {
		case closedDays[day] && isWorkDay(day.Weekday()):
			streak++
		case isWorkDay(day.Weekday()):
			return streak
		}
		day = day.AddDate(0, 0, -1)
	}
	return streak
}

func plural(count int, word string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", count, word)
}

func SummaryLines(facts Facts, slot Slot) []string {
	var lines []string
	if slot == SlotMorning {
		switch {
		case facts.Pending == 0:
			return []string{"✨ Clean slate — nothing pending. Enjoy it."}
		case facts.OldestDays >= 2:
			lines = append(lines, fmt.Sprintf("☀️ %s waiting · oldest is %d days old", plural(facts.Pending, "note"), facts.OldestDays))
		default:
			lines = append(lines, fmt.Sprintf("☀️ %s on your plate today", plural(facts.Pending, "note")))
		}
		if facts.OpenMyPRs > 0 {
			lines = append(lines, fmt.Sprintf("🔀 %d of your PRs still open — worth a nudge?", facts.OpenMyPRs))
		}
		if facts.Streak > 0 {
			lines = append(lines, fmt.Sprintf("🔥 Day %d of your streak — close one to keep it going", facts.Streak))
		}
		return lines
	}
	switch {
	case facts.ClosedToday > 0 && facts.Pending > 0:
		lines = append(lines, fmt.Sprintf("✅ %d closed today, %d to go", facts.ClosedToday, facts.Pending))
	case facts.ClosedToday > 0:
		lines = append(lines, fmt.Sprintf("🎉 %d closed today and nothing left. Great day.", facts.ClosedToday))
	case facts.Pending > 0:
		lines = append(lines, fmt.Sprintf("🌙 %s carried to tomorrow", plural(facts.Pending, "note")))
	}
	if len(lines) == 0 {
		return nil
	}
	if facts.Streak > 1 {
		lines = append(lines, fmt.Sprintf("🔥 %d-day streak. Nice.", facts.Streak))
	}
	if facts.ClosedThisWeek > 0 {
		lines = append(lines, fmt.Sprintf("📈 %d closed this week", facts.ClosedThisWeek))
	}
	return lines
}

func PickLine(lines []string, now time.Time, slot Slot) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[(now.YearDay()*2+int(slot))%len(lines)]
}
