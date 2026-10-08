package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/paths"
)

func TestParseInterval(t *testing.T) {
	valid := map[string]time.Duration{
		"":               time.Hour,
		"@notify":        time.Hour,
		"@notify:":       time.Hour,
		"2":              2 * time.Hour,
		"2h":             2 * time.Hour,
		"@notify: 90 m ": 90 * time.Minute,
		"@NOTIFY:4H":     4 * time.Hour,
		" 1D ":           24 * time.Hour,
		"@notify:2d":     48 * time.Hour,
		"3d":             72 * time.Hour,
		"72":             72 * time.Hour,
		"1m":             time.Minute,
		"@notify:30m":    30 * time.Minute,
		"4320m":          72 * time.Hour,
	}
	for text, want := range valid {
		got, err := ParseInterval(text)
		if err != nil || got != want {
			t.Errorf("ParseInterval(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
	for _, text := range []string{"2.5h", "0.25", "1.5m", "3.5d", "73", "0", "-1", "abc", "4321m", "1w", "2hh", "@automate:2h", "4d"} {
		if _, err := ParseInterval(text); err == nil {
			t.Errorf("ParseInterval(%q) should fail", text)
		}
	}
}

func TestStoredDecimalIntervalsStillRun(t *testing.T) {
	if got, err := parseStoredInterval("2.5h"); err != nil || got != 150*time.Minute {
		t.Errorf("stored 2.5h = %v, %v", got, err)
	}
}

func TestIntervalLabel(t *testing.T) {
	cases := map[time.Duration]string{time.Hour: "1h", 150 * time.Minute: "150m", 24 * time.Hour: "1d", 48 * time.Hour: "2d", 36 * time.Hour: "36h", time.Minute: "1m", 30 * time.Minute: "30m"}
	for interval, want := range cases {
		if got := IntervalLabel(interval); got != want {
			t.Errorf("IntervalLabel(%v) = %q, want %q", interval, got, want)
		}
	}
}

func TestArgsGroupUnderDigest(t *testing.T) {
	withURL := strings.Join(Args(Notification{Title: "PR reviewed", Message: "console #1", OpenURL: "https://x/pull/1"}), " ")
	if withURL != "-title digest -subtitle PR reviewed -message console #1 -open https://x/pull/1" {
		t.Errorf("with url = %q", withURL)
	}
	withoutURL := strings.Join(Args(Notification{Title: "PR reviewed", Message: "console #1"}), " ")
	if withoutURL != "-title digest -subtitle PR reviewed -message console #1" {
		t.Errorf("without url = %q", withoutURL)
	}
}

func TestArgsGroupRepeatsAndOpenURL(t *testing.T) {
	args := strings.Join(Args(Notification{Title: "PR reviewed", Message: "console #1", OpenURL: "https://x/pull/1", Group: "note-1"}), " ")
	if !strings.Contains(args, "-title digest -subtitle PR reviewed -message console #1") || !strings.Contains(args, "-open https://x/pull/1") || !strings.Contains(args, "-group note-1") {
		t.Errorf("args = %q", args)
	}
	if bare := strings.Join(Args(Notification{Title: "t", Message: "m"}), " "); strings.Contains(bare, "-open") || strings.Contains(bare, "-group") {
		t.Errorf("args without url or group = %q", bare)
	}
}

func TestEntryRoundTripAndRemove(t *testing.T) {
	root := t.TempDir()
	setAt := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	if err := Save(root, Entry{NoteID: "note-1", Summary: "write docs", Interval: "2h", NotifiedAt: setAt}); err != nil {
		t.Fatal(err)
	}
	entry, found := Load(root, "note-1")
	if !found || entry.Summary != "write docs" || entry.Interval != "2h" || !entry.NotifiedAt.Equal(setAt) {
		t.Fatalf("Load = %+v, %v", entry, found)
	}
	if _, err := os.Stat(filepath.Join(root, "notify", "note-1.yaml")); err != nil {
		t.Errorf("entry file missing: %v", err)
	}
	if err := Remove(root, "note-1"); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, "note-1"); err != nil {
		t.Errorf("removing a missing entry should be a no-op: %v", err)
	}
	if _, found := Load(root, "note-1"); found {
		t.Errorf("entry should be gone")
	}
}

func TestRunDueSendsOnlyWhenIntervalHasPassed(t *testing.T) {
	root := t.TempDir()
	var sent []Notification
	previous := Send
	Send = func(notification Notification) error {
		sent = append(sent, notification)
		return nil
	}
	t.Cleanup(func() { Send = previous })
	setAt := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	if err := Save(root, Entry{NoteID: "note-1", Summary: "write docs", Interval: "2h", NotifiedAt: setAt}); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, Entry{NoteID: "note-2", Summary: "call bank", Interval: "1d", NotifiedAt: setAt}); err != nil {
		t.Fatal(err)
	}
	if err := RunDue(root, setAt.Add(119*time.Minute)); err != nil || len(sent) != 0 {
		t.Fatalf("before the interval: err %v sent %+v", err, sent)
	}
	dueAt := setAt.Add(2 * time.Hour)
	if err := RunDue(root, dueAt); err != nil || len(sent) != 1 || sent[0].Group != "note-1" || sent[0].Message != "write docs" {
		t.Fatalf("at the interval: err %v sent %+v", err, sent)
	}
	if entry, _ := Load(root, "note-1"); !entry.NotifiedAt.Equal(dueAt) {
		t.Errorf("notified_at = %v, want %v", entry.NotifiedAt, dueAt)
	}
	if err := RunDue(root, dueAt.Add(time.Minute)); err != nil || len(sent) != 1 {
		t.Errorf("right after notifying: err %v sent %d", err, len(sent))
	}
	if err := Remove(root, "note-1"); err != nil {
		t.Fatal(err)
	}
	if err := RunDue(root, dueAt.Add(4*time.Hour)); err != nil || len(sent) != 1 {
		t.Errorf("removed entry should not notify: err %v sent %+v", err, sent)
	}
}

func TestRunDueKeepsEntryDueWhenSendFails(t *testing.T) {
	root := t.TempDir()
	previous := Send
	Send = func(Notification) error { return os.ErrNotExist }
	t.Cleanup(func() { Send = previous })
	setAt := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	if err := Save(root, Entry{NoteID: "note-1", Summary: "write docs", Interval: "1h", NotifiedAt: setAt}); err != nil {
		t.Fatal(err)
	}
	if err := RunDue(root, setAt.Add(2*time.Hour)); err == nil {
		t.Errorf("a failed send should be reported")
	}
	if entry, _ := Load(root, "note-1"); !entry.NotifiedAt.Equal(setAt) {
		t.Errorf("a failed send should not stamp notified_at")
	}
}

func TestRunDueWithoutNotifyDirIsANoOp(t *testing.T) {
	if err := RunDue(t.TempDir(), time.Now()); err != nil {
		t.Errorf("RunDue = %v", err)
	}
}

func TestLaunchAgentPlist(t *testing.T) {
	plist := LaunchAgentPlist("/usr/local/bin/digest", "/tmp/notify.log", "/opt/homebrew/bin:/usr/bin")
	for _, want := range []string{"<string>" + LaunchAgentLabel + "</string>", "<string>/usr/local/bin/digest</string>", "<string>notify-due</string>", "<key>StartInterval</key>\n\t<integer>60</integer>", "<string>/tmp/notify.log</string>", "<string>/opt/homebrew/bin:/usr/bin</string>"} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q:\n%s", want, plist)
		}
	}
	if escaped := LaunchAgentPlist("/a&b/digest", "/l", "/p"); !strings.Contains(escaped, "/a&amp;b/digest") {
		t.Errorf("plist should escape xml:\n%s", escaped)
	}
}

func TestInstallWritesPlistAndBootstrapsIt(t *testing.T) {
	plistPath := filepath.Join(t.TempDir(), "LaunchAgents", LaunchAgentLabel+".plist")
	appPath := filepath.Join(t.TempDir(), "Digest Notifier.app")
	brewApp := filepath.Join(t.TempDir(), "terminal-notifier.app")
	if err := os.MkdirAll(brewApp, 0755); err != nil {
		t.Fatal(err)
	}
	previousPath, previousLaunchctl := LaunchAgentPath, launchctl
	previousApp, previousTool, previousLookPath := NotifierAppPath, runTool, lookPath
	NotifierAppPath = func() string { return appPath }
	lookPath = func(string) (string, error) {
		return filepath.Join(filepath.Dir(brewApp), "bin", "terminal-notifier"), nil
	}
	var tools []string
	runTool = func(name string, args ...string) error {
		tools = append(tools, name)
		return nil
	}
	t.Cleanup(func() { NotifierAppPath, runTool, lookPath = previousApp, previousTool, previousLookPath })
	var calls []string
	LaunchAgentPath = func() string { return plistPath }
	launchctl = func(args ...string) error {
		calls = append(calls, args[0])
		return nil
	}
	t.Cleanup(func() { LaunchAgentPath, launchctl = previousPath, previousLaunchctl })
	logPath := filepath.Join(t.TempDir(), "logs", "notify.log")
	if err := Install("/bin/digest", logPath); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{plistPath, logPath} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
			t.Errorf("%s should be owner-only: %v %v", path, info, err)
		}
	}
	if !Installed() || strings.Join(calls, ",") != "bootout,bootstrap" {
		t.Errorf("installed %v calls %v", Installed(), calls)
	}
	if len(tools) == 0 || tools[0] != "cp" || tools[len(tools)-1] != "codesign" {
		t.Errorf("install should build the digest notifier first: %v", tools)
	}
	if err := Uninstall(); err != nil || Installed() {
		t.Errorf("uninstall err %v installed %v", err, Installed())
	}
}

func TestArgsRunTheClickCommand(t *testing.T) {
	args := strings.Join(Args(Notification{Title: "t", Message: "m", Execute: "open -b 'com.mitchellh.ghostty'"}), " ")
	if !strings.HasSuffix(args, "-execute open -b 'com.mitchellh.ghostty'") {
		t.Errorf("args = %q", args)
	}
}

func TestClickOpensThePRLinkElseTheTerminal(t *testing.T) {
	root := t.TempDir()
	var sent []Notification
	previousSend := Send
	Send = func(notification Notification) error {
		sent = append(sent, notification)
		return nil
	}
	t.Cleanup(func() { Send = previousSend })
	setAt := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	entries := []Entry{
		{NoteID: "pr-note", Summary: "review", Interval: "1h", NotifiedAt: setAt, OpenURL: "https://github.com/acme/web/pull/7", Terminal: "com.mitchellh.ghostty"},
		{NoteID: "plain-note", Summary: "write docs", Interval: "1h", NotifiedAt: setAt, Terminal: "com.mitchellh.ghostty"},
	}
	for _, entry := range entries {
		if err := Save(root, entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunDue(root, setAt.Add(time.Hour)); err != nil || len(sent) != 2 {
		t.Fatalf("err %v sent %+v", err, sent)
	}
	byGroup := map[string]Notification{sent[0].Group: sent[0], sent[1].Group: sent[1]}
	if pr := byGroup["pr-note"]; pr.OpenURL != "https://github.com/acme/web/pull/7" || pr.Execute != "" {
		t.Errorf("pr note click = %+v", pr)
	}
	plain := byGroup["plain-note"]
	if plain.OpenURL != "" || plain.Execute != "open -b 'com.mitchellh.ghostty'" {
		t.Errorf("plain note click = %+v", plain)
	}
	if unknown := clickTarget(Entry{NoteID: "x"}, Notification{}); unknown.Execute != "" || unknown.OpenURL != "" {
		t.Errorf("no recorded terminal should do nothing on click: %+v", unknown)
	}
}

func TestReminderFilesAreOwnerOnly(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, Entry{NoteID: "note-1", Summary: "write docs", Interval: "2h", NotifiedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	assertPrivateFile(t, entryPath(root, "note-1"))
}

func assertPrivateFile(t *testing.T, path string) {
	t.Helper()
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != paths.PrivateFileMode {
		t.Errorf("%s should be %o: %v %v", path, paths.PrivateFileMode, info, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != paths.PrivateDirMode {
		t.Errorf("%s should be %o: %v %v", filepath.Dir(path), paths.PrivateDirMode, info, err)
	}
}
