package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/achandrapaul/digest/pkg/model"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	searchTopMargin    = 1
	searchSnippetWidth = 60
	searchTagPrefix    = "tag:"
	searchDatePrefix   = "date:"
	searchDayFormat    = "02-01-2006"
	invalidDateHint    = "date: use Nd, Nw, Nm, Ny or DD-MM-YYYY"
	olderDateFormat    = "Monday, 02 January 2006"
)

var searchHighlightStyle = lipgloss.NewStyle().Bold(true).Foreground(colourBase).Background(colourYellow)

type searchQuery struct {
	words        []string
	tags         []string
	dates        []dateFilter
	invalidDates []string
}

type dateFilter struct {
	amount int
	unit   byte
	onDay  time.Time
}

const relativeDateUnits = "dwmy"

func parseDateFilter(value string) (dateFilter, bool) {
	if len(value) >= 2 && strings.IndexByte(relativeDateUnits, value[len(value)-1]) >= 0 {
		amount, err := strconv.Atoi(value[:len(value)-1])
		if err != nil || amount < 0 {
			return dateFilter{}, false
		}
		return dateFilter{amount: amount, unit: value[len(value)-1]}, true
	}
	day, err := time.ParseInLocation(searchDayFormat, value, time.Local)
	if err != nil {
		return dateFilter{}, false
	}
	return dateFilter{onDay: day}, true
}

func calendarDaysBetween(earlier, later time.Time) int {
	year, month, day := earlier.Local().Date()
	laterYear, laterMonth, laterDay := later.Local().Date()
	earlierDay := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	laterMidnight := time.Date(laterYear, laterMonth, laterDay, 0, 0, 0, 0, time.Local)
	return int(laterMidnight.Sub(earlierDay).Hours()/24 + 0.5)
}

func (filter dateFilter) matches(noteTime, now time.Time) bool {
	if noteTime.IsZero() {
		return false
	}
	if !filter.onDay.IsZero() {
		return calendarDaysBetween(noteTime, filter.onDay) == 0
	}
	return calendarDaysBetween(noteTime, now) >= 0 && !noteTime.Local().Before(filter.cutoff(now))
}

func (filter dateFilter) cutoff(now time.Time) time.Time {
	year, month, day := now.Local().Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	switch filter.unit {
	case 'w':
		return today.AddDate(0, 0, -7*filter.amount)
	case 'm':
		return today.AddDate(0, -filter.amount, 0)
	case 'y':
		return today.AddDate(-filter.amount, 0, 0)
	}
	return today.AddDate(0, 0, -filter.amount)
}

func parseSearchQuery(input string) searchQuery {
	var query searchQuery
	for _, token := range strings.Fields(strings.ToLower(input)) {
		if tag, isTag := strings.CutPrefix(token, searchTagPrefix); isTag {
			if tag != "" {
				query.tags = append(query.tags, tag)
			}
			continue
		}
		if value, isDate := strings.CutPrefix(token, searchDatePrefix); isDate {
			if filter, valid := parseDateFilter(value); valid {
				query.dates = append(query.dates, filter)
			} else {
				query.invalidDates = append(query.invalidDates, value)
			}
			continue
		}
		query.words = append(query.words, token)
	}
	return query
}

func noteTags(note *model.Note) []string {
	var tags []string
	if note.Source != "" {
		tags = append(tags, string(note.Source))
	}
	if note.Subject != "" && !strings.EqualFold(note.Subject, string(note.Source)) {
		tags = append(tags, note.Subject)
	}
	return tags
}

func tagMatches(tags []string, wanted string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, wanted) {
			return true
		}
	}
	return false
}

var lowerSearchText = strings.ToLower

type searchableNote struct {
	note         *model.Note
	summary      string
	body         string
	lowerSummary string
	lowerBody    string
}

func (entry *searchableNote) refresh(note *model.Note) {
	if entry.note == note && entry.summary == note.Summary && entry.body == note.Body {
		return
	}
	entry.note, entry.summary, entry.body = note, note.Summary, note.Body
	entry.lowerSummary, entry.lowerBody = lowerSearchText(note.Summary), lowerSearchText(note.Body)
}

func (query searchQuery) matches(entry *searchableNote, now time.Time) (matched, bodyMatched bool) {
	note := entry.note
	for _, filter := range query.dates {
		if !filter.matches(note.Created, now) && !filter.matches(note.Updated, now) {
			return false, false
		}
	}
	tags := noteTags(note)
	for _, wanted := range query.tags {
		if !tagMatches(tags, wanted) {
			return false, false
		}
	}
	for _, word := range query.words {
		inBody := strings.Contains(entry.lowerBody, word)
		if !inBody && !strings.Contains(entry.lowerSummary, word) {
			return false, false
		}
		bodyMatched = bodyMatched || inBody
	}
	return true, bodyMatched
}

type searchResultKey struct {
	query  string
	day    string
	status []model.Status
	update []time.Time
}

type searchMemo struct {
	entries []searchableNote
	key     searchResultKey
	results []*model.Note
	heights []int
}

func (memo *searchMemo) matchesKey(query, day string, notes []*model.Note) bool {
	if memo.key.query != query || memo.key.day != day || len(memo.entries) != len(notes) {
		return false
	}
	for index, note := range notes {
		entry := memo.entries[index]
		if entry.note != note || entry.summary != note.Summary || entry.body != note.Body || memo.key.status[index] != note.Status || !memo.key.update[index].Equal(note.Updated) {
			return false
		}
	}
	return true
}

func (m Model) searchMatches() ([]*model.Note, []int) {
	memo := m.searchCache
	if memo == nil {
		memo = &searchMemo{}
	}
	input, now := m.searchInput.Value(), time.Now()
	day := now.Format(searchDayFormat)
	notes := m.notesForBrowsing()
	if memo.matchesKey(input, day, notes) {
		return memo.results, memo.heights
	}
	query := parseSearchQuery(input)
	if len(memo.entries) != len(notes) {
		memo.entries = append(memo.entries[:0], make([]searchableNote, len(notes))...)
	}
	key := searchResultKey{query: input, day: day, status: make([]model.Status, len(notes)), update: make([]time.Time, len(notes))}
	type match struct {
		note        *model.Note
		bodyMatched bool
	}
	var matches []match
	blankQuery := strings.TrimSpace(input) == ""
	for index, note := range notes {
		entry := &memo.entries[index]
		entry.refresh(note)
		key.status[index], key.update[index] = note.Status, note.Updated
		if blankQuery || note.Status != model.StatusActive && note.Status != model.StatusDone {
			continue
		}
		if matched, bodyMatched := query.matches(entry, now); matched {
			matches = append(matches, match{note, bodyMatched})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].note.Updated.After(matches[j].note.Updated)
	})
	results, heights := make([]*model.Note, len(matches)), make([]int, len(matches))
	for index, found := range matches {
		results[index], heights[index] = found.note, 1
		if found.bodyMatched {
			heights[index] = 2
		}
	}
	memo.key, memo.results, memo.heights = key, results, heights
	return results, heights
}

func (m Model) searchResults() []*model.Note {
	results, _ := m.searchMatches()
	return results
}

func searchDateLabel(updated, now time.Time) string {
	if updated.IsZero() {
		return ""
	}
	days := calendarDaysBetween(updated, now)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days <= 7:
		return fmt.Sprintf("%d days ago", days)
	}
	return updated.Local().Format(olderDateFormat)
}

func matchedRunes(text string, words []string) []bool {
	lowered := []rune(text)
	for index, r := range lowered {
		lowered[index] = unicode.ToLower(r)
	}
	marked := make([]bool, len(lowered))
	for _, word := range words {
		wordRunes := []rune(word)
		if len(wordRunes) == 0 {
			continue
		}
		for start := 0; start+len(wordRunes) <= len(lowered); start++ {
			if string(lowered[start:start+len(wordRunes)]) == word {
				for offset := range wordRunes {
					marked[start+offset] = true
				}
			}
		}
	}
	return marked
}

func highlightMatches(text string, words []string, base lipgloss.Style) string {
	textRunes := []rune(text)
	marked := matchedRunes(text, words)
	var rendered strings.Builder
	for start := 0; start < len(textRunes); {
		end := start
		for end < len(textRunes) && marked[end] == marked[start] {
			end++
		}
		segment := string(textRunes[start:end])
		if marked[start] {
			rendered.WriteString(searchHighlightStyle.Render(segment))
		} else {
			rendered.WriteString(base.Render(segment))
		}
		start = end
	}
	return rendered.String()
}

func bodySnippet(body string, words []string, width int) (string, bool) {
	flattened := []rune(strings.Join(strings.Fields(body), " "))
	marked := matchedRunes(string(flattened), words)
	firstMatch := -1
	for index, isMatch := range marked {
		if isMatch {
			firstMatch = index
			break
		}
	}
	if firstMatch < 0 || width < 8 {
		return "", false
	}
	start := max(0, firstMatch-width/3)
	end := min(len(flattened), start+width)
	snippet := string(flattened[start:end])
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(flattened) {
		snippet += "…"
	}
	return snippet, true
}

func searchWindow(heights []int, selected, scroll, available int) (first, last int) {
	if len(heights) == 0 {
		return 0, 0
	}
	selected = min(max(selected, 0), len(heights)-1)
	first = min(max(scroll, 0), selected)
	used := 0
	for index := first; index <= selected; index++ {
		used += heights[index]
	}
	for used > available && first < selected {
		used -= heights[first]
		first++
	}
	last = first
	used = 0
	for last < len(heights) && used+heights[last] <= available {
		used += heights[last]
		last++
	}
	return first, max(last, selected+1)
}

func (m Model) searchFooter(modalWidth int) string {
	return renderModalFooter(footerItemsFrom(m.searchBindings()), modalWidth-6)
}

func (m Model) searchListHeight() int {
	modalWidth := modalWidthFor(m.width)
	fixedLines := modalStyle.GetVerticalFrameSize() + 5 + lipgloss.Height(m.searchFooter(modalWidth))
	return max(3, m.height-searchTopMargin-fixedLines)
}

func (m *Model) keepSearchSelectionVisible() {
	results, heights := m.searchMatches()
	m.searchSelected = min(max(m.searchSelected, 0), max(len(results)-1, 0))
	m.searchScroll, _ = searchWindow(heights, m.searchSelected, m.searchScroll, m.searchListHeight())
}

func (m Model) searchPageSize() int {
	_, heights := m.searchMatches()
	first, last := searchWindow(heights, m.searchSelected, m.searchScroll, m.searchListHeight())
	return max(1, last-first)
}

func (m Model) moveSearchSelection(delta int) Model {
	m.searchSelected += delta
	m.keepSearchSelectionVisible()
	return m
}

func (m Model) renderSearchRow(note *model.Note, selected bool, query searchQuery, dateWidth, width int) string {
	marker, summaryStyle := "  ", itemStyle
	if selected {
		marker, summaryStyle = "› ", selectedSummaryStyle
	}
	dateLabel := searchDateLabel(note.Updated, time.Now())
	dateStyle := mutedStyle
	if len(query.dates) > 0 {
		dateStyle = searchHighlightStyle
	}
	dateColumn := dateStyle.Render(dateLabel) + safeRepeat(" ", dateWidth-ansi.StringWidth(dateLabel))

	var trailing []string
	if note.Status == model.StatusDone {
		trailing = append(trailing, badgeDone.Render("DONE"))
	}
	for _, tag := range noteTags(note) {
		if tagMatches(query.tags, tag) {
			trailing = append(trailing, searchHighlightStyle.Render("#"+tag))
		} else {
			trailing = append(trailing, tagStyle.Render("#"+tag))
		}
	}
	trailingText := strings.Join(trailing, " ")

	summaryIndent := ansi.StringWidth(marker) + dateWidth + 2
	summaryWidth := max(5, width-summaryIndent-ansi.StringWidth(trailingText)-1)
	summary := ansi.Truncate(note.Summary, summaryWidth, "…")
	gap := max(1, summaryWidth-ansi.StringWidth(summary)+1)
	row := marker + dateColumn + "  " + underlinedWhen(selected, highlightMatches(summary, query.words, summaryStyle)) + safeRepeat(" ", gap) + underlinedWhen(selected, trailingText)

	snippet, found := bodySnippet(note.Body, query.words, min(searchSnippetWidth, max(8, width-summaryIndent)))
	if !found {
		return row
	}
	return row + "\n" + safeRepeat(" ", summaryIndent) + highlightMatches(snippet, query.words, mutedStyle)
}

func (m Model) renderSearchModal(modalWidth int) string {
	innerWidth := modalWidth - 6
	results, heights := m.searchMatches()
	query := parseSearchQuery(m.searchInput.Value())
	listHeight := m.searchListHeight()

	titleText := modalTitleStyle.Render(" SEARCH NOTES ")
	countText := mutedStyle.Render(fmt.Sprintf("%d results", len(results)))
	topLine := titleText + safeRepeat(" ", innerWidth-lipgloss.Width(titleText)-lipgloss.Width(countText)) + countText

	searchInput := *m.searchInput
	searchInput.Width = max(10, innerWidth-lipgloss.Width(searchInput.Prompt)-1)

	hintLine := ""
	if len(query.invalidDates) > 0 {
		hintLine = mutedStyle.Render("  " + invalidDateHint)
	} else if notice := m.screenErrorNotice(innerWidth - 2); notice != "" {
		hintLine = "  " + notice
	} else if m.searchNotice != "" {
		hintLine = mutedStyle.Render(ansi.Truncate("  "+m.searchNotice, innerWidth, "…"))
	}

	var listLines []string
	if len(results) == 0 && strings.TrimSpace(m.searchInput.Value()) != "" {
		listLines = append(listLines, "  "+mutedStyle.Render("(no matching notes)"))
	} else if len(results) > 0 {
		dateWidth, now := 0, time.Now()
		for _, note := range results {
			dateWidth = max(dateWidth, ansi.StringWidth(searchDateLabel(note.Updated, now)))
		}
		first, last := searchWindow(heights, m.searchSelected, m.searchScroll, listHeight)
		for index := first; index < last; index++ {
			listLines = append(listLines, strings.Split(m.renderSearchRow(results[index], index == m.searchSelected, query, dateWidth, innerWidth), "\n")...)
		}
	}
	listLines = listLines[:min(len(listLines), listHeight)]
	for len(listLines) < listHeight {
		listLines = append(listLines, "")
	}

	popupContent := lipgloss.JoinVertical(
		lipgloss.Left,
		topLine,
		"",
		searchInput.View(),
		hintLine,
		strings.Join(listLines, "\n"),
		"",
		m.searchFooter(modalWidth),
	)
	modal := modalStyle.Width(modalWidth).Render(popupContent)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, safeRepeat("\n", searchTopMargin)+fitPopup(modal, m.width, m.height-searchTopMargin))
}

func (m Model) searchPreviewIndex(results []*model.Note) int {
	for index, note := range results {
		if note.ID != "" && note.ID == m.searchPreviewID {
			return index
		}
	}
	return min(max(m.searchSelected, 0), len(results)-1)
}

func (m Model) searchPreviewNote() *model.Note {
	results := m.searchResults()
	if len(results) == 0 {
		return nil
	}
	return results[m.searchPreviewIndex(results)]
}

func (m Model) previewingSearch() bool {
	return m.searchPreviewing && m.mode != ViewDashboard && m.mode != ViewSearch
}

func (m Model) searchPreviewItem() (NavItem, bool) {
	note := m.searchPreviewNote()
	if note == nil {
		return NavItem{}, false
	}
	kind := KindTodayNote
	if note.Status == model.StatusDone {
		kind = KindTodayDone
	}
	return NavItem{Kind: kind, Note: note}, true
}

func (m *Model) showSearchPreviewAt(index int) {
	results := m.searchResults()
	if len(results) == 0 {
		return
	}
	m.searchSelected = min(max(index, 0), len(results)-1)
	m.searchPreviewID = results[m.searchSelected].ID
	m.keepSearchSelectionVisible()
	m.searchInput.Blur()
	m.searchPreviewing, m.mode = true, ViewPreview
	m.resetReviewView()
	m.updatePreviewViewport()
}

func (m *Model) afterSearchPreviewArchive() {
	if m.searchSelected < len(m.searchResults()) {
		m.showSearchPreviewAt(m.searchSelected)
		return
	}
	m.leaveSearchPreview()
}

func (m *Model) leaveSearchPreview() {
	m.searchPreviewing, m.mode = false, ViewSearch
	m.keepSearchSelectionVisible()
	m.searchInput.Focus()
}

func (m Model) openSearchPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.showSearchPreviewAt(m.searchSelected)
	return m, nil
}

func (m Model) openSearchResult(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.showSearchPreviewAt(m.searchSelected)
	if !m.previewingSearch() {
		return m, nil
	}
	return m.previewEnter(msg)
}

func (m Model) closeSearchPreview(tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.leaveSearchPreview()
	return m, textinput.Blink
}
