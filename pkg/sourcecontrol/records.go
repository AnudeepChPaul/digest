package sourcecontrol

import (
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
)

var ReviewNoteStates = map[string]bool{"APPROVED": true, "CHANGES_REQUESTED": true, "COMMENTED": true}

func ReviewRecord(pr review.QueuedPR, state string, reviewedAt time.Time) review.ActivityPR {
	return review.ActivityPR{Number: pr.Ref.Number, Title: pr.Title, URL: pr.Ref.URL, Repository: pr.Ref.Repo, Owner: pr.Ref.Owner, CreatedAt: pr.CreatedAt, State: state, ReviewedAt: reviewedAt}
}
