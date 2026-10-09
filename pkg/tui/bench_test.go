package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/model"
	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"

	tea "github.com/charmbracelet/bubbletea"
)

func benchModel(b *testing.B) Model {
	cfg := &config.Config{DigestRoot: b.TempDir(), GreenOnly: true, Jobs: []config.JobSpec{{Name: "janitor"}, {Name: "repo sync"}}}
	m := NewModel(cfg, nil)
	m.width, m.height = 160, 50
	now := time.Now()
	var notes []*model.Note
	for i := 0; i < 300; i++ {
		status := model.StatusActive
		if i%3 == 0 {
			status = model.StatusDone
		}
		if i%7 == 0 {
			status = model.StatusArchived
		}
		created := now.Add(-time.Duration(i) * 6 * time.Hour)
		notes = append(notes, &model.Note{ID: fmt.Sprint(i), Summary: fmt.Sprintf("note summary number %d with some text", i), Status: status, Source: model.SourceManual, Created: created, Updated: created.Add(time.Hour)})
	}
	m.notes = notes
	addBenchGitData(&m)
	m.mode = ViewDashboard
	m.selected = 5
	return m
}

func addBenchGitData(m *Model) {
	now := time.Now()
	var pending []GitPRItem
	for i := 1; i <= 40; i++ {
		ref := review.PRRef{Host: "github.com", Owner: "o", Repo: fmt.Sprintf("repo%d", i%6), Number: i, URL: fmt.Sprintf("https://github.com/o/repo%d/pull/%d", i%6, i)}
		pending = append(pending, sourcecontrol.NewPRItem(review.QueuedPR{Ref: ref, Title: "Some PR title here", CIState: "SUCCESS", DirectRequest: true, RequestedAt: now.Add(-time.Duration(i) * time.Hour)}, "Pending Review"))
	}
	m.git.ghPendingPRs = pending
	m.git.pendingGitAction = pending
	for i := 0; i < 8; i++ {
		m.git.todayGitRepos = append(m.git.todayGitRepos, &GitRepoStat{Name: fmt.Sprintf("repo%d", i), Reviewed: i, Assigned: 1})
		m.git.yesterdayGitRepo = append(m.git.yesterdayGitRepo, &GitRepoStat{Name: fmt.Sprintf("repo%d", i), Reviewed: i})
	}
	for i := 0; i < 6; i++ {
		m.git.myPRs = append(m.git.myPRs, review.QueuedPR{Ref: review.PRRef{Repo: "mine", Number: i}, HeadRef: "feature/x", CreatedAt: now})
	}
	m.refreshLocalReviews()
}

func BenchmarkKeyDownPlusView(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
		_ = m.View()
		if m.selected > 150 {
			m.selected = 5
		}
	}
}

func BenchmarkPulseTickPlusView(b *testing.B) {
	m := benchModel(b)
	m.git.loadingGit = true
	m.syncPulseRunning = true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(syncPulseTickMsg{})
		m = next.(Model)
		_ = m.View()
	}
}

func BenchmarkUnrelatedTickPlusView(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(bannerWaveTickMsg{})
		m = next.(Model)
		_ = m.View()
	}
}

func BenchmarkAllNavItems(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.allNavItems()
	}
}

func BenchmarkGroupNotes(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.groupNotes()
	}
}

func BenchmarkBuildDashboardContent(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.buildDashboardContent()
	}
}

func BenchmarkRenderHeader(b *testing.B) {
	m := benchModel(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = headerSection{}.Render(m)
	}
}

func BenchmarkRenderFrameBody(b *testing.B) {
	m := benchModel(b)
	frame := m.currentDashboardFrame()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.renderFrameBody(frame)
	}
}

func BenchmarkPreviewView(b *testing.B) {
	m := benchModel(b)
	m.selected = len(m.allNavItems()) - 5
	m.mode = ViewPreview
	m.updatePreviewViewport()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkPreviewNext(b *testing.B) {
	m := benchModel(b)
	m.mode = ViewPreview
	start := len(m.allNavItems()) - 60
	m.selected = start
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
		m = next.(Model)
		_ = m.View()
		if m.selected > start+40 {
			m.selected = start
		}
	}
}

func BenchmarkRenderHeaderSettled(b *testing.B) {
	m := benchModel(b)
	m.bannerWaveActive = false
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = headerSection{}.Render(m)
	}
}

func BenchmarkPulseTickPlusViewSettled(b *testing.B) {
	m := benchModel(b)
	m.bannerWaveActive = false
	m.git.loadingGit = true
	m.syncPulseRunning = true
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		next, _ := m.Update(syncPulseTickMsg{})
		m = next.(Model)
		_ = m.View()
	}
}
