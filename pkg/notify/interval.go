package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultInterval = time.Hour
	MaxInterval     = 72 * time.Hour
)

func ParseInterval(text string) (time.Duration, error) {
	return parseInterval(text, false)
}

func parseStoredInterval(text string) (time.Duration, error) {
	return parseInterval(text, true)
}

func parseInterval(text string, allowDecimals bool) (time.Duration, error) {
	normalized := strings.ToLower(strings.TrimSpace(text))
	normalized = strings.TrimPrefix(normalized, "@notify")
	normalized = strings.ReplaceAll(strings.TrimPrefix(normalized, ":"), " ", "")
	if normalized == "" {
		return DefaultInterval, nil
	}
	unit := time.Hour
	switch {
	case strings.HasSuffix(normalized, "d"):
		unit, normalized = 24*time.Hour, strings.TrimSuffix(normalized, "d")
	case strings.HasSuffix(normalized, "h"):
		normalized = strings.TrimSuffix(normalized, "h")
	case strings.HasSuffix(normalized, "m"):
		unit, normalized = time.Minute, strings.TrimSuffix(normalized, "m")
	}
	amount, err := strconv.ParseFloat(normalized, 64)
	if err != nil || (!allowDecimals && strings.Contains(normalized, ".")) {
		return 0, fmt.Errorf("interval %q: use whole minutes like 30m, hours like 2h or days like 1d", text)
	}
	interval := time.Duration(amount * float64(unit))
	if interval <= 0 || interval > MaxInterval {
		return 0, fmt.Errorf("interval %q must be more than 0 and at most 3d", text)
	}
	return interval, nil
}

func IntervalLabel(interval time.Duration) string {
	if interval%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", interval/(24*time.Hour))
	}
	if interval%time.Hour == 0 {
		return fmt.Sprintf("%dh", interval/time.Hour)
	}
	return fmt.Sprintf("%dm", interval/time.Minute)
}
