package sourcecontrol

import (
	"context"
	"sync"
	"time"

	"github.com/achandrapaul/digest/pkg/config"
	"github.com/achandrapaul/digest/pkg/review"
)

const SectionCount = 4

type SyncParams struct {
	Config      *config.Config
	Today       time.Time
	PreviousDay time.Time
	Sort        Sort
	KnownMyPRs  []review.PRRef
}

func Sync(ctx context.Context, params SyncParams) <-chan Section {
	engine := NewEngine(params.Config)
	sections := make(chan Section, SectionCount)
	var wg sync.WaitGroup
	wg.Add(SectionCount)
	sendDay := func(day Day, date time.Time) {
		defer wg.Done()
		result := engine.FetchDay(ctx, day, date)
		sections <- Section{Day: &result}
	}
	go sendDay(Today, params.Today)
	previousDay := params.PreviousDay
	if previousDay.IsZero() {
		previousDay = params.Today.AddDate(0, 0, -1)
	}
	go sendDay(Yesterday, previousDay)
	go func() {
		defer wg.Done()
		result := engine.FetchPendingPRs(ctx, params.Sort)
		sections <- Section{Pending: &result}
	}()
	go func() {
		defer wg.Done()
		result := engine.FetchMyPRs(ctx, params.KnownMyPRs)
		sections <- Section{MyPRs: &result}
	}()
	go func() {
		wg.Wait()
		close(sections)
	}()
	return sections
}
