package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMessageLogKeepsOnlyTheLatestFifty(t *testing.T) {
	m := syncTestModel(t)
	for index := 1; index <= maxMessages+5; index++ {
		m.postMessage("Source", messageProgress, fmt.Sprintf("message %d", index))
	}
	if len(m.messages) != maxMessages || m.messages[0].text != "message 6" || latestMessageText(m) != fmt.Sprintf("message %d", maxMessages+5) {
		t.Fatalf("kept %d messages, first %q", len(m.messages), m.messages[0].text)
	}
}

func TestMessagesPopupListsNewestFirst(t *testing.T) {
	m := syncTestModel(t)
	m.postMessage("Clipboard", messageSuccess, "copied")
	m.postMessage("JOB ERROR", messageError, "no shell")
	m, _ = pressKey(t, m, "!")
	if m.mode != ViewError || m.errorTitle != "MESSAGES" {
		t.Fatalf("mode %v title %q", m.mode, m.errorTitle)
	}
	view := stripANSI(m.View())
	newest, oldest := strings.Index(view, "JOB ERROR: no shell"), strings.Index(view, "Clipboard: copied")
	if newest < 0 || oldest < 0 || newest > oldest {
		t.Fatalf("popup should list newest first:\n%s", view)
	}
}

func TestMessagesPopupWithNothingPosted(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	m, _ = pressKey(t, m, "!")
	if lines := m.messageLogLines(); m.mode != ViewError || len(lines) != 0 || strings.Contains(stripANSI(m.View()), "No messages yet.") {
		t.Fatalf("lines = %v", lines)
	}
}

func withTrueColour(t *testing.T) {
	t.Helper()
	originalProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(originalProfile) })
}

func TestMessagesPopupStylesEachMessageByItsKind(t *testing.T) {
	withTrueColour(t)
	m := syncTestModel(t)
	m.messages = nil
	m.postMessage("Clipboard", messageSuccess, "copied")
	m.postMessage("JOB ERROR", messageError, "no shell")
	m.postMessage("notes", messageProgress, "reloading")
	m, _ = pressKey(t, m, "!")
	view := m.View()
	for _, want := range []string{
		messageSuccessStyle.Render("✓ " + m.messageLogLines()[2]),
		messageErrorStyle.Render("✗ " + m.messageLogLines()[1]),
		messageLogProgressStyle.Render(m.messageLogLines()[0]),
	} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in:\n%s", want, view)
		}
	}
	if strings.Contains(view, messageErrorStyle.Render("✓ "+m.messageLogLines()[2])) {
		t.Error("success lines must not be red")
	}
}

func TestMessagesPopupAnimatesTheLiveGitSync(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	m.git.loadingGit = true
	m.postMessage(messageSourceGit, messageProgress, "syncing")
	m, _ = pressKey(t, m, "!")
	frames := map[string]bool{}
	for frame := range progressFrames {
		m.syncPulseFrame = frame
		for _, symbol := range progressFrames {
			if strings.Contains(stripANSI(m.View()), symbol+" ") {
				frames[symbol] = true
			}
		}
	}
	if len(frames) != len(progressFrames) || !m.headerAnimating() {
		t.Fatalf("popup frames = %v", frames)
	}
}

func TestMessagesPopupTrimsLongMessagesToItsWidth(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	m.postMessage("AUTOMATION ERROR", messageError, strings.Repeat("long failure ", 30)+"END")
	m, _ = pressKey(t, m, "!")
	view := stripANSI(m.View())
	if strings.Contains(view, "END") || !strings.Contains(view, "…") {
		t.Fatalf("long lines should be trimmed:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line wider than the screen: %q", line)
		}
	}
}

func TestBangInTheMessagesPopupShowsTheFullScrollableLog(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	for index := range 40 {
		m.postMessage("AUTOMATION ERROR", messageError, fmt.Sprintf("failure %d ", index)+strings.Repeat("detail ", 30)+fmt.Sprintf("END%d", index))
	}
	m, _ = pressKey(t, m, "!")
	m, _ = pressKey(t, m, "!")
	view := stripANSI(m.View())
	if m.mode != ViewError || !m.messageLogExpanded || !strings.Contains(strings.Join(strings.Fields(view), " "), "END39") {
		t.Fatalf("! should show the untrimmed newest message:\n%s", view)
	}
	if strings.Contains(view, "END0") {
		t.Fatal("the oldest message should be below the fold")
	}
	for range 400 {
		m, _ = pressKey(t, m, "j")
	}
	if view := strings.Join(strings.Fields(stripANSI(m.View())), " "); !strings.Contains(view, "END0") {
		t.Fatalf("scrolling should reach the oldest message:\n%s", view)
	}
	if m, _ = pressKey(t, m, "esc"); m.mode != ViewDashboard || m.messageLogExpanded {
		t.Fatalf("esc should close the log, mode %v", m.mode)
	}
}

func TestGitSyncDoneWithoutASyncInProgressPostsNothing(t *testing.T) {
	m := syncTestModel(t)
	m.messages = nil
	m.git.loadingGit, m.git.loadingCommits = false, false
	m.noteGitSyncDone()
	if len(m.messages) != 0 || m.gitSyncInProgress() {
		t.Fatalf("messages = %+v", m.messages)
	}
}
