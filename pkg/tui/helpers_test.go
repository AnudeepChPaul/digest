package tui

import "app/pkg/model"

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
	return footerItemsFrom(reviewRunPreviewBindings(running))
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
