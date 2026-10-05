package sourcecontrol

import (
	"context"
	"sync"
	"time"

	"app/pkg/config"
)

type SyncParams struct {
	Config      *config.Config
	Today       time.Time
	PreviousDay time.Time
	Sort        Sort
}

func Sync(ctx context.Context, params SyncParams) <-chan Section {
	engine := NewEngine(params.Config)
	sections := make(chan Section, 3)
	var wg sync.WaitGroup
	wg.Add(3)
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
		wg.Wait()
		close(sections)
	}()
	return sections
}
