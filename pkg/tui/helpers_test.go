package tui

import (
	"github.com/achandrapaul/digest/pkg/model"
	"github.com/achandrapaul/digest/pkg/review"
)

func (m Model) loadReviewPollSnapshot() reviewPollSnapshot {
	return loadReviewPollSnapshot(m.reviewRoot(), m.bragRoot(), m.listedReviewRefs(), m.localReviews)
}

func (m Model) renderPRTag(item *GitPRItem, selected bool) string {
	return underlinedWhen(selected, joinTags(m.prTags(item, selected)))
}

func (m Model) reviewFooterItems(item *GitPRItem) []footerItem {
	return footerItemsFrom(m.prPreviewBindings(item))
}

func reviewRunFooterItems(running bool) []footerItem {
	status := review.RunFailed
	if running {
		status = review.RunRunning
	}
	item := NavItem{Kind: KindReviewRun, ReviewRun: &review.ReviewRun{Status: status}}
	return footerItemsFrom(Model{}.runPreviewBindings(item))
}

func (m Model) getYesterdayDoneNotes() []*model.Note {
	return m.groupNotes().previousDone
}

func (m Model) renderDashboardBody() string {
	if m.scrollPending {
		m.settleScroll()
	}
	return m.renderFrameBody(m.currentDashboardFrame())
}
