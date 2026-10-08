package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/brag"
	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/store"

	tea "github.com/charmbracelet/bubbletea"
)

const benchSizesEnv = "DIGEST_BENCH_SIZES"

var (
	defaultBenchSizes = []int{1, 100, 1000, 10000}
	longNoteLineCounts = []int{500, 2000, 10000}
	maxBenchReviews    = 1000

	benchFixturesMu sync.Mutex
	benchFixtures   = map[int]string{}
)

func benchSizes(b *testing.B) []int {
	raw := strings.TrimSpace(os.Getenv(benchSizesEnv))
	if raw == "" {
		return defaultBenchSizes
	}
	var sizes []int
	for _, field := range strings.Split(raw, ",") {
		size, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || size < 1 {
			b.Fatalf("%s: invalid size %q", benchSizesEnv, field)
		}
		sizes = append(sizes, size)
	}
	return sizes
}

func forEachSize(b *testing.B, label string, run func(b *testing.B, size int)) {
	for _, size := range benchSizes(b) {
		b.Run(fmt.Sprintf("%s=%d", label, size), func(b *testing.B) { run(b, size) })
	}
}

func fixtureNote(index int, now time.Time) *model.Note {
	status := model.StatusActive
	switch {
	case (index+1)%7 == 0:
		status = model.StatusArchived
	case (index+1)%3 == 0:
		status = model.StatusDone
	}
	created := now.Add(-time.Duration(index) * 17 * time.Minute)
	body := fmt.Sprintf("Details for note %d.\nSome context about the work that was done today.", index)
	if index%5 == 0 {
		body += fmt.Sprintf("\nhttps://github.com/acme/console/pull/%d\nhttps://jira.example.com/browse/PROJ-%d", index, index)
	}
	return &model.Note{
		Created: created,
		Updated: created.Add(time.Minute),
		Status:  status,
		Source:  model.SourceManual,
		Summary: fmt.Sprintf("note summary number %d with some text", index),
		Body:    body,
	}
}

func benchFixtureRoot(b *testing.B, notes int) string {
	b.Helper()
	benchFixturesMu.Lock()
	defer benchFixturesMu.Unlock()
	if root, ok := benchFixtures[notes]; ok {
		return root
	}
	root, err := os.MkdirTemp("", fmt.Sprintf("digest-bench-%d-", notes))
	if err != nil {
		b.Fatal(err)
	}
	noteStore := store.New(filepath.Join(root, "notes"))
	now := time.Now()
	for index := 0; index < notes; index++ {
		if err := noteStore.Save(fixtureNote(index, now)); err != nil {
			b.Fatal(err)
		}
	}
	benchFixtures[notes] = root
	return root
}

func removeBenchFixtures() {
	benchFixturesMu.Lock()
	defer benchFixturesMu.Unlock()
	for size, root := range benchFixtures {
		os.RemoveAll(root)
		delete(benchFixtures, size)
	}
}

func benchConfig(root string) *config.Config {
	return &config.Config{DigestRoot: root, GreenOnly: true, Jobs: []config.JobSpec{{Name: "janitor"}, {Name: "repo sync"}}}
}

func startModel(root string) Model {
	m := NewModel(benchConfig(root), nil)
	m = update(m, tea.WindowSizeMsg{Width: 160, Height: 50})
	m = update(m, m.startupNotesCmd()())
	return m
}

func scenarioModel(b *testing.B, notes int) Model {
	b.Helper()
	m := startModel(benchFixtureRoot(b, notes))
	b.Cleanup(m.cancelSession)
	addBenchGitData(&m)
	m.bannerWaveActive = false
	m.mode = ViewDashboard
	selectFirstNote(&m, noteIsActive)
	return m
}

func selectFirstNote(m *Model, matches func(*model.Note) bool) bool {
	for index, item := range m.allNavItems() {
		if item.Note != nil && matches(item.Note) {
			m.selected = index
			return true
		}
	}
	return false
}

func heapInUse() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func BenchmarkScenarioStartup(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		root := benchFixtureRoot(b, notes)
		before := heapInUse()
		kept := startModel(root)
		_ = kept.View()
		after := heapInUse()
		heapMegabytes := float64(max(int64(after)-int64(before), 0)) / (1 << 20)
		kept.cancelSession()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := startModel(root)
			_ = m.View()
			m.cancelSession()
		}
		b.ReportMetric(heapMegabytes, "heap-MB")
	})
}

func BenchmarkScenarioLoadNotes(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		noteStore := store.New(filepath.Join(benchFixtureRoot(b, notes), "notes"))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := noteStore.List(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkScenarioLoadReviews(b *testing.B) {
	forEachSize(b, "reviews", func(b *testing.B, reviews int) {
		reviews = min(reviews, maxBenchReviews)
		root := b.TempDir()
		var refs []review.PRRef
		for number := 1; number <= reviews; number++ {
			ref := review.PRRef{Host: "github.com", Owner: "acme", Repo: "console", Number: number, URL: fmt.Sprintf("https://github.com/acme/console/pull/%d", number)}
			stateDir := review.StateDir(root, ref)
			if err := review.WriteMeta(stateDir, review.Meta{Ref: ref, HeadSHA: "abc"}); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stateDir, review.FindingsFile), []byte(`{"findings":[]}`), 0o644); err != nil {
				b.Fatal(err)
			}
			refs = append(refs, ref)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = readLocalReviews(root, refs, nil)
		}
	})
}

func BenchmarkScenarioTyping(b *testing.B) {
	for _, lines := range longNoteLineCounts {
		for _, cursorAtBottom := range []bool{false, true} {
			position := "top"
			if cursorAtBottom {
				position = "bottom"
			}
			b.Run(fmt.Sprintf("lines=%d/cursor=%s", lines, position), func(b *testing.B) {
				m := editLongNote(benchModel(b), lines)
				if cursorAtBottom {
					m.replaceEditorText(m.editor.Value())
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
					m = next.(Model)
					_ = m.View()
				}
			})
		}
	}
}

func benchKeyLoop(b *testing.B, m Model, keys ...tea.KeyMsg) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(keys[i%len(keys)])
		m = next.(Model)
		_ = m.View()
	}
}

func alternating(forward, back tea.KeyMsg, steps int) []tea.KeyMsg {
	var keys []tea.KeyMsg
	for step := 0; step < steps; step++ {
		keys = append(keys, forward)
	}
	for step := 0; step < steps; step++ {
		keys = append(keys, back)
	}
	return keys
}

func BenchmarkScenarioNavigate(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		m.selected = 0
		benchKeyLoop(b, m, alternating(runes("j"), runes("k"), 20)...)
	})
}

func BenchmarkScenarioPulse(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		m.loadingGit = true
		m.syncPulseRunning = true
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			next, _ := m.Update(syncPulseTickMsg{})
			m = next.(Model)
			_ = m.View()
		}
	})
}

func BenchmarkScenarioHeader(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = m.renderHeader()
		}
	})
}

func BenchmarkScenarioPreviewOpen(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		benchKeyLoop(b, m, tea.KeyMsg{Type: tea.KeyTab}, tea.KeyMsg{Type: tea.KeyEsc})
	})
}

func BenchmarkScenarioPreviewNext(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		m = update(m, tea.KeyMsg{Type: tea.KeyTab})
		benchKeyLoop(b, m, alternating(runes("n"), runes("p"), 20)...)
	})
}

func BenchmarkScenarioQuickActionsOpen(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		benchKeyLoop(b, m, runes("@"), tea.KeyMsg{Type: tea.KeyEsc})
	})
}

func BenchmarkScenarioQuickActionsMove(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := update(scenarioModel(b, notes), runes("@"))
		if m.mode != ViewActionMenu {
			b.Fatalf("quick actions did not open, mode %v", m.mode)
		}
		benchKeyLoop(b, m, runes("j"), runes("k"))
	})
}

func BenchmarkScenarioSearch(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := update(scenarioModel(b, notes), runes("/"))
		for _, key := range "summary" {
			m = update(m, runes(string(key)))
		}
		benchKeyLoop(b, m, runes("1"), tea.KeyMsg{Type: tea.KeyBackspace})
	})
}

func saveAndReload(m Model, note *model.Note) Model {
	m = update(m, m.saveNotesCmd(note)())
	m = update(m, m.loadNotesCmd())
	_ = m.View()
	return m
}

func BenchmarkScenarioSaveNote(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			note := &model.Note{Status: model.StatusActive, Source: model.SourceManual, Created: time.Now().Add(time.Duration(i) * time.Millisecond), Summary: "benchmark save"}
			m = saveAndReload(m, note)
			b.StopTimer()
			saved := findNoteBySummary(m, "benchmark save")
			if saved == nil {
				b.Fatal("saved note not reloaded")
			}
			if err := m.store.Delete(saved); err != nil {
				b.Fatal(err)
			}
			m = update(m, m.loadNotesCmd())
			b.StartTimer()
		}
	})
}

func BenchmarkScenarioDeleteNote(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			note := &model.Note{Status: model.StatusActive, Source: model.SourceManual, Created: time.Now().Add(time.Duration(i) * time.Millisecond), Summary: "benchmark delete"}
			if err := m.store.Save(note); err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			m = update(m, m.deleteNotesCmd(note)())
			m = update(m, m.loadNotesCmd())
			_ = m.View()
		}
	})
}

func findNoteBySummary(m Model, summary string) *model.Note {
	for _, note := range m.notes {
		if note.Summary == summary {
			return note
		}
	}
	return nil
}

func benchScreen(b *testing.B, m Model) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkScenarioScreenArchive(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := update(scenarioModel(b, notes), tea.KeyMsg{Type: tea.KeyCtrlE})
		if m.mode != ViewArchived {
			b.Fatalf("archive did not open, mode %v", m.mode)
		}
		benchKeyLoop(b, m, alternating(runes("j"), runes("k"), 20)...)
	})
}

func BenchmarkScenarioScreenHelp(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := update(scenarioModel(b, notes), runes("?"))
		if m.mode != ViewHelp {
			b.Fatalf("help did not open, mode %v", m.mode)
		}
		benchScreen(b, m)
	})
}

func BenchmarkScenarioScreenSettings(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		m.configPath = filepath.Join(m.cfg.Root(), "config.yaml")
		m = update(m, runes(","))
		if m.mode != ViewSetup {
			b.Fatalf("settings did not open, mode %v", m.mode)
		}
		benchScreen(b, m)
	})
}

func BenchmarkScenarioScreenBrag(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		period := brag.WeekOf(time.Now().AddDate(0, 0, -7))
		entry := &brag.Brag{Period: period, Created: time.Now(), Facts: strings.Repeat("- shipped a thing\n", 40), Summary: strings.Repeat("- Did things that mattered\n", 20)}
		if err := entry.Save(m.cfg.BragDir()); err != nil {
			b.Fatal(err)
		}
		next, _ := m.showBragView(period)
		m = next.(Model)
		if m.mode != ViewBragView {
			b.Fatalf("brag view did not open, mode %v (%s)", m.mode, m.bragNotice)
		}
		benchScreen(b, m)
	})
}

func BenchmarkScenarioScreenReviewDetails(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		found := false
		for index, item := range m.allNavItems() {
			if item.Kind == KindPendingGit {
				m.selected, found = index, true
				break
			}
		}
		if !found {
			b.Fatal("no pull request row")
		}
		m = update(m, tea.KeyMsg{Type: tea.KeyTab})
		if m.mode != ViewPreview {
			b.Fatalf("review details did not open, mode %v", m.mode)
		}
		benchScreen(b, m)
	})
}

func BenchmarkScenarioOverlayLinkMenu(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		if !selectFirstNote(&m, func(note *model.Note) bool { return noteIsActive(note) && len(noteLinks(note)) > 1 }) {
			b.Skip("no note with several links at this size")
		}
		m = update(m, runes("o"))
		if m.mode != ViewLinkMenu {
			b.Fatalf("link menu did not open, mode %v", m.mode)
		}
		benchKeyLoop(b, m, runes("j"), runes("k"))
	})
}

func BenchmarkScenarioOverlayDeleteConfirm(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := update(scenarioModel(b, notes), runes("d"))
		if m.mode != ViewDeleteConfirm {
			b.Fatalf("delete confirm did not open, mode %v", m.mode)
		}
		benchScreen(b, m)
	})
}

func BenchmarkScenarioOverlayError(b *testing.B) {
	forEachSize(b, "notes", func(b *testing.B, notes int) {
		m := scenarioModel(b, notes)
		m.showError("STORE ERROR", errors.New("disk full while saving the note"))
		benchScreen(b, m)
	})
}
