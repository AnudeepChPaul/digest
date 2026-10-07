package brag

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"app/pkg/config"
	"app/pkg/paths"

	"gopkg.in/yaml.v3"
)

type Period interface {
	ID() string
	Label() string
	Path(root string) string
	Range() (time.Time, time.Time)
	Kind() config.PromptKind
}

type Week struct {
	Year   int
	Number int
	Start  time.Time
}

func WeekOf(moment time.Time) Week {
	daysSinceMonday := (int(moment.Weekday()) + 6) % 7
	start := time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, moment.Location()).AddDate(0, 0, -daysSinceMonday)
	year, number := start.ISOWeek()
	return Week{Year: year, Number: number, Start: start}
}

func (w Week) End() time.Time {
	return w.Start.AddDate(0, 0, 7)
}

func (w Week) Range() (time.Time, time.Time) { return w.Start, w.End() }

func (w Week) Kind() config.PromptKind { return config.PromptWeek }

func (w Week) Previous() Week {
	return WeekOf(w.Start.AddDate(0, 0, -7))
}

func (w Week) ID() string {
	return fmt.Sprintf("%d-W%02d", w.Year, w.Number)
}

func (w Week) Days() string {
	return fmt.Sprintf("%s – %s", w.Start.Format("02 Jan"), w.End().AddDate(0, 0, -1).Format("02 Jan"))
}

func (w Week) Label() string {
	return fmt.Sprintf("%d · Week %02d · %s", w.Year, w.Number, w.Days())
}

func (w Week) Path(root string) string {
	return filepath.Join(root, fmt.Sprint(w.Year), fmt.Sprintf("week-%02d.md", w.Number))
}

func (w Week) Contains(moment time.Time) bool {
	return !moment.Before(w.Start) && moment.Before(w.End())
}

type Month struct {
	Start time.Time
}

func MonthOf(moment time.Time) Month {
	return Month{Start: time.Date(moment.Year(), moment.Month(), 1, 0, 0, 0, 0, moment.Location())}
}

func MonthOfWeek(week Week) Month {
	return MonthOf(week.End().AddDate(0, 0, -1))
}

func (m Month) Range() (time.Time, time.Time) { return m.Start, m.Start.AddDate(0, 1, 0) }
func (m Month) Kind() config.PromptKind       { return config.PromptMonth }
func (m Month) ID() string                    { return m.Start.Format("2006-01") }
func (m Month) Label() string                 { return m.Start.Format("January 2006") }
func (m Month) Previous() Month               { return MonthOf(m.Start.AddDate(0, -1, 0)) }

func (m Month) Path(root string) string {
	return filepath.Join(root, fmt.Sprint(m.Start.Year()), fmt.Sprintf("month-%02d.md", int(m.Start.Month())))
}

type Year struct {
	Start time.Time
}

func YearOf(moment time.Time) Year {
	return Year{Start: time.Date(moment.Year(), time.January, 1, 0, 0, 0, 0, moment.Location())}
}

func (y Year) Range() (time.Time, time.Time) { return y.Start, y.Start.AddDate(1, 0, 0) }
func (y Year) Kind() config.PromptKind       { return config.PromptYear }
func (y Year) ID() string                    { return fmt.Sprint(y.Start.Year()) }
func (y Year) Label() string                 { return fmt.Sprintf("%d performance review", y.Start.Year()) }

func (y Year) Path(root string) string {
	return filepath.Join(root, y.ID(), "performance-review.md")
}

var (
	weekIDPattern  = regexp.MustCompile(`^(\d{4})-W(\d{2})$`)
	monthIDPattern = regexp.MustCompile(`^(\d{4})-(\d{2})$`)
	yearIDPattern  = regexp.MustCompile(`^\d{4}$`)
)

func ParsePeriod(id string, location *time.Location) (Period, error) {
	if match := weekIDPattern.FindStringSubmatch(id); match != nil {
		year, _ := strconv.Atoi(match[1])
		number, _ := strconv.Atoi(match[2])
		week := WeekOf(time.Date(year, time.January, 4, 12, 0, 0, 0, location))
		week = WeekOf(week.Start.AddDate(0, 0, 7*(number-1)))
		if week.ID() != id {
			return nil, fmt.Errorf("%s is not an ISO week", id)
		}
		return week, nil
	}
	if match := monthIDPattern.FindStringSubmatch(id); match != nil {
		year, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		if month < 1 || month > 12 {
			return nil, fmt.Errorf("%s is not a month", id)
		}
		return MonthOf(time.Date(year, time.Month(month), 1, 12, 0, 0, 0, location)), nil
	}
	if yearIDPattern.MatchString(id) {
		year, _ := strconv.Atoi(id)
		return YearOf(time.Date(year, time.June, 1, 12, 0, 0, 0, location)), nil
	}
	return nil, fmt.Errorf("unknown brag period %q (want 2026-W40, 2026-10 or 2026)", id)
}

func Braggable(period Period, now time.Time) bool {
	if period.Kind() == config.PromptYear {
		return true
	}
	_, end := period.Range()
	return !now.Before(end)
}

func WeeksSince(first, now time.Time) []Week {
	current := WeekOf(now)
	if first.IsZero() {
		return []Week{current}
	}
	oldest := WeekOf(first.In(now.Location()))
	var weeks []Week
	for week := current; !week.Start.Before(oldest.Start); week = week.Previous() {
		weeks = append(weeks, week)
	}
	return weeks
}

type Brag struct {
	Period       Period
	Created      time.Time
	Updated      time.Time
	SummarizedAt time.Time
	Facts        string
	Summary      string
}

type frontmatter struct {
	ID           string    `yaml:"id"`
	Kind         string    `yaml:"kind"`
	Start        string    `yaml:"start"`
	End          string    `yaml:"end"`
	Created      time.Time `yaml:"created"`
	Updated      time.Time `yaml:"updated"`
	SummarizedAt time.Time `yaml:"summarized_at,omitempty"`
}

const factsHeading = "## Facts"

func summaryHeading(period Period) string {
	if period.Kind() == config.PromptYear {
		return "## Performance review"
	}
	return "## Summary"
}

func headingIndex(text, heading string) int {
	for offset := 0; offset < len(text); {
		found := strings.Index(text[offset:], heading)
		if found < 0 {
			return -1
		}
		at := offset + found
		after := at + len(heading)
		lineStart := at == 0 || text[at-1] == '\n'
		lineEnd := after == len(text) || text[after] == '\n' || text[after] == '\r'
		if lineStart && lineEnd {
			return at
		}
		offset = after
	}
	return -1
}

func (b *Brag) Body() string {
	return fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s\n", factsHeading, b.Facts, summaryHeading(b.Period), b.Summary)
}

func ParseBody(period Period, body string) (*Brag, error) {
	factsStart := headingIndex(body, factsHeading)
	if factsStart < 0 {
		return nil, errors.New("brag needs a '## Facts' section")
	}
	rest := body[factsStart+len(factsHeading):]
	parsed := &Brag{Period: period}
	heading := summaryHeading(period)
	if summaryStart := headingIndex(rest, heading); summaryStart >= 0 {
		parsed.Summary = strings.TrimSpace(rest[summaryStart+len(heading):])
		rest = rest[:summaryStart]
	}
	parsed.Facts = strings.TrimSpace(rest)
	return parsed, nil
}

func Exists(root string, period Period) bool {
	_, err := os.Stat(period.Path(root))
	return err == nil
}

func Load(root string, period Period) (*Brag, error) {
	path := period.Path(root)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(content)
	if !strings.HasPrefix(text, "---\n") {
		return nil, fmt.Errorf("%s: missing frontmatter", path)
	}
	parts := strings.SplitN(text[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%s: unterminated frontmatter", path)
	}
	var meta frontmatter
	if err := yaml.Unmarshal([]byte(parts[0]), &meta); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	parsed, err := ParseBody(period, parts[1])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	parsed.Created, parsed.Updated, parsed.SummarizedAt = meta.Created, meta.Updated, meta.SummarizedAt
	return parsed, nil
}

func (b *Brag) Save(root string) error {
	start, end := b.Period.Range()
	meta, err := yaml.Marshal(frontmatter{
		ID:           b.Period.ID(),
		Kind:         string(b.Period.Kind()),
		Start:        start.Format("2006-01-02"),
		End:          end.AddDate(0, 0, -1).Format("2006-01-02"),
		Created:      b.Created,
		Updated:      b.Updated,
		SummarizedAt: b.SummarizedAt,
	})
	if err != nil {
		return err
	}
	path := b.Period.Path(root)
	if err := os.MkdirAll(filepath.Dir(path), paths.PrivateDirMode); err != nil {
		return err
	}
	var content bytes.Buffer
	content.WriteString("---\n")
	content.Write(meta)
	content.WriteString("---\n")
	content.WriteString(b.Body())
	return os.WriteFile(path, content.Bytes(), paths.PrivateFileMode)
}
