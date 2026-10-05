package model

import (
	"net/http"
	"testing"
	"time"
)

func TestRetryAfterParsesSecondsAndDatesWithinBounds(t *testing.T) {
	now := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for value, want := range map[string]time.Duration{
		"":      0,
		"soon":  0,
		"-5":    0,
		"120":   2 * time.Minute,
		"86400": MaxRetryAfter,
		now.Add(90 * time.Second).Format(http.TimeFormat): 90 * time.Second,
		now.Add(-time.Hour).Format(http.TimeFormat):       0,
	} {
		header := http.Header{}
		if value != "" {
			header.Set("Retry-After", value)
		}
		if got := RetryAfter(header, now); got != want {
			t.Errorf("Retry-After %q = %v, want %v", value, got, want)
		}
	}
}
