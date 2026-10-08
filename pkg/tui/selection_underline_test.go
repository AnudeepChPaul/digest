package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func withTrueColor(t *testing.T) {
	t.Helper()
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })
}

func assertUnderlinedRightBlock(t *testing.T, name, selectedRow, unselectedRow, wantText string) {
	t.Helper()
	titleEnd := strings.Index(selectedRow, underlineOff)
	if titleEnd < 0 {
		t.Fatalf("%s: title is not underlined: %q", name, selectedRow)
	}
	start := strings.Index(selectedRow[titleEnd:], underlineOn) + titleEnd
	end := strings.LastIndex(selectedRow, underlineOff)
	if start < titleEnd || end < start {
		t.Fatalf("%s: no underlined right block in %q", name, selectedRow)
	}
	block := selectedRow[start+len(underlineOn) : end]
	if plain := stripANSI(block); plain != wantText {
		t.Errorf("%s: underlined text = %q, want %q", name, plain, wantText)
	}
	if before := stripANSI(selectedRow[:start]); !strings.HasSuffix(before, "  ") {
		t.Errorf("%s: gap before the right block must stay plain: %q", name, before)
	}
	if after := strings.TrimSpace(stripANSI(selectedRow[end+len(underlineOff):])); after != "" {
		t.Errorf("%s: text after the underline: %q", name, after)
	}
	colours := strings.ReplaceAll(block, "\x1b[0m"+underlineOn, "\x1b[0m")
	if !strings.HasSuffix(strings.TrimRight(unselectedRow, " \n"), strings.TrimRight(colours, " ")) {
		t.Errorf("%s: right block colours differ from the unselected row:\nselected   %q\nunselected %q", name, colours, unselectedRow)
	}
	if stripANSI(selectedRow) != stripANSI(unselectedRow) {
		t.Errorf("%s: selection changed the row text:\n%q\n%q", name, stripANSI(selectedRow), stripANSI(unselectedRow))
	}
}

func TestUnderlinedKeepsPaddingPlainAndReappliesAfterResets(t *testing.T) {
	block := "  \x1b[31mred\x1b[0m  \x1b[32m✓\x1b[0m  "
	want := "  " + underlineOn + "\x1b[31mred\x1b[0m" + underlineOn + "  \x1b[32m✓\x1b[0m" + underlineOn + underlineOff + "  "
	if underlineOn != "\x1b[4;58;2;205;214;244m" {
		t.Errorf("underline must use the row colour: %q", underlineOn)
	}
	if got := underlined(block); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if underlined("   ") != "   " {
		t.Error("blank block must stay untouched")
	}
}

func TestSelectedRowsUnderlineTheWholeRightBlock(t *testing.T) {
	withTrueColor(t)
	m := myPRStripModel(t)
	note := &model.Note{Summary: "Fix the picker", Created: m.currentDate.Add(8 * time.Hour), Source: model.SourceManual}
	age, source := m.noteTagCells(note)
	assertUnderlinedRightBlock(t, "note", m.renderRow(note, true, 80), m.renderNoteRowWithTags(note, false, 80, m.noteRowTags(note)), stripANSI(age)+tagGap+stripANSI(source))

	selectedNote := m.renderRow(note, true, 80)
	titleAt := strings.Index(selectedNote, "Fix the picker")
	if titleAt < 0 || strings.Count(selectedNote[:titleAt], underlineOn) != 1 || strings.Contains(selectedNote, ";4;") || strings.Contains(selectedNote, "[4;1") {
		t.Errorf("title must use the same white underline, not the text-coloured one: %q", selectedNote)
	}

	pending := pendingItem(5)
	previousShowTags := m.cfg.ShowTags
	m.cfg.ShowTags = config.ShowTagsAlways
	assertUnderlinedRightBlock(t, "pending", m.renderPendingGitRow(&pending, true, 100), m.renderPendingGitRow(&pending, false, 100), stripANSI(joinTags(m.prTags(&pending, true))))
	m.cfg.ShowTags = previousShowTags

	repo := &GitRepoStat{Name: "console", Reviewed: 2, Assigned: 1}
	wantStats := "1 assigned · 2 reviewed"
	if m.cfg.DailyCommitsEnabled() {
		wantStats += " · 0 commits"
	}
	assertUnderlinedRightBlock(t, "repo", m.renderGitRepoRow(repo, true, 60), m.renderGitRepoRow(repo, false, 60), wantStats)

	draft := &JobDraft{Name: "nightly", HasRunDryRun: true}
	assertUnderlinedRightBlock(t, "job", m.renderDraftRow(draft, true, 80), m.renderDraftRow(draft, false, 80), "#job   success")

	m.myPRs[0].ReviewDecision, m.myPRs[0].ChangedFiles, m.myPRs[0].CreatedAt = "APPROVED", 12, time.Now().Add(-49*time.Hour)
	myPRRow := func(selected int) string {
		m.selected = selected
		return m.renderMyPRBlock(3, 80)[1].text
	}
	assertUnderlinedRightBlock(t, "my PR", myPRRow(3), myPRRow(-1), "12 files · 2d ago  "+myPRApprovedGlyph+" ✓")
}
