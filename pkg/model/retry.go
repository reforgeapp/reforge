package model

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const MaxRetryAfter = 6 * time.Hour

func RetryAfter(header http.Header, now time.Time) time.Duration {
	value := strings.TrimSpace(header.Get("Retry-After"))
	var delay time.Duration
	if seconds, err := strconv.Atoi(value); err == nil {
		delay = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		delay = at.Sub(now)
	}
	return min(max(delay, 0), MaxRetryAfter)
}
