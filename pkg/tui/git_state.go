package tui

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/review"
	"github.com/AnudeepChPaul/digest/pkg/sourcecontrol"
)

type gitState struct {
	todayGitRepos         []*GitRepoStat
	yesterdayGitRepo      []*GitRepoStat
	pendingGitAction      []GitPRItem
	loadingGit            bool
	fetchGeneration       int
	gitSectionsPending    int
	gitSectionDates       map[string]string
	syncErrors            map[string]string
	gitCancel             context.CancelFunc
	gitFetchCtx           context.Context
	commitsCtx            context.Context
	commitsCancel         context.CancelFunc
	daySyncGeneration     int
	ghReviewedToday       []GitPRItem
	ghReviewedYesterday   []GitPRItem
	ghPendingPRs          []GitPRItem
	myPRs                 []review.QueuedPR
	knownMyPRs            []review.PRRef
	loadingMyPRs          bool
	prDetails             map[string]json.RawMessage
	pendingSort           sourcecontrol.Sort
	pendingMeOnly         bool
	pendingSortChosen     bool
	syncOnLoad            bool
	localCommitsToday     map[string][]GitPRItem
	localCommitsYesterday map[string][]GitPRItem
	loadingCommits        bool
	fetchedPreviousDay    time.Time
	commitsGeneration     int
	closedMyPRs           map[string]string
	prAlertsDue           bool
	gitPopupRepo          *GitRepoStat
	gitPopupTab           int
	gitPopupSelected      int
	changesSince          map[string]changesSinceReview
}
