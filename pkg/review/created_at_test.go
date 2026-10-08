package review

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFetchPRCreatedAtParsesGHOutput(t *testing.T) {
	original := runGHPRView
	t.Cleanup(func() { runGHPRView = original })
	runGHPRView = func(ctx context.Context, url string) ([]byte, error) {
		if url != "https://github.com/o/r/pull/1" {
			return nil, errors.New("wrong url")
		}
		return []byte("2026-09-30T10:00:00Z\n"), nil
	}
	created, err := FetchPRCreatedAt(context.Background(), "https://github.com/o/r/pull/1")
	if err != nil || !created.Equal(time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("created = %v, err = %v", created, err)
	}
	runGHPRView = func(context.Context, string) ([]byte, error) { return nil, errors.New("boom") }
	if _, err := FetchPRCreatedAt(context.Background(), "x"); err == nil {
		t.Error("want the gh error")
	}
}
