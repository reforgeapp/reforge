package discovery

import (
	"testing"
	"time"

	"reforge/internal/domain"
)

func TestRateLimitRetryDelayIsBoundedAndScheduled(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		requested time.Duration
		want      time.Duration
	}{
		{name: "minimum delay", want: 5 * time.Minute},
		{name: "provider deadline", requested: 12 * time.Minute, want: 12 * time.Minute},
		{name: "bounded deadline", requested: 48 * time.Hour, want: 24 * time.Hour},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, limited := rateLimitRetry(&domain.ProviderError{Kind: "rate_limit", RetryAfter: testCase.requested})
			if !limited || got != testCase.want {
				t.Fatalf("retry = %s, rate limited = %t", got, limited)
			}
		})
	}
	if _, limited := rateLimitRetry(&domain.ProviderError{Kind: "auth"}); limited {
		t.Fatal("auth error treated as rate limit")
	}
}
