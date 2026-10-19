package budget

import (
	"math"
	"testing"
	"time"

	"github.com/reforgeapp/reforge/internal/domain"
)

func n(value int64) *int64 { return &value }
func TestQuoteUpperBoundAndOverflow(t *testing.T) {
	r := Route{ConnectionID: domain.NewID(), Model: "model", Name: "api", Mode: "priced", PricingVersion: "fixture-v1", InputMicroUSDPerMillion: 1, OutputMicroUSDPerMillion: 1000001, RequestMicroUSD: 3, MaxInputTokens: 100, MaxOutputTokens: 100, MaxMilliseconds: 10000, MaxRequests: 1}
	q := Quote{InputTokens: 1, MaxOutputTokens: 2, MaxMilliseconds: 1000, MaxRequests: 1}
	got, err := estimate(r, q)
	if err != nil || got.MicroUSD != 6 || got.Tokens != 3 || got.Concurrency != 1 {
		t.Fatalf("incorrect conservative quote: %+v %v", got, err)
	}
	r.OutputMicroUSDPerMillion = math.MaxInt64
	r.MaxOutputTokens = math.MaxInt64
	q.MaxOutputTokens = math.MaxInt64
	if _, err = estimate(r, q); err == nil {
		t.Fatal("overflow produced cheap quote")
	}
	r.Mode = "priced"
	r.PricingVersion = ""
	if _, err = estimate(r, q); err == nil {
		t.Fatal("missing direct API price accepted")
	}
	r.Mode = "quota"
	r.InputMicroUSDPerMillion = 0
	r.OutputMicroUSDPerMillion = 0
	r.RequestMicroUSD = 0
	r.MaxOutputTokens = 100
	q.MaxOutputTokens = 2
	if _, err = estimate(r, q); err != ErrRevoked {
		t.Fatalf("unqualified subscription route accepted: %v", err)
	}
	r.Qualified = true
	r.QualificationRef = "T16-fixture"
	if got, err = estimate(r, q); err != nil || got.MicroUSD != 0 || got.Requests != 1 {
		t.Fatalf("quota route failed: %+v %v", got, err)
	}
}

func TestAccountingWindowsAndArithmetic(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	for _, tt := range []struct {
		period string
		day    int
	}{{"daily", 20}, {"monthly", 1}} {
		start, err := periodStart(Limit{Period: tt.period}, now)
		if err != nil || start.Day() != tt.day || start.Hour() != 0 || start.Location() != time.UTC {
			t.Fatalf("wrong %s bucket %v", tt.period, start)
		}
	}
	if _, err := periodStart(Limit{Period: "custom", Start: now.Add(-time.Hour), End: now}, now); err != ErrCapacity {
		t.Fatal("expired custom window accepted")
	}
	if _, err := plus(Amount{MicroUSD: math.MaxInt64}, Amount{MicroUSD: 1}); err == nil {
		t.Fatal("sum overflow accepted")
	}
	if _, err := minus(Amount{}, Amount{Requests: 1}); err == nil {
		t.Fatal("negative held balance accepted")
	}
}

func TestObservedAmountUsesActualUnitsAndPreservesDebt(t *testing.T) {
	route := Route{ConnectionID: "00000000-0000-4000-8000-000000000001", Model: "example", Name: "default", Mode: "priced", PricingVersion: "test", InputMicroUSDPerMillion: 1000000, OutputMicroUSDPerMillion: 2000000, RequestMicroUSD: 3, MaxInputTokens: 10, MaxOutputTokens: 10, MaxMilliseconds: 1000, MaxRequests: 1}
	for _, check := range []struct{ input, output, cost int64 }{{2, 0, 5}, {2, 4, 13}, {100, 100, 303}} {
		amount, err := ObservedAmount(route, check.input, check.output, 2000)
		if err != nil || amount.MicroUSD != check.cost || amount.Tokens != check.input+check.output || amount.Milliseconds != 2000 || amount.Concurrency != 0 {
			t.Fatalf("actual unit calculation: %+v %v", amount, err)
		}
	}
	if _, err := ObservedAmount(route, -1, 1, 1); err == nil {
		t.Fatal("negative usage accepted")
	}
}
